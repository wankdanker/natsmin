package natsx

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// ServerInfo identifies the server that answered a system request.
type ServerInfo struct {
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	ID        string    `json:"id"`
	Cluster   string    `json:"cluster,omitempty"`
	Domain    string    `json:"domain,omitempty"`
	Version   string    `json:"ver"`
	Tags      []string  `json:"tags,omitempty"`
	JetStream bool      `json:"jetstream"`
	Seq       uint64    `json:"seq"`
	Time      time.Time `json:"time"`
}

// APIError is the error part of a system response.
type APIError struct {
	Code        int    `json:"code"`
	Description string `json:"description,omitempty"`
}

// APIResponse is one server's answer to a system request.
type APIResponse struct {
	Server ServerInfo      `json:"server"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  *APIError       `json:"error,omitempty"`
}

// SysClient issues requests on a connection bound to the system account.
type SysClient struct {
	nc *nats.Conn
	// Expected is the number of servers seen in the last full poll; ping
	// requests return early once that many have answered.
	Expected func() int
}

// NewSysClient wraps a system account connection.
func NewSysClient(nc *nats.Conn) *SysClient {
	return &SysClient{nc: nc}
}

// Conn returns the underlying connection.
func (c *SysClient) Conn() *nats.Conn { return c.nc }

// Ping sends a request to every server ($SYS.REQ.SERVER.PING.<kind>) and
// gathers the answers until timeout, or until answers stop arriving.
func (c *SysClient) Ping(ctx context.Context, kind string, opts any, timeout time.Duration) ([]APIResponse, error) {
	expected := 0
	if c.Expected != nil {
		expected = c.Expected()
	}
	return c.gather(ctx, "$SYS.REQ.SERVER.PING."+kind, opts, timeout, expected)
}

// PingAccount sends a request to $SYS.REQ.ACCOUNT.PING.<kind>.
func (c *SysClient) PingAccount(ctx context.Context, kind string, opts any, timeout time.Duration) ([]APIResponse, error) {
	return c.gather(ctx, "$SYS.REQ.ACCOUNT.PING."+kind, opts, timeout, 0)
}

// Server sends a request to one server ($SYS.REQ.SERVER.<id>.<kind>).
func (c *SysClient) Server(ctx context.Context, serverID, kind string, opts any, timeout time.Duration) (*APIResponse, error) {
	body, err := marshalOpts(opts)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	msg, err := c.nc.RequestWithContext(ctx, fmt.Sprintf("$SYS.REQ.SERVER.%s.%s", serverID, kind), body)
	if err != nil {
		return nil, err
	}
	var r APIResponse
	if err := json.Unmarshal(msg.Data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Request sends a raw request on the system connection and returns the reply.
func (c *SysClient) Request(ctx context.Context, subject string, body []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	msg, err := c.nc.RequestWithContext(ctx, subject, body)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

func marshalOpts(opts any) ([]byte, error) {
	if opts == nil {
		return nil, nil
	}
	if b, ok := opts.([]byte); ok {
		return b, nil
	}
	return json.Marshal(opts)
}

func (c *SysClient) gather(ctx context.Context, subject string, opts any, timeout time.Duration, expected int) ([]APIResponse, error) {
	body, err := marshalOpts(opts)
	if err != nil {
		return nil, err
	}
	inbox := c.nc.NewRespInbox()
	ch := make(chan *nats.Msg, 256)
	sub, err := c.nc.ChanSubscribe(inbox, ch)
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()
	if err := c.nc.PublishRequest(subject, inbox, body); err != nil {
		return nil, err
	}

	const idleGap = 300 * time.Millisecond
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var idle <-chan time.Time
	out := []APIResponse{}
	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-deadline.C:
			return out, nil
		case <-idle:
			return out, nil
		case m := <-ch:
			var r APIResponse
			if err := json.Unmarshal(m.Data, &r); err == nil {
				out = append(out, r)
			}
			if expected > 0 && len(out) >= expected {
				// Give stragglers (new servers) a brief chance to answer.
				idle = time.After(50 * time.Millisecond)
			} else {
				idle = time.After(idleGap)
			}
		}
	}
}
