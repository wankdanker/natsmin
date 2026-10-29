package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/natsmin/internal/store"
)

var tagMaint = []string{"Maintenance"}

// ReplicaOut is one replica of a stream.
type ReplicaOut struct {
	Server  string `json:"server"`
	Leader  bool   `json:"leader"`
	Current bool   `json:"current"`
	Offline bool   `json:"offline"`
	Lag     uint64 `json:"lag"`
	Active  int64  `json:"activeNanos"`
}

// StreamPlacement shows where a stream's replicas live.
type StreamPlacement struct {
	Account     string       `json:"account"`
	AccountName string       `json:"accountName,omitempty"`
	Stream      string       `json:"stream"`
	Cluster     string       `json:"cluster,omitempty"`
	Leader      string       `json:"leader,omitempty"`
	Replicas    []ReplicaOut `json:"replicas"`
	Configured  int          `json:"configuredReplicas"`
	Messages    uint64       `json:"messages"`
	Bytes       uint64       `json:"bytes"`
	Consumers   int          `json:"consumers"`
	Created     time.Time    `json:"created"`
	Storage     string       `json:"storage,omitempty"`
	Subjects    []string     `json:"subjects,omitempty"`
	Mirror      string       `json:"mirror,omitempty"`
	Sources     []string     `json:"sources,omitempty"`
	// Degraded is true when a replica is offline or not current.
	Degraded bool `json:"degraded"`
}

// JSAdminResult is the raw JetStream API answer.
type JSAdminResult struct {
	Body json.RawMessage
}

