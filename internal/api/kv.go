package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/wankdanker/natsmin/internal/store"
)

var tagKV = []string{"Key-Value"}

// PlacementIn pins a bucket or stream to a cluster or tags.
type PlacementIn struct {
	Cluster string   `json:"cluster,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// KVConfigIn creates or updates a bucket.
type KVConfigIn struct {
	Bucket       string            `json:"bucket" minLength:"1" pattern:"^[a-zA-Z0-9_-]+$"`
	Description  string            `json:"description,omitempty"`
	History      int               `json:"history,omitempty" minimum:"0" maximum:"64" doc:"Values kept per key (default 1)"`
	TTL          string            `json:"ttl,omitempty" doc:"Go duration, e.g. 24h; empty for no expiry"`
	MaxValueSize int32             `json:"maxValueSize,omitempty"`
	MaxBytes     int64             `json:"maxBytes,omitempty"`
	Storage      string            `json:"storage,omitempty" enum:"file,memory" default:"file"`
	Replicas     int               `json:"replicas,omitempty" minimum:"0" maximum:"5"`
	Compression  bool              `json:"compression,omitempty"`
	Placement    *PlacementIn      `json:"placement,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// KVStatusOut describes a bucket.
type KVStatusOut struct {
	Bucket       string            `json:"bucket"`
	Description  string            `json:"description,omitempty"`
	Values       uint64            `json:"values"`
	Bytes        uint64            `json:"bytes"`
	History      int64             `json:"history"`
	TTL          string            `json:"ttl,omitempty"`
	TTLNanos     int64             `json:"ttlNanos"`
	MaxValueSize int32             `json:"maxValueSize"`
	MaxBytes     int64             `json:"maxBytes"`
	Storage      string            `json:"storage"`
	Replicas     int               `json:"replicas"`
	Compression  bool              `json:"compression"`
	BackingStore string            `json:"backingStore"`
	Placement    *PlacementIn      `json:"placement,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// KVEntryOut is one value of a key.
type KVEntryOut struct {
	Key       string    `json:"key"`
	Revision  uint64    `json:"revision"`
	Created   time.Time `json:"created"`
	Delta     uint64    `json:"delta"`
	Operation string    `json:"operation" enum:"put,delete,purge"`
	Value     string    `json:"value" doc:"base64"`
	Text      *string   `json:"text,omitempty"`
	IsJSON    bool      `json:"isJson"`
	Size      int       `json:"size"`
}

func kvStatusOut(st jetstream.KeyValueStatus) KVStatusOut {
	cfg := st.Config()
	o := KVStatusOut{Bucket: st.Bucket(), Description: cfg.Description, Values: st.Values(), Bytes: st.Bytes(), History: st.History(),
		TTLNanos: int64(st.TTL()), MaxValueSize: cfg.MaxValueSize, MaxBytes: cfg.MaxBytes, Storage: cfg.Storage.String(),
		Replicas: cfg.Replicas, Compression: st.IsCompressed(), BackingStore: st.BackingStore(), Metadata: cfg.Metadata}
	if st.TTL() > 0 {
		o.TTL = st.TTL().String()
	}
	if cfg.Placement != nil {
		o.Placement = &PlacementIn{Cluster: cfg.Placement.Cluster, Tags: cfg.Placement.Tags}
	}
	return o
}

func kvEntryOut(e jetstream.KeyValueEntry) KVEntryOut {
	m := toMsgOut(0, "", time.Time{}, nil, e.Value())
	op := "put"
	switch e.Operation() {
	case jetstream.KeyValueDelete:
		op = "delete"
	case jetstream.KeyValuePurge:
		op = "purge"
	}
	return KVEntryOut{Key: e.Key(), Revision: e.Revision(), Created: e.Created(), Delta: e.Delta(), Operation: op,
		Value: m.Data, Text: m.Text, IsJSON: m.IsJSON, Size: m.Size}
}

func kvConfig(in KVConfigIn) (jetstream.KeyValueConfig, error) {
	cfg := jetstream.KeyValueConfig{Bucket: in.Bucket, Description: in.Description, MaxValueSize: in.MaxValueSize, MaxBytes: in.MaxBytes,
		Replicas: in.Replicas, Compression: in.Compression, Metadata: in.Metadata}
	if in.History > 0 {
		cfg.History = uint8(in.History)
	}
	if in.TTL != "" {
		d, err := time.ParseDuration(in.TTL)
		if err != nil {
			return cfg, huma.Error422UnprocessableEntity("ttl: " + err.Error())
		}
		cfg.TTL = d
	}
	if in.Storage == "memory" {
		cfg.Storage = jetstream.MemoryStorage
	}
	if in.Placement != nil && (in.Placement.Cluster != "" || len(in.Placement.Tags) > 0) {
		cfg.Placement = &jetstream.Placement{Cluster: in.Placement.Cluster, Tags: in.Placement.Tags}
	}
	return cfg, nil
}

// BucketPath identifies a KV bucket.
type BucketPath struct {
	AccountPath
	Bucket string `path:"bucket"`
}

func (s *Server) registerKV() {
	base := "/v1/systems/{system}/accounts/{account}/kv"

	reg(s, route{method: http.MethodGet, path: base, id: "list-kv", summary: "List KV buckets", tags: tagKV, role: store.RoleViewer},
		func(ctx context.Context, in *AccountPath) (*struct{ Body []KVStatusOut }, error) {
			js, err := s.js(ctx, *in)
			if err != nil {
				return nil, err
			}
			l := js.KeyValueStores(ctx)
			out := []KVStatusOut{}
			for st := range l.Status() {
				out = append(out, kvStatusOut(st))
			}
			if err := l.Error(); err != nil {
				return nil, apiErr(err)
			}
			sort.Slice(out, func(i, k int) bool { return out[i].Bucket < out[k].Bucket })
			return &struct{ Body []KVStatusOut }{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: base, id: "create-kv", summary: "Create a KV bucket", tags: tagKV, role: store.RoleOperator, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			AccountPath
			Body KVConfigIn
		}) (*struct{ Body KVStatusOut }, error) {
			cfg, err := kvConfig(in.Body)
			if err != nil {
				return nil, err
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			kv, err := js.CreateKeyValue(ctx, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			st, err := kv.Status(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body KVStatusOut }{kvStatusOut(st)}, nil
		})

	reg(s, route{method: http.MethodGet, path: base + "/{bucket}", id: "get-kv", summary: "Get a KV bucket", tags: tagKV, role: store.RoleViewer},
		func(ctx context.Context, in *BucketPath) (*struct{ Body KVStatusOut }, error) {
			kv, err := s.kv(ctx, in)
			if err != nil {
				return nil, err
			}
			st, err := kv.Status(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body KVStatusOut }{kvStatusOut(st)}, nil
		})

	reg(s, route{method: http.MethodPut, path: base + "/{bucket}", id: "update-kv", summary: "Update a KV bucket", tags: tagKV, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			BucketPath
			Body KVConfigIn
		}) (*struct{ Body KVStatusOut }, error) {
			in.Body.Bucket = in.Bucket
			cfg, err := kvConfig(in.Body)
			if err != nil {
				return nil, err
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			kv, err := js.UpdateKeyValue(ctx, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			st, err := kv.Status(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body KVStatusOut }{kvStatusOut(st)}, nil
		})

	reg(s, route{method: http.MethodDelete, path: base + "/{bucket}", id: "delete-kv", summary: "Delete a KV bucket", tags: tagKV, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *BucketPath) (*struct{}, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(js.DeleteKeyValue(ctx, in.Bucket))
		})

	reg(s, route{method: http.MethodGet, path: base + "/{bucket}/keys", id: "list-kv-keys", summary: "List keys", tags: tagKV, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			BucketPath
			Filter string `query:"filter" doc:"Key filter, wildcards allowed (default >)"`
			Limit  int    `query:"limit" minimum:"1" maximum:"100000" default:"10000"`
		}) (*struct {
			Body struct {
				Keys      []string `json:"keys"`
				Truncated bool     `json:"truncated"`
			}
		}, error) {
			kv, err := s.kv(ctx, &in.BucketPath)
			if err != nil {
				return nil, err
			}
			filter := in.Filter
			if filter == "" {
				filter = ">"
			}
			lister, err := kv.ListKeysFiltered(ctx, filter)
			if err != nil {
				return nil, apiErr(err)
			}
			out := &struct {
				Body struct {
					Keys      []string `json:"keys"`
					Truncated bool     `json:"truncated"`
				}
			}{}
			out.Body.Keys = []string{}
			for k := range lister.Keys() {
				if len(out.Body.Keys) >= in.Limit {
					out.Body.Truncated = true
					_ = lister.Stop()
					break
				}
				out.Body.Keys = append(out.Body.Keys, k)
			}
			sort.Strings(out.Body.Keys)
			return out, nil
		})

	reg(s, route{method: http.MethodGet, path: base + "/{bucket}/entry", id: "get-kv-entry", summary: "Get a key", tags: tagKV, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			BucketPath
			Key      string `query:"key" required:"true"`
			Revision uint64 `query:"revision"`
		}) (*struct{ Body KVEntryOut }, error) {
			kv, err := s.kv(ctx, &in.BucketPath)
			if err != nil {
				return nil, err
			}
			var e jetstream.KeyValueEntry
			if in.Revision > 0 {
				e, err = kv.GetRevision(ctx, in.Key, in.Revision)
			} else {
				e, err = kv.Get(ctx, in.Key)
			}
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body KVEntryOut }{kvEntryOut(e)}, nil
		})

	reg(s, route{method: http.MethodPut, path: base + "/{bucket}/entry", id: "put-kv-entry", summary: "Put a key", tags: tagKV, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			BucketPath
			Body struct {
				Key            string  `json:"key" minLength:"1"`
				Value          string  `json:"value"`
				Encoding       string  `json:"encoding,omitempty" enum:"text,base64" default:"text"`
				ExpectRevision *uint64 `json:"expectRevision,omitempty" doc:"Only write if the key is at this revision (0 = must not exist)"`
			}
		}) (*struct {
			Body struct {
				Revision uint64 `json:"revision"`
			}
		}, error) {
			kv, err := s.kv(ctx, &in.BucketPath)
			if err != nil {
				return nil, err
			}
			val, err := decodePayload(in.Body.Value, in.Body.Encoding)
			if err != nil {
				return nil, err
			}
			var rev uint64
			switch {
			case in.Body.ExpectRevision == nil:
				rev, err = kv.Put(ctx, in.Body.Key, val)
			case *in.Body.ExpectRevision == 0:
				rev, err = kv.Create(ctx, in.Body.Key, val)
			default:
				rev, err = kv.Update(ctx, in.Body.Key, val, *in.Body.ExpectRevision)
			}
			if err != nil {
				if errors.Is(err, jetstream.ErrKeyExists) {
					return nil, huma.Error409Conflict(err.Error())
				}
				var apiE *jetstream.APIError
				if errors.As(err, &apiE) && apiE.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
					return nil, huma.Error409Conflict("key was changed by someone else (revision mismatch)")
				}
				return nil, apiErr(err)
			}
			out := &struct {
				Body struct {
					Revision uint64 `json:"revision"`
				}
			}{}
			out.Body.Revision = rev
			return out, nil
		})

	reg(s, route{method: http.MethodDelete, path: base + "/{bucket}/entry", id: "delete-kv-entry", summary: "Delete a key", tags: tagKV, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			BucketPath
			Key   string `query:"key" required:"true"`
			Purge bool   `query:"purge" doc:"Also remove the key's history"`
		}) (*struct{}, error) {
			kv, err := s.kv(ctx, &in.BucketPath)
			if err != nil {
				return nil, err
			}
			if in.Purge {
				return nil, apiErr(kv.Purge(ctx, in.Key))
			}
			return nil, apiErr(kv.Delete(ctx, in.Key))
		})

	reg(s, route{method: http.MethodGet, path: base + "/{bucket}/history", id: "get-kv-history", summary: "History of a key", tags: tagKV, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			BucketPath
			Key string `query:"key" required:"true"`
		}) (*struct{ Body []KVEntryOut }, error) {
			kv, err := s.kv(ctx, &in.BucketPath)
			if err != nil {
				return nil, err
			}
			hist, err := kv.History(ctx, in.Key)
			if err != nil {
				return nil, apiErr(err)
			}
			out := []KVEntryOut{}
			for i := len(hist) - 1; i >= 0; i-- {
				out = append(out, kvEntryOut(hist[i]))
			}
			return &struct{ Body []KVEntryOut }{out}, nil
		})

	sse.Register(s.api, huma.Operation{
		OperationID: "watch-kv", Method: http.MethodGet, Path: base + "/{bucket}/watch",
		Summary: "Watch keys (SSE)", Description: "Streams an `entry` event for each change; with includeExisting, the current values come first. Requires role: **viewer**.",
		Tags: tagKV, Metadata: map[string]any{"role": store.RoleViewer},
	}, map[string]any{"entry": KVEntryOut{}}, func(ctx context.Context, in *struct {
		BucketPath
		Keys            string `query:"keys" doc:"Key filter (default >)"`
		IncludeExisting bool   `query:"includeExisting"`
	}, send sse.Sender) {
		kv, err := s.kv(ctx, &in.BucketPath)
		if err != nil {
			return
		}
		keys := in.Keys
		if keys == "" {
			keys = ">"
		}
		opts := []jetstream.WatchOpt{}
		if !in.IncludeExisting {
			opts = append(opts, jetstream.UpdatesOnly())
		}
		w, err := kv.Watch(ctx, keys, opts...)
		if err != nil {
			return
		}
		defer func() { _ = w.Stop() }()
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-keepalive.C:
				if send.Comment("keepalive") != nil {
					return
				}
			case e, ok := <-w.Updates():
				if !ok {
					return
				}
				// A nil entry marks the end of the initial values.
				if e == nil {
					continue
				}
				if send.Data(kvEntryOut(e)) != nil {
					return
				}
			}
		}
	})
}

func (s *Server) kv(ctx context.Context, in *BucketPath) (jetstream.KeyValue, error) {
	js, err := s.js(ctx, in.AccountPath)
	if err != nil {
		return nil, err
	}
	kv, err := js.KeyValue(ctx, in.Bucket)
	return kv, apiErr(err)
}
