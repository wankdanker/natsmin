package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/wankdanker/natsmin/internal/store"
)

var tagObj = []string{"Object store"}

// ObjConfigIn creates an object store bucket.
type ObjConfigIn struct {
	Bucket      string            `json:"bucket" minLength:"1" pattern:"^[a-zA-Z0-9_-]+$"`
	Description string            `json:"description,omitempty"`
	TTL         string            `json:"ttl,omitempty" doc:"Go duration; empty for no expiry"`
	MaxBytes    int64             `json:"maxBytes,omitempty"`
	Storage     string            `json:"storage,omitempty" enum:"file,memory" default:"file"`
	Replicas    int               `json:"replicas,omitempty" minimum:"0" maximum:"5"`
	Compression bool              `json:"compression,omitempty"`
	Placement   *PlacementIn      `json:"placement,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ObjStatusOut describes an object store bucket.
type ObjStatusOut struct {
	Bucket       string            `json:"bucket"`
	Description  string            `json:"description,omitempty"`
	TTL          string            `json:"ttl,omitempty"`
	Storage      string            `json:"storage"`
	Replicas     int               `json:"replicas"`
	Sealed       bool              `json:"sealed"`
	Size         uint64            `json:"size"`
	BackingStore string            `json:"backingStore"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ObjInfoOut describes one object.
type ObjInfoOut struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Size        uint64              `json:"size"`
	Chunks      uint32              `json:"chunks"`
	Modified    time.Time           `json:"modified"`
	Digest      string              `json:"digest"`
	Deleted     bool                `json:"deleted"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
	Link        string              `json:"link,omitempty" doc:"Target when the object is a link"`
}

func objStatusOut(st jetstream.ObjectStoreStatus) ObjStatusOut {
	o := ObjStatusOut{Bucket: st.Bucket(), Description: st.Description(), Storage: st.Storage().String(), Replicas: st.Replicas(),
		Sealed: st.Sealed(), Size: st.Size(), BackingStore: st.BackingStore(), Metadata: st.Metadata()}
	if st.TTL() > 0 {
		o.TTL = st.TTL().String()
	}
	return o
}

func objInfoOut(i *jetstream.ObjectInfo) ObjInfoOut {
	o := ObjInfoOut{Name: i.Name, Description: i.Description, Size: i.Size, Chunks: i.Chunks, Modified: i.ModTime, Digest: i.Digest,
		Deleted: i.Deleted, Headers: i.Headers, Metadata: i.Metadata}
	if i.Opts != nil && i.Opts.Link != nil {
		o.Link = i.Opts.Link.Bucket
		if i.Opts.Link.Name != "" {
			o.Link += "/" + i.Opts.Link.Name
		}
	}
	return o
}

// ObjBucketPath identifies an object store bucket.
type ObjBucketPath struct {
	AccountPath
	Bucket string `path:"bucket"`
}

func (s *Server) obj(ctx context.Context, p *ObjBucketPath) (jetstream.ObjectStore, error) {
	js, err := s.js(ctx, p.AccountPath)
	if err != nil {
		return nil, err
	}
	o, err := js.ObjectStore(ctx, p.Bucket)
	return o, apiErr(err)
}

func (s *Server) registerObjects() {
	base := "/v1/systems/{system}/accounts/{account}/objects"

	reg(s, route{method: http.MethodGet, path: base, id: "list-object-stores", summary: "List object store buckets", tags: tagObj, role: store.RoleViewer},
		func(ctx context.Context, in *AccountPath) (*struct{ Body []ObjStatusOut }, error) {
			js, err := s.js(ctx, *in)
			if err != nil {
				return nil, err
			}
			l := js.ObjectStores(ctx)
			out := []ObjStatusOut{}
			for st := range l.Status() {
				out = append(out, objStatusOut(st))
			}
			if err := l.Error(); err != nil {
				return nil, apiErr(err)
			}
			sort.Slice(out, func(i, k int) bool { return out[i].Bucket < out[k].Bucket })
			return &struct{ Body []ObjStatusOut }{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: base, id: "create-object-store", summary: "Create an object store bucket", tags: tagObj, role: store.RoleOperator, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			AccountPath
			Body ObjConfigIn
		}) (*struct{ Body ObjStatusOut }, error) {
			b := in.Body
			cfg := jetstream.ObjectStoreConfig{Bucket: b.Bucket, Description: b.Description, MaxBytes: b.MaxBytes, Replicas: b.Replicas, Compression: b.Compression, Metadata: b.Metadata}
			if b.TTL != "" {
				d, err := time.ParseDuration(b.TTL)
				if err != nil {
					return nil, huma.Error422UnprocessableEntity("ttl: " + err.Error())
				}
				cfg.TTL = d
			}
			if b.Storage == "memory" {
				cfg.Storage = jetstream.MemoryStorage
			}
			if b.Placement != nil && (b.Placement.Cluster != "" || len(b.Placement.Tags) > 0) {
				cfg.Placement = &jetstream.Placement{Cluster: b.Placement.Cluster, Tags: b.Placement.Tags}
			}
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			o, err := js.CreateObjectStore(ctx, cfg)
			if err != nil {
				return nil, apiErr(err)
			}
			st, err := o.Status(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body ObjStatusOut }{objStatusOut(st)}, nil
		})

	reg(s, route{method: http.MethodDelete, path: base + "/{bucket}", id: "delete-object-store", summary: "Delete an object store bucket", tags: tagObj, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *ObjBucketPath) (*struct{}, error) {
			js, err := s.js(ctx, in.AccountPath)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(js.DeleteObjectStore(ctx, in.Bucket))
		})

	reg(s, route{method: http.MethodGet, path: base + "/{bucket}", id: "list-objects", summary: "List objects", tags: tagObj, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ObjBucketPath
			Deleted bool `query:"deleted" doc:"Include deleted objects"`
		}) (*struct {
			Body struct {
				Status  ObjStatusOut `json:"status"`
				Objects []ObjInfoOut `json:"objects"`
			}
		}, error) {
			o, err := s.obj(ctx, &in.ObjBucketPath)
			if err != nil {
				return nil, err
			}
			st, err := o.Status(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			var opts []jetstream.ListObjectsOpt
			if in.Deleted {
				opts = append(opts, jetstream.ListObjectsShowDeleted())
			}
			list, err := o.List(ctx, opts...)
			if err != nil && err != jetstream.ErrNoObjectsFound {
				return nil, apiErr(err)
			}
			out := &struct {
				Body struct {
					Status  ObjStatusOut `json:"status"`
					Objects []ObjInfoOut `json:"objects"`
				}
			}{}
			out.Body.Status = objStatusOut(st)
			out.Body.Objects = []ObjInfoOut{}
			for _, i := range list {
				out.Body.Objects = append(out.Body.Objects, objInfoOut(i))
			}
			sort.Slice(out.Body.Objects, func(i, k int) bool { return out.Body.Objects[i].Name < out.Body.Objects[k].Name })
			return out, nil
		})

	reg(s, route{method: http.MethodGet, path: base + "/{bucket}/download", id: "download-object", summary: "Download an object", tags: tagObj, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ObjBucketPath
			Name string `query:"name" required:"true"`
		}) (*huma.StreamResponse, error) {
			o, err := s.obj(ctx, &in.ObjBucketPath)
			if err != nil {
				return nil, err
			}
			r, err := o.Get(ctx, in.Name)
			if err != nil {
				return nil, apiErr(err)
			}
			info, err := r.Info()
			if err != nil {
				r.Close()
				return nil, apiErr(err)
			}
			return &huma.StreamResponse{Body: func(hctx huma.Context) {
				defer r.Close()
				ct := mime.TypeByExtension(path.Ext(in.Name))
				if ct == "" {
					ct = "application/octet-stream"
				}
				hctx.SetHeader("Content-Type", ct)
				hctx.SetHeader("Content-Length", fmt.Sprint(info.Size))
				hctx.SetHeader("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(in.Name)}))
				_, _ = io.Copy(hctx.BodyWriter(), r)
			}}, nil
		})

	reg(s, route{method: http.MethodPut, path: base + "/{bucket}/upload", id: "upload-object", summary: "Upload an object",
		desc: "The request body is the object content (up to 256 MiB through this endpoint).", tags: tagObj, role: store.RoleOperator, maxBody: 256 << 20},
		func(ctx context.Context, in *struct {
			ObjBucketPath
			Name        string `query:"name" required:"true"`
			Description string `query:"description"`
			RawBody     []byte `contentType:"application/octet-stream"`
		}) (*struct{ Body ObjInfoOut }, error) {
			o, err := s.obj(ctx, &in.ObjBucketPath)
			if err != nil {
				return nil, err
			}
			info, err := o.Put(ctx, jetstream.ObjectMeta{Name: in.Name, Description: in.Description}, bytes.NewReader(in.RawBody))
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body ObjInfoOut }{objInfoOut(info)}, nil
		})

	reg(s, route{method: http.MethodDelete, path: base + "/{bucket}/object", id: "delete-object", summary: "Delete an object", tags: tagObj, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ObjBucketPath
			Name string `query:"name" required:"true"`
		}) (*struct{}, error) {
			o, err := s.obj(ctx, &in.ObjBucketPath)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(o.Delete(ctx, in.Name))
		})
}