func (s *Server) registerMaintenance() {
	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/jetstream/assets", id: "list-stream-placement", summary: "Streams in every account and where their replicas are",
		desc: "Built from JSZ with stream details, from each stream's leader. Needs the system account only.", tags: tagMaint, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			SystemPath
			Account string `query:"account" doc:"Only this NATS account (name or public key)"`
		}) (*struct{ Body []StreamPlacement }, error) {
			cl, err := s.mgr.Sys(in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			opts := map[string]any{"accounts": true, "streams": true, "config": true, "stream_leader_only": true, "limit": 10000}
			if in.Account != "" {
				opts["account"] = in.Account
			}
			res, err := cl.Ping(ctx, "JSZ", opts, s.mgr.Options().RequestTimeout)
			if err != nil {
				return nil, apiErr(err)
			}
			seen := map[string]bool{}
			out := []StreamPlacement{}
			for _, r := range res {
				if r.Error != nil {
					continue
				}
				var j struct {
					Accounts []struct {
						Name    string `json:"name"`
						ID      string `json:"id"`
						Streams []struct {
							Name    string    `json:"name"`
							Created time.Time `json:"created"`
							Cluster *struct {
								Name     string `json:"name"`
								Leader   string `json:"leader"`
								Replicas []struct {
									Name    string `json:"name"`
									Current bool   `json:"current"`
									Offline bool   `json:"offline"`
									Active  int64  `json:"active"`
									Lag     uint64 `json:"lag"`
								} `json:"replicas"`
							} `json:"cluster"`
							Config *struct {
								Subjects []string `json:"subjects"`
								Storage  string   `json:"storage"`
								Replicas int      `json:"num_replicas"`
							} `json:"config"`
							State struct {
								Messages  uint64 `json:"messages"`
								Bytes     uint64 `json:"bytes"`
								Consumers int    `json:"consumer_count"`
							} `json:"state"`
							Mirror *struct {
								Name string `json:"name"`
							} `json:"mirror"`
							Sources []struct {
								Name string `json:"name"`
							} `json:"sources"`
						} `json:"stream_detail"`
					} `json:"account_details"`
				}
				if json.Unmarshal(r.Data, &j) != nil {
					continue
				}
				for _, a := range j.Accounts {
					for _, st := range a.Streams {
						key := a.ID + "/" + st.Name
						// Without leader-only support older servers report every replica.
						if seen[key] {
							continue
						}
						if st.Cluster != nil && st.Cluster.Leader != "" && st.Cluster.Leader != r.Server.Name {
							continue
						}
						seen[key] = true
						p := StreamPlacement{Account: a.ID, AccountName: a.Name, Stream: st.Name, Created: st.Created,
							Messages: st.State.Messages, Bytes: st.State.Bytes, Consumers: st.State.Consumers, Replicas: []ReplicaOut{}}
						if a.ID == "" {
							p.Account = a.Name
						}
						if st.Config != nil {
							p.Subjects, p.Storage, p.Configured = st.Config.Subjects, st.Config.Storage, st.Config.Replicas
						}
						if st.Mirror != nil {
							p.Mirror = st.Mirror.Name
						}
						for _, src := range st.Sources {
							p.Sources = append(p.Sources, src.Name)
						}
						if st.Cluster != nil {
							p.Cluster, p.Leader = st.Cluster.Name, st.Cluster.Leader
							if p.Leader != "" {
								p.Replicas = append(p.Replicas, ReplicaOut{Server: p.Leader, Leader: true, Current: true})
							}
							for _, rep := range st.Cluster.Replicas {
								p.Replicas = append(p.Replicas, ReplicaOut{Server: rep.Name, Current: rep.Current, Offline: rep.Offline, Lag: rep.Lag, Active: rep.Active})
								if rep.Offline || !rep.Current {
									p.Degraded = true
								}
							}
						} else {
							p.Leader = r.Server.Name
							p.Replicas = append(p.Replicas, ReplicaOut{Server: r.Server.Name, Leader: true, Current: true})
						}
						if p.Configured > 0 && len(p.Replicas) < p.Configured {
							p.Degraded = true
						}
						out = append(out, p)
					}
				}
			}
			sort.Slice(out, func(i, k int) bool {
				if out[i].AccountName != out[k].AccountName {
					return out[i].AccountName < out[k].AccountName
				}
				return out[i].Stream < out[k].Stream
			})
			return &struct{ Body []StreamPlacement }{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/jetstream/meta/stepdown", id: "meta-stepdown", summary: "Step down the JetStream meta leader",
		desc: "Asks the meta leader to hand over leadership, optionally to a preferred server, cluster or tags.", tags: tagMaint, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			SystemPath
			Body struct {
				Preferred string   `json:"preferred,omitempty" doc:"Preferred server name"`
				Cluster   string   `json:"cluster,omitempty"`
				Tags      []string `json:"tags,omitempty"`
			}
		}) (*JSAdminResult, error) {
			req := map[string]any{}
			if in.Body.Preferred != "" || in.Body.Cluster != "" || len(in.Body.Tags) > 0 {
				req["placement"] = map[string]any{"preferred": in.Body.Preferred, "cluster": in.Body.Cluster, "tags": in.Body.Tags}
			}
			return s.sysJSRequest(ctx, in.System, "$JS.API.META.LEADER.STEPDOWN", req)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/jetstream/peers/remove", id: "remove-peer", summary: "Remove a server from the JetStream meta group",
		desc: "Use after a server is permanently gone. Its stream and consumer replicas are re-placed on other servers.", tags: tagMaint, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			SystemPath
			Body struct {
				Server string `json:"server" doc:"Server name"`
				PeerID string `json:"peerId,omitempty" doc:"Peer id; takes precedence over server"`
			}
		}) (*JSAdminResult, error) {
			if in.Body.Server == "" && in.Body.PeerID == "" {
				return nil, huma.Error422UnprocessableEntity("server or peerId is required")
			}
			return s.sysJSRequest(ctx, in.System, "$JS.API.SERVER.REMOVE", map[string]any{"peer": in.Body.Server, "peer_id": in.Body.PeerID})
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/jetstream/streams/move", id: "move-stream", summary: "Move a stream's replica off a server",
		desc: "Evacuates one stream (in any account) from a server, optionally into a cluster or tag set. Repeat per stream to drain a server.", tags: tagMaint, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			SystemPath
			Body struct {
				Account string   `json:"account" minLength:"1" doc:"NATS account name or public key"`
				Stream  string   `json:"stream" minLength:"1"`
				Server  string   `json:"server,omitempty" doc:"Server to move off"`
				Cluster string   `json:"cluster,omitempty" doc:"Target cluster"`
				Domain  string   `json:"domain,omitempty"`
				Tags    []string `json:"tags,omitempty"`
				Cancel  bool     `json:"cancel,omitempty" doc:"Cancel a move in progress instead"`
			}
		}) (*JSAdminResult, error) {
			b := in.Body
			if b.Cancel {
				return s.sysJSRequest(ctx, in.System, fmt.Sprintf("$JS.API.ACCOUNT.STREAM.CANCEL_MOVE.%s.%s", b.Account, b.Stream), nil)
			}
			return s.sysJSRequest(ctx, in.System, fmt.Sprintf("$JS.API.ACCOUNT.STREAM.MOVE.%s.%s", b.Account, b.Stream),
				map[string]any{"server": b.Server, "cluster": b.Cluster, "domain": b.Domain, "tags": b.Tags})
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/servers/{server}/reload", id: "reload-server", summary: "Reload a server's configuration",
		tags: tagMaint, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			SystemPath
			Server string `path:"server" doc:"Server id"`
		}) (*struct{ Body MonitorResponse }, error) {
			return s.serverCommand(ctx, in.System, in.Server, "RELOAD", nil)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/servers/{server}/connections/{cid}/kick", id: "kick-connection", summary: "Disconnect a client",
		tags: tagMaint, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			SystemPath
			Server string `path:"server" doc:"Server id"`
			CID    uint64 `path:"cid"`
		}) (*struct{ Body MonitorResponse }, error) {
			return s.serverCommand(ctx, in.System, in.Server, "KICK", map[string]any{"cid": in.CID})
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/servers/{server}/connections/{cid}/lame-duck", id: "lame-duck-connection", summary: "Ask a client to reconnect elsewhere",
		desc: "Sends the client a lame duck notice so it moves to another server.", tags: tagMaint, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			SystemPath
			Server string `path:"server" doc:"Server id"`
			CID    uint64 `path:"cid"`
		}) (*struct{ Body MonitorResponse }, error) {
			return s.serverCommand(ctx, in.System, in.Server, "LDM", map[string]any{"cid": in.CID})
		})
}

