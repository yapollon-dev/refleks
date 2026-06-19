package recording

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"refleks/internal/models"
)

const obsWebSocketSubprotocol = "obswebsocket.json"

type OBSVersionInfo struct {
	OBSVersion          string
	OBSWebSocketVersion string
}

type OBSReplayBufferInfo struct {
	Active bool
}

type OBSClient struct {
	cfg       models.RecordingSettings
	conn      *websocket.Conn
	requestID atomic.Uint64
}

func NewOBSClient(cfg models.RecordingSettings) *OBSClient {
	return &OBSClient{cfg: cfg}
}

func CreateAuthentication(password, salt, challenge string) string {
	secretHash := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(secretHash[:])
	authHash := sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(authHash[:])
}

func (c *OBSClient) Connect(ctx context.Context) error {
	if c == nil {
		return errors.New("OBS client is not initialized")
	}

	endpoint, err := websocketURL(c.cfg)
	if err != nil {
		return err
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Subprotocols:     []string{obsWebSocketSubprotocol},
	}
	conn, _, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return fmt.Errorf("OBS connection failed: %w", err)
	}
	c.conn = conn

	hello, err := c.readMessage()
	if err != nil {
		c.Close()
		return fmt.Errorf("OBS hello failed: %w", err)
	}
	if hello.Op != 0 {
		c.Close()
		return fmt.Errorf("OBS hello failed: unexpected op %d", hello.Op)
	}

	var helloData obsHelloData
	if err := json.Unmarshal(hello.Data, &helloData); err != nil {
		c.Close()
		return fmt.Errorf("OBS hello parse failed: %w", err)
	}

	identify := obsIdentifyData{RPCVersion: helloData.RPCVersion}
	if helloData.Authentication != nil {
		if strings.TrimSpace(c.cfg.OBSPassword) == "" {
			c.Close()
			return errors.New("OBS authentication required but no password is configured")
		}
		identify.Authentication = CreateAuthentication(c.cfg.OBSPassword, helloData.Authentication.Salt, helloData.Authentication.Challenge)
	}

	if err := c.writeMessage(obsMessage{Op: 1, Data: mustJSON(identify)}); err != nil {
		c.Close()
		return fmt.Errorf("OBS identify failed: %w", err)
	}

	identified, err := c.readMessage()
	if err != nil {
		c.Close()
		return fmt.Errorf("OBS authentication failed: %w", err)
	}
	if identified.Op != 2 {
		c.Close()
		return fmt.Errorf("OBS authentication failed: unexpected op %d", identified.Op)
	}
	return nil
}

func (c *OBSClient) Close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.Close()
	c.conn = nil
}

func (c *OBSClient) GetVersion(ctx context.Context) (OBSVersionInfo, error) {
	data, err := c.request(ctx, "GetVersion", nil)
	if err != nil {
		return OBSVersionInfo{}, err
	}
	return OBSVersionInfo{
		OBSVersion:          stringField(data, "obsVersion"),
		OBSWebSocketVersion: stringField(data, "obsWebSocketVersion"),
	}, nil
}

func (c *OBSClient) GetReplayBufferStatus(ctx context.Context) (OBSReplayBufferInfo, error) {
	data, err := c.request(ctx, "GetReplayBufferStatus", nil)
	if err != nil {
		return OBSReplayBufferInfo{}, err
	}
	return OBSReplayBufferInfo{Active: boolField(data, "outputActive")}, nil
}

func (c *OBSClient) request(ctx context.Context, requestType string, requestData any) (map[string]json.RawMessage, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("OBS client is not connected")
	}

	requestID := strconv.FormatUint(c.requestID.Add(1), 10)
	req := obsRequestData{
		RequestType: requestType,
		RequestID:   requestID,
		RequestData: requestData,
	}
	if err := c.writeMessage(obsMessage{Op: 6, Data: mustJSON(req)}); err != nil {
		return nil, err
	}

	type result struct {
		msg obsMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		msg, err := c.readMessage()
		ch <- result{msg: msg, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		if res.msg.Op != 7 {
			return nil, fmt.Errorf("OBS request %s failed: unexpected op %d", requestType, res.msg.Op)
		}

		var response obsRequestResponseData
		if err := json.Unmarshal(res.msg.Data, &response); err != nil {
			return nil, err
		}
		if response.RequestID != requestID {
			return nil, fmt.Errorf("OBS request %s failed: mismatched request id", requestType)
		}
		if !response.RequestStatus.Result {
			if response.RequestStatus.Comment != "" {
				return nil, fmt.Errorf("OBS request %s failed: %s", requestType, response.RequestStatus.Comment)
			}
			return nil, fmt.Errorf("OBS request %s failed with code %d", requestType, response.RequestStatus.Code)
		}
		if response.ResponseData == nil {
			return map[string]json.RawMessage{}, nil
		}
		return response.ResponseData, nil
	}
}

func (c *OBSClient) readMessage() (obsMessage, error) {
	var msg obsMessage
	if c == nil || c.conn == nil {
		return msg, errors.New("OBS client is not connected")
	}
	if err := c.conn.ReadJSON(&msg); err != nil {
		return msg, err
	}
	return msg, nil
}

func (c *OBSClient) writeMessage(msg obsMessage) error {
	if c == nil || c.conn == nil {
		return errors.New("OBS client is not connected")
	}
	return c.conn.WriteJSON(msg)
}

func websocketURL(cfg models.RecordingSettings) (string, error) {
	host := strings.TrimSpace(cfg.OBSHost)
	if host == "" {
		return "", errors.New("OBS host is not configured")
	}
	if cfg.OBSPort <= 0 {
		return "", errors.New("OBS port is not configured")
	}

	u := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%d", host, cfg.OBSPort),
		Path:   "/",
	}
	return u.String(), nil
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func stringField(data map[string]json.RawMessage, key string) string {
	if raw, ok := data[key]; ok {
		var out string
		if err := json.Unmarshal(raw, &out); err == nil {
			return out
		}
	}
	return ""
}

func boolField(data map[string]json.RawMessage, key string) bool {
	if raw, ok := data[key]; ok {
		var out bool
		if err := json.Unmarshal(raw, &out); err == nil {
			return out
		}
	}
	return false
}

type obsMessage struct {
	Op   int             `json:"op"`
	Data json.RawMessage `json:"d,omitempty"`
}

type obsHelloData struct {
	RPCVersion     int                    `json:"rpcVersion"`
	Authentication *obsAuthenticationData `json:"authentication,omitempty"`
}

type obsAuthenticationData struct {
	Challenge string `json:"challenge"`
	Salt      string `json:"salt"`
}

type obsIdentifyData struct {
	RPCVersion     int    `json:"rpcVersion"`
	Authentication string `json:"authentication,omitempty"`
}

type obsRequestData struct {
	RequestType string `json:"requestType"`
	RequestID   string `json:"requestId"`
	RequestData any    `json:"requestData,omitempty"`
}

type obsRequestStatus struct {
	Result  bool   `json:"result"`
	Code    int    `json:"code"`
	Comment string `json:"comment,omitempty"`
}

type obsRequestResponseData struct {
	RequestType   string                     `json:"requestType"`
	RequestID     string                     `json:"requestId"`
	RequestStatus obsRequestStatus           `json:"requestStatus"`
	ResponseData  map[string]json.RawMessage `json:"responseData,omitempty"`
}
