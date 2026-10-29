package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/nats-io/nats.go"

	"github.com/wankdanker/natsmin/internal/store"
)

var tagMsgs = []string{"Messaging"}

// PublishIn is a message to send.
type PublishIn struct {
	Subject  string              `json:"subject" minLength:"1"`
	Data     string              `json:"data,omitempty"`
	Encoding string              `json:"encoding,omitempty" enum:"text,base64" default:"text"`
	Headers  map[string][]string `json:"headers,omitempty"`
}

// PublishOut reports a publish.
type PublishOut struct {
	Subject   string `json:"subject"`
	JetStream bool   `json:"jetstream"`
	Stream    string `json:"stream,omitempty"`
	Seq       uint64 `json:"seq,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
	Domain    string `json:"domain,omitempty"`
}

// TailMsg is a message seen by a live tail.
type TailMsg struct {
	MsgOut
	Reply string `json:"reply,omitempty"`
}

// TailDropped reports messages skipped because the client fell behind.
type TailDropped struct {
	Dropped int `json:"dropped"`
}

func decodePayload(data, enc string) ([]byte, error) {
	if enc == "base64" {
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity("data is not valid base64")
		}
		return b, nil
	}
	return []byte(data), nil
}

func (s *Server) publish(ctx context.Context, p AccountPath, subject string, data []byte, hdr nats.Header, useJS bool) (*PublishOut, error) {
	msg := &nats.Msg{Subject: subject, Data: data, Header: hdr}
	if useJS {
		js, err := s.js(ctx, p)
		if err != nil {
			return nil, err
		}
		ack, err := js.PublishMsg(ctx, msg)
		if err != nil {
			return nil, apiErr(err)
		}
		return &PublishOut{Subject: subject, JetStream: true, Stream: ack.Stream, Seq: ack.Sequence, Duplicate: ack.Duplicate, Domain: ack.Domain}, nil
	}
	nc, err := s.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	if err := nc.PublishMsg(msg); err != nil {
		return nil, apiErr(err)
	}
	if err := nc.FlushWithContext(ctx); err != nil {
		return nil, apiErr(err)
	}
	return &PublishOut{Subject: subject}, nil
}

func (s *Server) registerMessages() {
	base := "/v1/systems/{system}/accounts/{account}"

	reg(s, route{method: http.MethodPost, path: base + "/publish", id: "publish", summary: "Publish a message",
		desc: "Publishes on core NATS, or with jetstream=true waits for a stream to acknowledge it.", tags: tagMsgs, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			AccountPath
			Body struct {
				PublishIn
				JetStream bool `json:"jetstream,omitempty"`
			}
		}) (*struct{ Body PublishOut }, error) {
			data, err := decodePayload(in.Body.Data, in.Body.Encoding)
			if err != nil {
				return nil, err
			}
			var hdr nats.Header
			if len(in.Body.Headers) > 0 {
				hdr = nats.Header(in.Body.Headers)
			}
			out, err := s.publish(ctx, in.AccountPath, in.Body.Subject, data, hdr, in.Body.JetStream)
			if err != nil {
				return nil, err
			}
			return &struct{ Body PublishOut }{*out}, nil
		})

	reg(s, route{method: http.MethodPost, path: base + "/request", id: "request", summary: "Send a request and wait for the reply", tags: tagMsgs, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			AccountPath
			Body struct {
				PublishIn
				TimeoutMs int `json:"timeoutMs,omitempty" minimum:"1" maximum:"60000" default:"5000"`
			}
		}) (*struct{ Body TailMsg }, error) {
			data, err := decodePayload(in.Body.Data, in.Body.Encoding)
			if err != nil {
				return nil, err
			}
			nc, err := s.conn(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			timeout := time.Duration(in.Body.TimeoutMs) * time.Millisecond
			if timeout == 0 {
				timeout = 5 * time.Second
			}
			rctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			msg := &nats.Msg{Subject: in.Body.Subject, Data: data}
			if len(in.Body.Headers) > 0 {
				msg.Header = nats.Header(in.Body.Headers)
			}
			start := time.Now()
			reply, err := nc.RequestMsgWithContext(rctx, msg)
			if err != nil {
				return nil, apiErr(err)
			}
			out := TailMsg{MsgOut: toMsgOut(0, reply.Subject, start, reply.Header, reply.Data)}
			return &struct{ Body TailMsg }{out}, nil
		})

	sse.Register(s.api, huma.Operation{
		OperationID: "tail", Method: http.MethodGet, Path: base + "/tail",
		Summary:     "Live tail of a subject (SSE)",
		Description: "Subscribes to a subject and streams each message as a `message` event. If the client falls behind, messages are skipped and a `dropped` event reports how many. Requires role: **operator**.",
		Tags:        tagMsgs, Metadata: map[string]any{"role": store.RoleOperator},
	}, map[string]any{"message": TailMsg{}, "dropped": TailDropped{}}, func(ctx context.Context, in *struct {
		AccountPath
		Subject string `query:"subject" required:"true" doc:"Subject, wildcards allowed"`
		Queue   string `query:"queue" doc:"Join a queue group (takes messages away from other members)"`
	}, send sse.Sender) {
		nc, err := s.conn(ctx, in.AccountPath)
		if err != nil {
			_ = send(sse.Message{ID: 0, Data: map[string]string{"error": err.Error()}})
			return
		}
		ch := make(chan *nats.Msg, 512)
		var sub *nats.Subscription
		if in.Queue != "" {
			sub, err = nc.ChanQueueSubscribe(in.Subject, in.Queue, ch)
		} else {
			sub, err = nc.ChanSubscribe(in.Subject, ch)
		}
		if err != nil {
			_ = send(sse.Message{Data: map[string]string{"error": err.Error()}})
			return
		}
		defer sub.Unsubscribe()
		_ = sub.SetPendingLimits(10000, 64*1024*1024)
		s.audit(ctx, "tail", in.System+"/"+in.Account+"/"+in.Subject, "", nil)

		const maxPerSecond = 200
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		sent, dropped := 0, 0
		limit := time.NewTimer(time.Hour)
		defer limit.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-limit.C:
				return
			case <-ticker.C:
				sent = 0
				if dropped > 0 {
					if send.Data(TailDropped{Dropped: dropped}) != nil {
						return
					}
					dropped = 0
				}
			case <-keepalive.C:
				if send.Comment("keepalive") != nil {
					return
				}
			case m := <-ch:
				if sent >= maxPerSecond {
					dropped++
					continue
				}
				sent++
				// Hide our own request inboxes.
				if strings.HasPrefix(m.Subject, "_INBOX.") && in.Subject == ">" {
					continue
				}
				if send.Data(TailMsg{MsgOut: toMsgOut(0, m.Subject, time.Now(), m.Header, m.Data), Reply: m.Reply}) != nil {
					return
				}
			}
		}
	})
}
