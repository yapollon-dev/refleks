package recording

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"refleks/internal/models"
)

type fakeOBSServer struct {
	server                    *httptest.Server
	password                  string
	salt                      string
	challenge                 string
	replayActive              bool
	requestError              bool
	closeAfterHello           bool
	replayPath                string
	replayStatusNotReadyCount int
	noReplayEvent             bool
	replayEventAfterResponse  bool
	writeReplayOnSave         bool
	saveReplayRequests        int
	startReplayRequests       int
}

func newFakeOBSServer(t *testing.T, configure func(*fakeOBSServer)) *fakeOBSServer {
	t.Helper()
	fake := &fakeOBSServer{
		salt:         "salt",
		challenge:    "challenge",
		replayActive: true,
	}
	if configure != nil {
		configure(fake)
	}

	upgrader := websocket.Upgrader{Subprotocols: []string{obsWebSocketSubprotocol}}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		hello := obsHelloData{RPCVersion: 1}
		if fake.password != "" {
			hello.Authentication = &obsAuthenticationData{Salt: fake.salt, Challenge: fake.challenge}
		}
		if err := conn.WriteJSON(obsMessage{Op: 0, Data: mustJSON(hello)}); err != nil {
			return
		}
		if fake.closeAfterHello {
			return
		}

		var identifyMsg obsMessage
		if err := conn.ReadJSON(&identifyMsg); err != nil {
			return
		}
		if identifyMsg.Op != 1 {
			return
		}
		var identify obsIdentifyData
		_ = json.Unmarshal(identifyMsg.Data, &identify)
		if fake.password != "" {
			want := CreateAuthentication(fake.password, fake.salt, fake.challenge)
			if identify.Authentication != want {
				return
			}
		}
		if err := conn.WriteJSON(obsMessage{Op: 2, Data: mustJSON(map[string]any{"negotiatedRpcVersion": 1})}); err != nil {
			return
		}

		for {
			var reqMsg obsMessage
			if err := conn.ReadJSON(&reqMsg); err != nil {
				return
			}
			if reqMsg.Op != 6 {
				return
			}
			var req obsRequestData
			if err := json.Unmarshal(reqMsg.Data, &req); err != nil {
				return
			}
			response := obsRequestResponseData{
				RequestType: req.RequestType,
				RequestID:   req.RequestID,
				RequestStatus: obsRequestStatus{
					Result: true,
					Code:   100,
				},
			}
			var pendingEvent *obsEventData
			if fake.requestError {
				response.RequestStatus = obsRequestStatus{Result: false, Code: 500, Comment: "boom"}
			} else {
				switch req.RequestType {
				case "GetVersion":
					response.ResponseData = map[string]json.RawMessage{
						"obsVersion":          mustJSON("30.2.0"),
						"obsWebSocketVersion": mustJSON("5.5.0"),
					}
				case "GetReplayBufferStatus":
					if fake.replayStatusNotReadyCount > 0 {
						fake.replayStatusNotReadyCount--
						response.RequestStatus = obsRequestStatus{Result: false, Code: 500, Comment: "OBS is not ready to perform the request."}
					} else {
						response.ResponseData = map[string]json.RawMessage{
							"outputActive": mustJSON(fake.replayActive),
						}
					}
				case "StartReplayBuffer":
					fake.startReplayRequests++
					fake.replayActive = true
				case "SaveReplayBuffer":
					fake.saveReplayRequests++
					if fake.replayPath != "" && fake.writeReplayOnSave {
						if err := os.WriteFile(fake.replayPath, []byte("clip"), 0o644); err != nil {
							return
						}
					}
					if fake.replayPath != "" && !fake.noReplayEvent {
						event := obsEventData{
							EventType: "ReplayBufferSaved",
							EventData: map[string]json.RawMessage{
								"savedReplayPath": mustJSON(fake.replayPath),
							},
						}
						if fake.replayEventAfterResponse {
							pendingEvent = &event
						} else {
							if err := conn.WriteJSON(obsMessage{Op: 5, Data: mustJSON(event)}); err != nil {
								return
							}
						}
					}
				case "GetLastReplayBufferReplay":
					response.ResponseData = map[string]json.RawMessage{
						"savedReplayPath": mustJSON(fake.replayPath),
					}
				default:
					response.RequestStatus = obsRequestStatus{Result: false, Code: 404, Comment: "unknown request"}
				}
			}
			if err := conn.WriteJSON(obsMessage{Op: 7, Data: mustJSON(response)}); err != nil {
				return
			}
			if pendingEvent != nil {
				if err := conn.WriteJSON(obsMessage{Op: 5, Data: mustJSON(*pendingEvent)}); err != nil {
					return
				}
			}
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func TestCreateAuthenticationMatchesOBSAlgorithm(t *testing.T) {
	password := "secret"
	salt := "salt"
	challenge := "challenge"

	secretHash := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(secretHash[:])
	authHash := sha256.Sum256([]byte(secret + challenge))
	want := base64.StdEncoding.EncodeToString(authHash[:])

	if got := CreateAuthentication(password, salt, challenge); got != want {
		t.Fatalf("authentication = %q, want %q", got, want)
	}
}

func TestOBSClientConnectsAndReadsStatus(t *testing.T) {
	fake := newFakeOBSServer(t, nil)
	client := NewOBSClient(fake.settings(t, ""))

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	version, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if version.OBSVersion != "30.2.0" || version.OBSWebSocketVersion != "5.5.0" {
		t.Fatalf("unexpected version: %#v", version)
	}

	replay, err := client.GetReplayBufferStatus(context.Background())
	if err != nil {
		t.Fatalf("GetReplayBufferStatus failed: %v", err)
	}
	if !replay.Active {
		t.Fatalf("replay buffer should be active")
	}
}

func TestOBSClientAuthenticatesWithChallenge(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.password = "correct-password"
	})
	client := NewOBSClient(fake.settings(t, "correct-password"))

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("authenticated connect failed: %v", err)
	}
	client.Close()
}

