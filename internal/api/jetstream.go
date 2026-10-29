package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/wankdanker/natsmin/internal/store"
)

var tagJS = []string{"JetStream"}

const jsBase = "/v1/systems/{system}/accounts/{account}/jetstream"

// StreamPath identifies a stream.
type StreamPath struct {
	AccountPath
	Stream string `path:"stream"`
}

// ConsumerPath identifies a consumer.
type ConsumerPath struct {
	StreamPath
	Consumer string `path:"consumer"`
}

// RawList is a list of native NATS JSON objects.
type RawList struct {
	Body []json.RawMessage
}

// MsgOut is a stored message.
type MsgOut struct {
	Seq     uint64              `json:"seq"`
	Subject string              `json:"subject"`
	Time    time.Time           `json:"time"`
	Headers map[string][]string `json:"headers,omitempty"`
	// Data is the payload, base64 encoded.
	Data string `json:"data"`
	// Text is the payload as UTF-8 when it is valid UTF-8.
	Text   *string `json:"text,omitempty"`
	Size   int     `json:"size"`
	IsJSON bool    `json:"isJson"`
}

// MsgPage is a page of messages.
type MsgPage struct {
	Messages []MsgOut `json:"messages"`
	// NextSeq is where the next page starts; 0 when the end was reached.
	NextSeq uint64 `json:"nextSeq"`
}

func (s *Server) js(ctx context.Context, p AccountPath) (jetstream.JetStream, error) {
	if _, err := s.store.GetAccount(ctx, p.System, p.Account); err != nil {
		return nil, apiErr(err)
	}
	js, err := s.mgr.JetStream(ctx, p.System, p.Account)
	return js, apiErr(err)
}

func (s *Server) conn(ctx context.Context, p AccountPath) (*nats.Conn, error) {
	if _, err := s.store.GetAccount(ctx, p.System, p.Account); err != nil {
		return nil, apiErr(err)
	}
	nc, err := s.mgr.Account(ctx, p.System, p.Account)
	return nc, apiErr(err)
}

// jsRequest sends a raw JetStream API request on an account connection.
func (s *Server) jsRequest(ctx context.Context, p AccountPath, subject string, req any) (json.RawMessage, error) {
	nc, err := s.conn(ctx, p)
	if err != nil {
		return nil, err
	}
	var body []byte
	if req != nil {
		if body, err = json.Marshal(req); err != nil {
			return nil, apiErr(err)
		}
	}
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	msg, err := nc.RequestWithContext(rctx, "$JS.API."+subject, body)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return nil, huma.NewError(http.StatusPreconditionFailed, "JetStream is not enabled for this account")
		}
		return nil, apiErr(err)
	}
	var resp struct {
		Error *jetstream.APIError `json:"error"`
	}
	if json.Unmarshal(msg.Data, &resp) == nil && resp.Error != nil {
		return nil, apiErr(resp.Error)
	}
	return msg.Data, nil
}

