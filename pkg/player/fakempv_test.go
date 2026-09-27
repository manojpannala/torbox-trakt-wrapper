package player_test

import (
	"bufio"
	"context"
	"encoding/json"
	"maps"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
)

type fakeMPV struct {
	sockPath string
	listener net.Listener

	mu            sync.Mutex
	conn          net.Conn
	props         map[string]any
	observed      map[string]int64
	rejectObserve bool
	requests      int
	gets          int
}

func startFakeMPV(t *testing.T, initial map[string]any) *fakeMPV {
	t.Helper()
	dir, err := os.MkdirTemp("", "fakempv-*")
	require.NoError(t, err)
	sockPath := filepath.Join(dir, "mpv.sock")
	listener, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	f := &fakeMPV{
		sockPath: sockPath,
		listener: listener,
		props:    map[string]any{},
		observed: map[string]int64{},
	}
	maps.Copy(f.props, initial)
	go f.serve()

	t.Cleanup(func() {
		_ = listener.Close()
		f.mu.Lock()
		if f.conn != nil {
			_ = f.conn.Close()
		}
		f.mu.Unlock()
		_ = os.RemoveAll(dir)
	})
	return f
}

func (f *fakeMPV) serve() {
	conn, err := f.listener.Accept()
	if err != nil {
		return
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var req struct {
			Command   []any  `json:"command"`
			RequestID uint64 `json:"request_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil || len(req.Command) == 0 {
			continue
		}
		f.handle(req.Command, req.RequestID)
	}
}

func (f *fakeMPV) handle(cmd []any, reqID uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++

	name, _ := cmd[0].(string)
	switch name {
	case "get_property":
		f.gets++
		prop, _ := cmd[1].(string)
		v, ok := f.props[prop]
		if !ok {
			f.writeLocked(map[string]any{"request_id": reqID, "error": "property unavailable"})
			return
		}
		f.writeLocked(map[string]any{"request_id": reqID, "error": "success", "data": v})
	case "observe_property":
		if f.rejectObserve {
			f.writeLocked(map[string]any{"request_id": reqID, "error": "invalid parameter"})
			return
		}
		id, _ := cmd[1].(float64)
		prop, _ := cmd[2].(string)
		f.observed[prop] = int64(id)
		f.writeLocked(map[string]any{"request_id": reqID, "error": "success"})
		f.pushChangeLocked(prop)
	default:
		f.writeLocked(map[string]any{"request_id": reqID, "error": "success"})
	}
}

func (f *fakeMPV) pushChangeLocked(prop string) {
	id, ok := f.observed[prop]
	if !ok {
		return
	}
	msg := map[string]any{"event": "property-change", "id": id, "name": prop}
	if v, ok := f.props[prop]; ok {
		msg["data"] = v
	}
	f.writeLocked(msg)
}

func (f *fakeMPV) writeLocked(msg map[string]any) {
	if f.conn == nil {
		return
	}
	b, _ := json.Marshal(msg)
	_, _ = f.conn.Write(append(b, '\n'))
}

func (f *fakeMPV) RejectObserve() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rejectObserve = true
}

func (f *fakeMPV) Set(prop string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.props[prop] = v
	f.pushChangeLocked(prop)
}

func (f *fakeMPV) Unset(prop string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.props, prop)
	f.pushChangeLocked(prop)
}

func (f *fakeMPV) Emit(event string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writeLocked(map[string]any{"event": event})
}

func (f *fakeMPV) Requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func (f *fakeMPV) Gets() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets
}

func dialFake(t *testing.T, f *fakeMPV) *player.IPCClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := player.DialIPC(ctx, f.sockPath, 2*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting on channel")
		var zero T
		return zero
	}
}