func TestOBSClientAuthFailure(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.password = "correct-password"
	})
	client := NewOBSClient(fake.settings(t, "wrong-password"))

	err := client.Connect(context.Background())
	if err == nil {
		t.Fatalf("connect should fail with wrong password")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "authentication") {
		t.Fatalf("error should mention authentication, got %v", err)
	}
}

func TestOBSClientRequestError(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.requestError = true
	})
	client := NewOBSClient(fake.settings(t, ""))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	_, err := client.GetVersion(context.Background())
	if err == nil {
		t.Fatalf("GetVersion should fail when OBS returns request error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should include OBS comment, got %v", err)
	}
}

func TestOBSClientConnectionLoss(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.closeAfterHello = true
	})
	client := NewOBSClient(fake.settings(t, ""))

	err := client.Connect(context.Background())
	if err == nil {
		t.Fatalf("connect should fail after server closes")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("expected connection loss, got %v", err)
	}
}

func TestOBSClientSaveReplayBufferUsesSavedEvent(t *testing.T) {
	replayPath := filepath.Join(t.TempDir(), "event replay.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	client := NewOBSClient(fake.settings(t, ""))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	got, err := client.SaveReplayBuffer(context.Background())
	if err != nil {
		t.Fatalf("SaveReplayBuffer failed: %v", err)
	}
	if got.Path != replayPath {
		t.Fatalf("saved path = %q, want %q", got.Path, replayPath)
	}
	if got.RequestedAt.IsZero() || got.ConfirmedAt.IsZero() || got.FileModTime.IsZero() {
		t.Fatalf("save result should include timing metadata: %#v", got)
	}
}

func TestOBSClientSaveReplayBufferUsesSavedEventAfterResponse(t *testing.T) {
	replayPath := filepath.Join(t.TempDir(), "delayed event replay.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayPath = replayPath
		f.writeReplayOnSave = true
		f.replayEventAfterResponse = true
	})
	client := NewOBSClient(fake.settings(t, ""))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	got, err := client.SaveReplayBuffer(context.Background())
	if err != nil {
		t.Fatalf("SaveReplayBuffer failed: %v", err)
	}
	if got.Path != replayPath {
		t.Fatalf("saved path = %q, want %q", got.Path, replayPath)
	}
}

func TestOBSClientSaveReplayBufferDoesNotFallBackToLastReplay(t *testing.T) {
	oldTimeout := obsReplaySavedEventTimeout
	obsReplaySavedEventTimeout = 100 * time.Millisecond
	t.Cleanup(func() { obsReplaySavedEventTimeout = oldTimeout })

	replayPath := filepath.Join(t.TempDir(), "fallback replay.mp4")
	if err := os.WriteFile(replayPath, []byte("clip"), 0o644); err != nil {
		t.Fatalf("write replay fixture: %v", err)
	}
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayPath = replayPath
		f.noReplayEvent = true
	})
	client := NewOBSClient(fake.settings(t, ""))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	if got, err := client.SaveReplayBuffer(context.Background()); err == nil {
		t.Fatalf("SaveReplayBuffer should not fall back to last replay, got %#v", got)
	}
}

func TestOBSClientSaveReplayBufferRejectsStaleSavedEvent(t *testing.T) {
	oldTimeout := obsReplaySavedEventTimeout
	obsReplaySavedEventTimeout = 100 * time.Millisecond
	t.Cleanup(func() { obsReplaySavedEventTimeout = oldTimeout })

	replayPath := filepath.Join(t.TempDir(), "stale replay.mp4")
	if err := os.WriteFile(replayPath, []byte("old clip"), 0o644); err != nil {
		t.Fatalf("write replay fixture: %v", err)
	}
	staleTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(replayPath, staleTime, staleTime); err != nil {
		t.Fatalf("make replay stale: %v", err)
	}
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayPath = replayPath
	})
	client := NewOBSClient(fake.settings(t, ""))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer client.Close()

	err := func() error {
		_, err := client.SaveReplayBuffer(context.Background())
		return err
	}()
	if err == nil {
		t.Fatalf("SaveReplayBuffer should reject stale saved event")
	}
	if !strings.Contains(err.Error(), "not a fresh file") {
		t.Fatalf("error should explain stale replay rejection, got %v", err)
	}
}

func (f *fakeOBSServer) settings(t *testing.T, password string) models.RecordingSettings {
	t.Helper()
	u, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatalf("parse fake URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split fake host: %v", err)
	}
	portInt, err := strconv.Atoi(port)
	if err != nil || portInt == 0 {
		t.Fatalf("invalid fake OBS port %q", port)
	}
	return models.RecordingSettings{
		OBSHost:     host,
		OBSPort:     portInt,
		OBSPassword: password,
	}
}