func (s *Server) registerJetStream() {
	reg(s, route{method: http.MethodGet, path: jsBase + "/info", id: "get-js-account-info", summary: "JetStream account usage and limits", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *AccountPath) (*RawBody, error) {
			js, err := s.js(ctx, *in)
			if err != nil {
				return nil, err
			}
			info, err := js.AccountInfo(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(info)}, nil
		})

	reg(s, route{method: http.MethodGet, path: jsBase + "/streams", id: "list-streams", summary: "List streams", desc: "Returns StreamInfo objects in NATS JSON format.", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			AccountPath
			Subject string `query:"subject" doc:"Only streams that capture this subject"`
		}) (*RawList, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			var opts []jetstream.StreamListOpt
			if in.Subject != "" {
				opts = append(opts, jetstream.WithStreamListSubject(in.Subject))
			}
			l := js.ListStreams(ctx, opts...)
			out := []json.RawMessage{}
			for info := range l.Info() {
				out = append(out, toRaw(info))
			}
			if err := l.Err(); err != nil {
				return nil, apiErr(err)
			}
			return &RawList{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams", id: "create-stream", summary: "Create a stream",
		desc: "Body is a StreamConfig in NATS JSON format.", tags: tagJS, role: store.RoleOperator, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			AccountPath
			Body json.RawMessage
		}) (*RawBody, error) {
			var cfg jetstream.StreamConfig
			if err := json.Unmarshal(in.Body, &cfg); err != nil {
				return nil, huma.Error422UnprocessableEntity("invalid stream config: " + err.Error())
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			st, err := js.CreateStream(ctx, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(st.CachedInfo())}, nil
		})

	reg(s, route{method: http.MethodGet, path: jsBase + "/streams/{stream}", id: "get-stream", summary: "Get a stream", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			StreamPath
			Subjects string `query:"subjects" doc:"Include per-subject message counts for subjects matching this filter (e.g. >)"`
			Deleted  bool   `query:"deleted" doc:"Include deleted sequence numbers"`
		}) (*RawBody, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			st, err := js.Stream(ctx, in.Stream)
			if err != nil {
				return nil, apiErr(err)
			}
			var opts []jetstream.StreamInfoOpt
			if in.Subjects != "" {
				opts = append(opts, jetstream.WithSubjectFilter(in.Subjects))
			}
			if in.Deleted {
				opts = append(opts, jetstream.WithDeletedDetails(true))
			}
			info, err := st.Info(ctx, opts...)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(info)}, nil
		})

	reg(s, route{method: http.MethodPut, path: jsBase + "/streams/{stream}", id: "update-stream", summary: "Update a stream",
		desc: "Body is the full StreamConfig. Changing placement moves the stream.", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			StreamPath
			Body json.RawMessage
		}) (*RawBody, error) {
			var cfg jetstream.StreamConfig
			if err := json.Unmarshal(in.Body, &cfg); err != nil {
				return nil, huma.Error422UnprocessableEntity("invalid stream config: " + err.Error())
			}
			if cfg.Name == "" {
				cfg.Name = in.Stream
			}
			if cfg.Name != in.Stream {
				return nil, huma.Error422UnprocessableEntity("stream name cannot change")
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			st, err := js.UpdateStream(ctx, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(st.CachedInfo())}, nil
		})

	reg(s, route{method: http.MethodDelete, path: jsBase + "/streams/{stream}", id: "delete-stream", summary: "Delete a stream", tags: tagJS, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *StreamPath) (*struct{}, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(js.DeleteStream(ctx, in.Stream))
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/purge", id: "purge-stream", summary: "Purge messages from a stream", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			StreamPath
			Body struct {
				Subject string `json:"subject,omitempty" doc:"Only purge this subject (wildcards allowed)"`
				Keep    uint64 `json:"keep,omitempty" doc:"Keep this many of the newest messages"`
				Seq     uint64 `json:"seq,omitempty" doc:"Purge up to, not including, this sequence"`
			}
		}) (*RawBody, error) {
			req := map[string]any{}
			if in.Body.Subject != "" {
				req["filter"] = in.Body.Subject
			}
			if in.Body.Keep > 0 {
				req["keep"] = in.Body.Keep
			}
			if in.Body.Seq > 0 {
				req["seq"] = in.Body.Seq
			}
			data, err := s.jsRequest(ctx, in.AccountPath, "STREAM.PURGE."+in.Stream, req)
			if err != nil {
				return nil, err
			}
			return &RawBody{data}, nil
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/stepdown", id: "stream-stepdown", summary: "Step down the stream leader", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			StreamPath
			Body *struct {
				Preferred string `json:"preferred,omitempty" doc:"Preferred server name"`
			}
		}) (*RawBody, error) {
			var req any
			if in.Body != nil && in.Body.Preferred != "" {
				req = map[string]any{"placement": map[string]any{"preferred": in.Body.Preferred}}
			}
			data, err := s.jsRequest(ctx, in.AccountPath, "STREAM.LEADER.STEPDOWN."+in.Stream, req)
			if err != nil {
				return nil, err
			}
			return &RawBody{data}, nil
		})

	reg(s, route{method: http.MethodGet, path: jsBase + "/streams/{stream}/messages", id: "list-messages", summary: "Browse messages",
		desc: "Reads messages without a consumer. Start from a sequence or a time, optionally filtered by subject.", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			StreamPath
			StartSeq  uint64    `query:"startSeq" doc:"First sequence to read (default: first in stream)"`
			StartTime time.Time `query:"startTime" doc:"Start at the first message at or after this time (RFC3339)"`
			Subject   string    `query:"subject" doc:"Subject filter, wildcards allowed"`
			Limit     int       `query:"limit" minimum:"1" maximum:"500" default:"50"`
		}) (*struct{ Body MsgPage }, error) {
			subj := in.Subject
			if subj == "" {
				subj = ">"
			}
			page := MsgPage{Messages: []MsgOut{}}
			seq := in.StartSeq
			if seq == 0 {
				seq = 1
			}
			first := true
			for len(page.Messages) < in.Limit {
				req := map[string]any{"seq": seq, "next_by_subj": subj}
				if first && !in.StartTime.IsZero() {
					req = map[string]any{"start_time": in.StartTime, "next_by_subj": subj}
				}
				first = false
				m, err := s.getMsg(ctx, in.AccountPath, in.Stream, req)
				if err != nil {
					var se huma.StatusError
					if errors.As(err, &se) && se.GetStatus() == http.StatusNotFound {
						seq = 0
						break
					}
					return nil, err
				}
				page.Messages = append(page.Messages, *m)
				seq = m.Seq + 1
			}
			page.NextSeq = seq
			return &struct{ Body MsgPage }{page}, nil
		})

	reg(s, route{method: http.MethodGet, path: jsBase + "/streams/{stream}/messages/{seq}", id: "get-message", summary: "Get a message by sequence", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			StreamPath
			Seq uint64 `path:"seq"`
		}) (*struct{ Body MsgOut }, error) {
			m, err := s.getMsg(ctx, in.AccountPath, in.Stream, map[string]any{"seq": in.Seq})
			if err != nil {
				return nil, err
			}
			return &struct{ Body MsgOut }{*m}, nil
		})

	reg(s, route{method: http.MethodGet, path: jsBase + "/streams/{stream}/last", id: "get-last-message", summary: "Get the last message for a subject", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			StreamPath
			Subject string `query:"subject" required:"true"`
		}) (*struct{ Body MsgOut }, error) {
			m, err := s.getMsg(ctx, in.AccountPath, in.Stream, map[string]any{"last_by_subj": in.Subject})
			if err != nil {
				return nil, err
			}
			return &struct{ Body MsgOut }{*m}, nil
		})

	reg(s, route{method: http.MethodDelete, path: jsBase + "/streams/{stream}/messages/{seq}", id: "delete-message", summary: "Delete a message", tags: tagJS, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			StreamPath
			Seq   uint64 `path:"seq"`
			Erase bool   `query:"erase" doc:"Overwrite the message data on disk"`
		}) (*struct{}, error) {
			req := map[string]any{"seq": in.Seq}
			if !in.Erase {
				req["no_erase"] = true
			}
			_, err := s.jsRequest(ctx, in.AccountPath, "STREAM.MSG.DELETE."+in.Stream, req)
			return nil, err
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/messages/{seq}/republish", id: "republish-message", summary: "Publish a stored message again",
		desc: "Publishes the message's data and headers to its original subject, or to another subject.", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			StreamPath
			Seq  uint64 `path:"seq"`
			Body *struct {
				Subject string `json:"subject,omitempty"`
			}
		}) (*struct{ Body PublishOut }, error) {
			m, err := s.getMsg(ctx, in.AccountPath, in.Stream, map[string]any{"seq": in.Seq})
			if err != nil {
				return nil, err
			}
			subject := m.Subject
			if in.Body != nil && in.Body.Subject != "" {
				subject = in.Body.Subject
			}
			data, _ := base64.StdEncoding.DecodeString(m.Data)
			hdr := nats.Header{}
			for k, v := range m.Headers {
				// Drop JetStream bookkeeping headers so the copy is stored fresh.
				if k == nats.MsgIdHdr || k == "Nats-Expected-Last-Sequence" {
					continue
				}
				hdr[k] = v
			}
			hdr.Set("Natsmin-Republished-From", fmt.Sprintf("%s:%d", in.Stream, in.Seq))
			out, err := s.publish(ctx, in.AccountPath, subject, data, hdr, true)
			if err != nil {
				return nil, err
			}
			return &struct{ Body PublishOut }{*out}, nil
		})

	// Consumers.
	reg(s, route{method: http.MethodGet, path: jsBase + "/streams/{stream}/consumers", id: "list-consumers", summary: "List consumers", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *StreamPath) (*RawList, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			st, err := js.Stream(ctx, in.Stream)
			if err != nil {
				return nil, apiErr(err)
			}
			l := st.ListConsumers(ctx)
			out := []json.RawMessage{}
			for info := range l.Info() {
				out = append(out, toRaw(info))
			}
			if err := l.Err(); err != nil {
				return nil, apiErr(err)
			}
			return &RawList{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/consumers", id: "create-consumer", summary: "Create or update a consumer",
		desc: "Body is a ConsumerConfig in NATS JSON format. A durable_name or name makes it durable.", tags: tagJS, role: store.RoleOperator, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			StreamPath
			Body json.RawMessage
		}) (*RawBody, error) {
			var cfg jetstream.ConsumerConfig
			if err := json.Unmarshal(in.Body, &cfg); err != nil {
				return nil, huma.Error422UnprocessableEntity("invalid consumer config: " + err.Error())
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			c, err := js.CreateOrUpdateConsumer(ctx, in.Stream, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(c.CachedInfo())}, nil
		})

	reg(s, route{method: http.MethodGet, path: jsBase + "/streams/{stream}/consumers/{consumer}", id: "get-consumer", summary: "Get a consumer", tags: tagJS, role: store.RoleViewer},
		func(ctx context.Context, in *ConsumerPath) (*RawBody, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			c, err := js.Consumer(ctx, in.Stream, in.Consumer)
			if err != nil {
				return nil, apiErr(err)
			}
			info, err := c.Info(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(info)}, nil
		})

	reg(s, route{method: http.MethodPut, path: jsBase + "/streams/{stream}/consumers/{consumer}", id: "update-consumer", summary: "Update a consumer", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			ConsumerPath
			Body json.RawMessage
		}) (*RawBody, error) {
			var cfg jetstream.ConsumerConfig
			if err := json.Unmarshal(in.Body, &cfg); err != nil {
				return nil, huma.Error422UnprocessableEntity("invalid consumer config: " + err.Error())
			}
			if cfg.Durable == "" && cfg.Name == "" {
				cfg.Name = in.Consumer
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			c, err := js.UpdateConsumer(ctx, in.Stream, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(c.CachedInfo())}, nil
		})

	reg(s, route{method: http.MethodDelete, path: jsBase + "/streams/{stream}/consumers/{consumer}", id: "delete-consumer", summary: "Delete a consumer", tags: tagJS, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *ConsumerPath) (*struct{}, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(js.DeleteConsumer(ctx, in.Stream, in.Consumer))
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/consumers/{consumer}/pause", id: "pause-consumer", summary: "Pause a consumer", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			ConsumerPath
			Body struct {
				Until time.Time `json:"until" doc:"Resume automatically at this time"`
			}
		}) (*RawBody, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			r, err := js.PauseConsumer(ctx, in.Stream, in.Consumer, in.Body.Until)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(r)}, nil
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/consumers/{consumer}/resume", id: "resume-consumer", summary: "Resume a paused consumer", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *ConsumerPath) (*RawBody, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			r, err := js.ResumeConsumer(ctx, in.Stream, in.Consumer)
			if err != nil {
				return nil, apiErr(err)
			}
			return &RawBody{toRaw(r)}, nil
		})

	reg(s, route{method: http.MethodPost, path: jsBase + "/streams/{stream}/consumers/{consumer}/stepdown", id: "consumer-stepdown", summary: "Step down the consumer leader", tags: tagJS, role: store.RoleOperator},
		func(ctx context.Context, in *ConsumerPath) (*RawBody, error) {
			data, err := s.jsRequest(ctx, in.AccountPath, fmt.Sprintf("CONSUMER.LEADER.STEPDOWN.%s.%s", in.Stream, in.Consumer), nil)
			if err != nil {
				return nil, err
			}
			return &RawBody{data}, nil
		})
}