func (s *Server) serverCommand(ctx context.Context, system, server, kind string, opts any) (*struct{ Body MonitorResponse }, error) {
	cl, err := s.mgr.Sys(system)
	if err != nil {
		return nil, apiErr(err)
	}
	r, err := cl.Server(ctx, server, kind, opts, s.mgr.Options().RequestTimeout)
	if err != nil {
		return nil, apiErr(err)
	}
	if r.Error != nil {
		return nil, huma.NewError(http.StatusBadGateway, r.Error.Description)
	}
	return &struct{ Body MonitorResponse }{MonitorResponse{Server: r.Server, Data: r.Data}}, nil
}

func (s *Server) sysJSRequest(ctx context.Context, system, subject string, req any) (*JSAdminResult, error) {
	cl, err := s.mgr.Sys(system)
	if err != nil {
		return nil, apiErr(err)
	}
	var body []byte
	if req != nil {
		if body, err = json.Marshal(req); err != nil {
			return nil, apiErr(err)
		}
	}
	data, err := cl.Request(ctx, subject, body, 10*time.Second)
	if err != nil {
		return nil, apiErr(err)
	}
	var resp struct {
		Error *struct {
			Code        int    `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &resp) == nil && resp.Error != nil {
		code := resp.Error.Code
		if code < 400 || code > 599 {
			code = http.StatusBadRequest
		}
		return nil, huma.NewError(code, strings.TrimSpace(resp.Error.Description))
	}
	return &JSAdminResult{Body: data}, nil
}
