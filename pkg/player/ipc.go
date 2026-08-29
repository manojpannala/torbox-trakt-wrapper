package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrIPCClosed       = errors.New("mpv ipc connection closed")
	ErrPropertyUnavail = errors.New("mpv property unavailable")
)

type ipcRequest struct {
	Command   []interface{} `json:"command"`
	RequestID uint64        `json:"request_id"`
}

type ipcResponse struct {
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data,omitempty"`
	RequestID uint64          `json:"request_id,omitempty"`
	Event     string          `json:"event,omitempty"`
}

type IPCClient struct {
	conn       net.Conn
	reader     *bufio.Reader
	mu         sync.Mutex
	reqSeq     uint64
	pending    map[uint64]chan ipcResponse
	closed     chan struct{}
	closeOnce  sync.Once
	eventHooks []func(event string, data json.RawMessage)
}

func DialIPC(ctx context.Context, socketPath string, maxWait time.Duration) (*IPCClient, error) {
	deadline := time.Now().Add(maxWait)
	var conn net.Conn
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		conn, err = net.Dial("unix", socketPath)
		if err == nil {
			break
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out connecting to mpv socket %s: %w", socketPath, err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	client := &IPCClient{
		conn:    conn,
		reader:  bufio.NewReader(conn),
		pending: make(map[uint64]chan ipcResponse),
		closed:  make(chan struct{}),
	}

	go client.readLoop()
	return client, nil
}

func (c *IPCClient) readLoop() {
	defer func() {
		_ = c.Close()
	}()

	for {
		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var resp ipcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		if resp.Event != "" {
			c.mu.Lock()
			hooks := make([]func(string, json.RawMessage), len(c.eventHooks))
			copy(hooks, c.eventHooks)
			c.mu.Unlock()
			for _, hook := range hooks {
				hook(resp.Event, resp.Data)
			}
			continue
		}

		if resp.RequestID > 0 {
			c.mu.Lock()
			ch, ok := c.pending[resp.RequestID]
			if ok {
				delete(c.pending, resp.RequestID)
			}
			c.mu.Unlock()

			if ok {
				ch <- resp
			}
		}
	}
}

func (c *IPCClient) SendCommand(ctx context.Context, cmd ...interface{}) (json.RawMessage, error) {
	reqID := atomic.AddUint64(&c.reqSeq, 1)
	req := ipcRequest{
		Command:   cmd,
		RequestID: reqID,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	respCh := make(chan ipcResponse, 1)
	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return nil, ErrIPCClosed
	default:
		c.pending[reqID] = respCh
	}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, reqID)
		c.mu.Unlock()
	}()

	if _, err := c.conn.Write(data); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, ErrIPCClosed
	case resp := <-respCh:
		if resp.Error != "success" {
			if resp.Error == "property unavailable" {
				return nil, ErrPropertyUnavail
			}
			return nil, fmt.Errorf("mpv error: %s", resp.Error)
		}
		return resp.Data, nil
	}
}

func (c *IPCClient) GetFloatProperty(ctx context.Context, prop string) (float64, error) {
	data, err := c.SendCommand(ctx, "get_property", prop)
	if err != nil {
		return 0, err
	}
	var val float64
	if err := json.Unmarshal(data, &val); err != nil {
		return 0, err
	}
	return val, nil
}

func (c *IPCClient) GetBoolProperty(ctx context.Context, prop string) (bool, error) {
	data, err := c.SendCommand(ctx, "get_property", prop)
	if err != nil {
		return false, err
	}
	var val bool
	if err := json.Unmarshal(data, &val); err != nil {
		return false, err
	}
	return val, nil
}

func (c *IPCClient) OnEvent(hook func(event string, data json.RawMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHooks = append(c.eventHooks, hook)
}

func (c *IPCClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.mu.Lock()
		for _, ch := range c.pending {
			close(ch)
		}
		c.pending = make(map[uint64]chan ipcResponse)
		c.mu.Unlock()
	})
	return nil
}