func (s *Server) getMsg(ctx context.Context, p AccountPath, stream string, req map[string]any) (*MsgOut, error) {
	data, err := s.jsRequest(ctx, p, "STREAM.MSG.GET."+stream, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Message struct {
			Subject string    `json:"subject"`
			Seq     uint64    `json:"seq"`
			Hdrs    []byte    `json:"hdrs"`
			Data    []byte    `json:"data"`
			Time    time.Time `json:"time"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, apiErr(err)
	}
	m := resp.Message
	out := toMsgOut(m.Seq, m.Subject, m.Time, nil, m.Data)
	if len(m.Hdrs) > 0 {
		if h, err := nats.DecodeHeadersMsg(m.Hdrs); err == nil {
			out.Headers = h
		}
	}
	return &out, nil
}

func toMsgOut(seq uint64, subject string, t time.Time, hdr nats.Header, data []byte) MsgOut {
	m := MsgOut{Seq: seq, Subject: subject, Time: t, Data: base64.StdEncoding.EncodeToString(data), Size: len(data)}
	if len(hdr) > 0 {
		m.Headers = hdr
	}
	if utf8.Valid(data) {
		txt := string(data)
		m.Text = &txt
		m.IsJSON = len(data) > 0 && json.Valid(data)
	}
	return m
}
