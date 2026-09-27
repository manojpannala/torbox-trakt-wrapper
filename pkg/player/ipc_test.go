package player_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
)

func startMockMPVSocket(t *testing.T, handler func(cmd []interface{}) (interface{}, string)) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "mpv-test-*")
	require.NoError(t, err)

	sockPath := filepath.Join(tmpDir, "mpv.sock")
	listener, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleMockConn(conn, handler)
		}
	}()

	cleanup := func() {
		_ = listener.Close()
		<-done
		_ = os.RemoveAll(tmpDir)
	}

	return sockPath, cleanup
}

func handleMockConn(conn net.Conn, handler func(cmd []interface{}) (interface{}, string)) {
	defer func() {
		_ = conn.Close()
	}()
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req struct {
			Command   []interface{} `json:"command"`
			RequestID uint64        `json:"request_id"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		if len(req.Command) > 0 && req.Command[0] == "trigger_event" {
			eventResp := map[string]interface{}{
				"event": "pause",
			}
			eventBytes, _ := json.Marshal(eventResp)
			eventBytes = append(eventBytes, '\n')
			_, _ = conn.Write(eventBytes)
		}

		data, errStr := handler(req.Command)
		resp := map[string]interface{}{
			"error":      errStr,
			"request_id": req.RequestID,
		}
		if data != nil {
			resp["data"] = data
		}

		respBytes, _ := json.Marshal(resp)
		respBytes = append(respBytes, '\n')
		_, _ = conn.Write(respBytes)
	}
}

func TestIPCClient_GetProperties(t *testing.T) {
	sockPath, cleanup := startMockMPVSocket(t, func(cmd []interface{}) (interface{}, string) {
		if len(cmd) >= 2 && cmd[0] == "get_property" {
			switch cmd[1] {
			case "time-pos":
				return 120.5, "success"
			case "percent-pos":
				return 45.2, "success"
			case "pause":
				return false, "success"
			case "invalid-prop":
				return nil, "property unavailable"
			case "bad-data":
				return "not-a-number", "success"
			}
		}
		return nil, "error"
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := player.DialIPC(ctx, sockPath, 2*time.Second)
	require.NoError(t, err)
	defer func() {
		_ = client.Close()
	}()

	var eventReceived bool
	client.OnEvent(func(event string, data json.RawMessage) {
		if event == "pause" {
			eventReceived = true
		}
	})

	_, _ = client.SendCommand(ctx, "trigger_event")
	time.Sleep(50 * time.Millisecond)
	assert.True(t, eventReceived)

	timePos, err := client.GetFloatProperty(ctx, "time-pos")
	require.NoError(t, err)
	assert.Equal(t, 120.5, timePos)

	percentPos, err := client.GetFloatProperty(ctx, "percent-pos")
	require.NoError(t, err)
	assert.Equal(t, 45.2, percentPos)

	paused, err := client.GetBoolProperty(ctx, "pause")
	require.NoError(t, err)
	assert.False(t, paused)

	_, err = client.GetFloatProperty(ctx, "invalid-prop")
	require.Error(t, err)
	assert.Equal(t, player.ErrPropertyUnavail, err)

	_, err = client.GetFloatProperty(ctx, "bad-data")
	require.Error(t, err)

	_, err = client.GetBoolProperty(ctx, "bad-data")
	require.Error(t, err)
}

func TestIPCClient_TimeoutAndClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := player.DialIPC(ctx, "/tmp/nonexistent-sock-path.sock", 100*time.Millisecond)
	require.Error(t, err)
}

func TestIPCClient_PropertyChangesReachHooksRegisteredBeforeObserving(t *testing.T) {
	fake := startFakeMPV(t, map[string]any{"time-pos": 12.5})
	client := dialFake(t, fake)

	type change struct{ name, data string }
	changes := make(chan change, 8)
	client.OnPropertyChange(func(name string, data json.RawMessage) {
		changes <- change{name, string(data)}
	})
	var mu sync.Mutex
	var events []string
	client.OnEvent(func(event string, _ json.RawMessage) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, client.ObserveProperty(ctx, "time-pos"))
	assert.Equal(t, change{"time-pos", "12.5"}, receive(t, changes), "observe_property's immediate value")

	fake.Emit("playback-restart")
	fake.Set("time-pos", 13.25)
	assert.Equal(t, change{"time-pos", "13.25"}, receive(t, changes), "a non-property event must not reach property hooks")

	fake.Unset("time-pos")
	assert.Equal(t, change{"time-pos", ""}, receive(t, changes), "an unavailable property arrives with no data")

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, events, "playback-restart")
}

func TestIPCClient_ObservePropertyReportsRejection(t *testing.T) {
	fake := startFakeMPV(t, nil)
	fake.RejectObserve()
	client := dialFake(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.Error(t, client.ObserveProperty(ctx, "time-pos"))
}

func TestIPCClient_DoneClosesWhenMpvHangsUp(t *testing.T) {
	fake := startFakeMPV(t, nil)
	client := dialFake(t, fake)

	// A round trip guarantees the fake has accepted the connection.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = client.GetFloatProperty(ctx, "time-pos")

	select {
	case <-client.Done():
		t.Fatal("Done closed before mpv hung up")
	default:
	}

	fake.Hangup()
	select {
	case <-client.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done did not close after mpv hung up")
	}
}
