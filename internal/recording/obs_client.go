package recording

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"refleks/internal/models"
)

const obsWebSocketSubprotocol = "obswebsocket.json"

var obsReplaySavedEventTimeout = 8 * time.Second

type OBSVersionInfo struct {
	OBSVersion          string
	OBSWebSocketVersion string
}

type OBSReplayBufferInfo struct {
	Active bool
}

type OBSReplaySaveResult struct {
	Path        string
	RequestedAt time.Time
	ConfirmedAt time.Time
	FileModTime time.Time
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

	hello, err := c.readMessage(ctx)
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

	if err := c.writeMessage(ctx, obsMessage{Op: 1, Data: mustJSON(identify)}); err != nil {
		c.Close()
		return fmt.Errorf("OBS identify failed: %w", err)
	}

	identified, err := c.readMessage(ctx)
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

func (c *OBSClient) StartReplayBuffer(ctx context.Context) error {
	_, err := c.request(ctx, "StartReplayBuffer", nil)
	return err
}

func (c *OBSClient) SaveReplayBuffer(ctx context.Context) (OBSReplaySaveResult, error) {
	if c == nil || c.conn == nil {
		return OBSReplaySaveResult{}, errors.New("OBS client is not connected")
	}

	requestStarted := time.Now().UTC()
	waitCtx, cancel := context.WithTimeout(ctx, obsReplaySavedEventTimeout)
	defer cancel()

	requestID := strconv.FormatUint(c.requestID.Add(1), 10)
	req := obsRequestData{
		RequestType: "SaveReplayBuffer",
		RequestID:   requestID,
	}
	if err := c.writeMessage(waitCtx, obsMessage{Op: 6, Data: mustJSON(req)}); err != nil {
		return OBSReplaySaveResult{}, err
	}

	var (
		responseOK   bool
		freshResult  OBSReplaySaveResult
		lastPathErr  error
		sawSavedPath bool
	)
	for {
		msg, err := c.readMessage(waitCtx)
		if err != nil {
			if waitCtx.Err() != nil {
				if lastPathErr != nil {
					return OBSReplaySaveResult{}, fmt.Errorf("OBS reported a replay path that was not a fresh file: %w", lastPathErr)
				}
				if sawSavedPath {
					return OBSReplaySaveResult{}, errors.New("OBS did not confirm a fresh replay path after SaveReplayBuffer")
				}
				return OBSReplaySaveResult{}, errors.New("OBS did not report ReplayBufferSaved after SaveReplayBuffer")
			}
			return OBSReplaySaveResult{}, err
		}

		if msg.Op == 5 {
			if path := replaySavedPathFromEvent(msg); path != "" {
				sawSavedPath = true
				modTime, err := validateFreshReplayPath(path, requestStarted)
				if err != nil {
					lastPathErr = err
					continue
				}
				freshResult = OBSReplaySaveResult{
					Path:        path,
					RequestedAt: requestStarted,
					ConfirmedAt: time.Now().UTC(),
					FileModTime: modTime,
				}
				if responseOK {
					return freshResult, nil
				}
			}
			continue
		}
		if msg.Op != 7 {
			continue
		}

		var response obsRequestResponseData
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			return OBSReplaySaveResult{}, err
		}
		if response.RequestID != requestID {
			continue
		}
		if !response.RequestStatus.Result {
			if response.RequestStatus.Comment != "" {
				return OBSReplaySaveResult{}, fmt.Errorf("OBS request SaveReplayBuffer failed: %s", response.RequestStatus.Comment)
			}
			return OBSReplaySaveResult{}, fmt.Errorf("OBS request SaveReplayBuffer failed with code %d", response.RequestStatus.Code)
		}
		responseOK = true
		if freshResult.Path != "" {
			return freshResult, nil
		}
	}
}

func validateFreshReplayPath(path string, requestStarted time.Time) (time.Time, error) {
	if strings.TrimSpace(path) == "" {
		return time.Time{}, errors.New("OBS reported an empty replay path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("OBS reported saved replay path %q but it is not available: %w", path, err)
	}
	if info.IsDir() {
		return time.Time{}, fmt.Errorf("OBS reported saved replay path %q but it is a directory", path)
	}
	if info.ModTime().Before(requestStarted) {
		return time.Time{}, fmt.Errorf("OBS reported stale replay path %q modified before SaveReplayBuffer request", path)
	}
	return info.ModTime().UTC(), nil
}

func (c *OBSClient) request(ctx context.Context, requestType string, requestData any) (map[string]json.RawMessage, error) {
	return c.requestWithEvents(ctx, requestType, requestData, nil)
}

func (c *OBSClient) requestWithEvents(ctx context.Context, requestType string, requestData any, onEvent func(obsMessage)) (map[string]json.RawMessage, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("OBS client is not connected")
	}

	requestID := strconv.FormatUint(c.requestID.Add(1), 10)
	req := obsRequestData{
		RequestType: requestType,
		RequestID:   requestID,
		RequestData: requestData,
	}
	if err := c.writeMessage(ctx, obsMessage{Op: 6, Data: mustJSON(req)}); err != nil {
		return nil, err
	}

	for {
		msg, err := c.readMessage(ctx)
		if err != nil {
			return nil, err
		}
		if msg.Op == 5 {
			if onEvent != nil {
				onEvent(msg)
			}
			continue
		}
		if msg.Op != 7 {
			continue
		}

		var response obsRequestResponseData
		if err := json.Unmarshal(msg.Data, &response); err != nil {
			return nil, err
		}
		if response.RequestID != requestID {
			continue
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

func (c *OBSClient) readMessage(ctx context.Context) (obsMessage, error) {
	var msg obsMessage
	if c == nil || c.conn == nil {
		return msg, errors.New("OBS client is not connected")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetReadDeadline(deadline)
	} else {
		_ = c.conn.SetReadDeadline(time.Time{})
	}
	if err := c.conn.ReadJSON(&msg); err != nil {
		return msg, err
	}
	return msg, nil
}

func (c *OBSClient) writeMessage(ctx context.Context, msg obsMessage) error {
	if c == nil || c.conn == nil {
		return errors.New("OBS client is not connected")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(deadline)
	} else {
		_ = c.conn.SetWriteDeadline(time.Time{})
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

func replayPathFromData(data map[string]json.RawMessage) string {
	for _, key := range []string{"savedReplayPath", "replayPath", "outputPath"} {
		if path := stringField(data, key); path != "" {
			return path
		}
	}
	return ""
}

func replaySavedPathFromEvent(msg obsMessage) string {
	if msg.Op != 5 {
		return ""
	}
	var event obsEventData
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return ""
	}
	if event.EventType != "ReplayBufferSaved" {
		return ""
	}
	return replayPathFromData(event.EventData)
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

type obsEventData struct {
	EventType string                     `json:"eventType"`
	EventData map[string]json.RawMessage `json:"eventData,omitempty"`
}
