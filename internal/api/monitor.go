package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"

	"github.com/wankdanker/natsmin/internal/natsx"
	"github.com/wankdanker/natsmin/internal/store"
)

var tagMonitor = []string{"Monitoring"}

// MonitorResponse is one server's answer to a monitoring request.
type MonitorResponse struct {
	Server natsx.ServerInfo `json:"server"`
	Data   json.RawMessage  `json:"data,omitempty"`
	Error  *natsx.APIError  `json:"error,omitempty"`
}

// ConnOut is a client connection as seen by one server.
type ConnOut struct {
	Server         string     `json:"server"`
	ServerID       string     `json:"serverId"`
	Cluster        string     `json:"cluster,omitempty"`
	CID            uint64     `json:"cid"`
	Kind           string     `json:"kind,omitempty"`
	Type           string     `json:"type,omitempty"`
	IP             string     `json:"ip"`
	Port           int        `json:"port"`
	Start          time.Time  `json:"start"`
	LastActivity   time.Time  `json:"lastActivity"`
	Stop           *time.Time `json:"stop,omitempty"`
	Reason         string     `json:"reason,omitempty"`
	RTT            string     `json:"rtt,omitempty"`
	Uptime         string     `json:"uptime"`
	Idle           string     `json:"idle"`
	PendingBytes   int        `json:"pendingBytes"`
	InMsgs         int64      `json:"inMsgs"`
	OutMsgs        int64      `json:"outMsgs"`
	InBytes        int64      `json:"inBytes"`
	OutBytes       int64      `json:"outBytes"`
	Subscriptions  uint32     `json:"subscriptions"`
	Name           string     `json:"name,omitempty"`
	Lang           string     `json:"lang,omitempty"`
	Version        string     `json:"version,omitempty"`
	TLSVersion     string     `json:"tlsVersion,omitempty"`
	AuthorizedUser string     `json:"authorizedUser,omitempty"`
	Account        string     `json:"account,omitempty"`
	NameTag        string     `json:"nameTag,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	SubsList       []string   `json:"subscriptionsList,omitempty"`
}

type rawConn struct {
	CID            uint64     `json:"cid"`
	Kind           string     `json:"kind"`
	Type           string     `json:"type"`
	IP             string     `json:"ip"`
	Port           int        `json:"port"`
	Start          time.Time  `json:"start"`
	LastActivity   time.Time  `json:"last_activity"`
	Stop           *time.Time `json:"stop"`
	Reason         string     `json:"reason"`
	RTT            string     `json:"rtt"`
	Uptime         string     `json:"uptime"`
	Idle           string     `json:"idle"`
	PendingBytes   int        `json:"pending_bytes"`
	InMsgs         int64      `json:"in_msgs"`
	OutMsgs        int64      `json:"out_msgs"`
	InBytes        int64      `json:"in_bytes"`
	OutBytes       int64      `json:"out_bytes"`
	Subscriptions  uint32     `json:"subscriptions"`
	Name           string     `json:"name"`
	Lang           string     `json:"lang"`
	Version        string     `json:"version"`
	TLSVersion     string     `json:"tls_version"`
	AuthorizedUser string     `json:"authorized_user"`
	Account        string     `json:"account"`
	NameTag        string     `json:"name_tag"`
	Tags           []string   `json:"tags"`
	SubsList       []string   `json:"subscriptions_list"`
}

// ConnectionsOut is a page of connections across servers.
type ConnectionsOut struct {
	Total       int       `json:"total" doc:"Connections matching the filter across all servers"`
	Offset      int       `json:"offset"`
	Limit       int       `json:"limit"`
	Connections []ConnOut `json:"connections"`
	Servers     int       `json:"servers" doc:"Servers that answered"`
}

// AccountStatOut aggregates an account's traffic across servers.
type AccountStatOut struct {
	Account       string   `json:"account"`
	Name          string   `json:"name,omitempty"`
	Conns         int      `json:"conns"`
	Leafnodes     int      `json:"leafnodes"`
	TotalConns    int      `json:"totalConns"`
	Subscriptions uint64   `json:"subscriptions"`
	SentMsgs      int64    `json:"sentMsgs"`
	SentBytes     int64    `json:"sentBytes"`
	ReceivedMsgs  int64    `json:"receivedMsgs"`
	ReceivedBytes int64    `json:"receivedBytes"`
	SlowConsumers int64    `json:"slowConsumers"`
	Servers       []string `json:"servers"`
}

// InterestOut lists subscriptions on each server that match a subject.
type InterestOut struct {
	Subject string           `json:"subject"`
	Account string           `json:"account,omitempty"`
	Total   int              `json:"total"`
	Servers []ServerInterest `json:"servers"`
}

// ServerInterest is the matching subscriptions on one server.
type ServerInterest struct {
	Server        string   `json:"server"`
	Matches       int      `json:"matches"`
	Subscriptions []SubOut `json:"subscriptions"`
}

// SubOut is one subscription.
type SubOut struct {
	Account string `json:"account,omitempty"`
	Subject string `json:"subject"`
	Queue   string `json:"queue,omitempty"`
	SID     string `json:"sid"`
	Msgs    int64  `json:"msgs"`
	Max     int64  `json:"max,omitempty"`
	CID     uint64 `json:"cid"`
}

var monitorKinds = map[string]string{
	"varz": "VARZ", "connz": "CONNZ", "routez": "ROUTEZ", "gatewayz": "GATEWAYZ", "leafz": "LEAFZ",
	"subsz": "SUBSZ", "jsz": "JSZ", "accountz": "ACCOUNTZ", "healthz": "HEALTHZ", "raftz": "RAFTZ",
	"ipqueuesz": "IPQUEUESZ", "statsz": "STATSZ", "idz": "IDZ", "expvarz": "EXPVARZ",
}

func (s *Server) registerMonitor() {
	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/snapshot", id: "get-snapshot", summary: "Topology and stats snapshot",
		desc: "The latest poll of every server: health, stats, routes, gateways, leafnodes and the JetStream meta group.", tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			SystemPath
			Refresh bool `query:"refresh" doc:"Poll now instead of returning the cached snapshot"`
		}) (*struct{ Body *natsx.Snapshot }, error) {
			if _, err := s.store.GetSystem(ctx, in.System); err != nil {
				return nil, apiErr(err)
			}
			snap := s.mgr.Snapshot(in.System)
			if in.Refresh || snap == nil {
				var err error
				if snap, err = s.mgr.PollNow(ctx, in.System); err != nil {
					return nil, apiErr(err)
				}
			}
			return &struct{ Body *natsx.Snapshot }{snap}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/history", id: "get-history", summary: "Rate history",
		desc: "Message, byte and connection rates sampled at each poll, kept in memory.", tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *SystemPath) (*struct{ Body []natsx.RatePoint }, error) {
			return &struct{ Body []natsx.RatePoint }{s.mgr.History(in.System)}, nil
		})

	sse.Register(s.api, huma.Operation{
		OperationID: "stream-snapshots", Method: http.MethodGet, Path: "/v1/systems/{system}/events",
		Summary: "Live snapshots (SSE)", Description: "Server-sent events: a `snapshot` event after every poll. Requires role: **viewer**.",
		Tags: tagMonitor, Metadata: map[string]any{"role": store.RoleViewer},
	}, map[string]any{"snapshot": natsx.Snapshot{}}, func(ctx context.Context, in *SystemPath, send sse.Sender) {
		ch, unsub := s.mgr.Subscribe(in.System)
		defer unsub()
		if snap := s.mgr.Snapshot(in.System); snap != nil {
			if send.Data(snap) != nil {
				return
			}
		}
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case snap, ok := <-ch:
				if !ok {
					return
				}
				if send.Data(snap) != nil {
					return
				}
			case <-keepalive.C:
				if send.Comment("keepalive") != nil {
					return
				}
			}
		}
	})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/monitor/{kind}", id: "monitor-raw", summary: "Raw monitoring request",
		desc: "Sends a system request (VARZ, CONNZ, JSZ, ...) to every server, or to one with `server`, and returns the raw answers. `options` is a JSON object passed as the request body, as documented for the NATS monitoring endpoints.",
		tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			SystemPath
			Kind    string `path:"kind" enum:"varz,connz,routez,gatewayz,leafz,subsz,jsz,accountz,healthz,raftz,ipqueuesz,statsz,idz,expvarz"`
			Server  string `query:"server" doc:"Server id; omit to ask every server"`
			Options string `query:"options" doc:"JSON options, e.g. {\"accounts\":true}"`
		}) (*struct{ Body []MonitorResponse }, error) {
			cl, err := s.mgr.Sys(in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			var opts any
			if in.Options != "" {
				if !json.Valid([]byte(in.Options)) {
					return nil, huma.Error422UnprocessableEntity("options must be valid JSON")
				}
				opts = []byte(in.Options)
			}
			kind := monitorKinds[in.Kind]
			timeout := s.mgr.Options().RequestTimeout
			var res []natsx.APIResponse
			if in.Server != "" {
				r, err := cl.Server(ctx, in.Server, kind, opts, timeout)
				if err != nil {
					return nil, apiErr(err)
				}
				res = []natsx.APIResponse{*r}
			} else if res, err = cl.Ping(ctx, kind, opts, timeout); err != nil {
				return nil, apiErr(err)
			}
			out := make([]MonitorResponse, 0, len(res))
			for _, r := range res {
				out = append(out, MonitorResponse{Server: r.Server, Data: r.Data, Error: r.Error})
			}
			sort.Slice(out, func(i, k int) bool { return out[i].Server.Name < out[k].Server.Name })
			return &struct{ Body []MonitorResponse }{out}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/connections", id: "list-connections", summary: "Client connections",
		desc: "Connections across every server (CONNZ), merged and sorted.", tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			SystemPath
			Server  string `query:"server" doc:"Only this server (name, substring match)"`
			Cluster string `query:"cluster"`
			Account string `query:"account"`
			User    string `query:"user"`
			Subject string `query:"subject" doc:"Only connections with a subscription matching this subject"`
			CID     uint64 `query:"cid"`
			State   string `query:"state" enum:"open,closed,all" default:"open"`
			Sort    string `query:"sort" enum:"cid,start,subs,pending,msgs_to,msgs_from,bytes_to,bytes_from,last,uptime,stop" default:"cid"`
			Subs    bool   `query:"subs" doc:"Include each connection's subscriptions"`
			Offset  int    `query:"offset" minimum:"0" default:"0"`
			Limit   int    `query:"limit" minimum:"1" maximum:"1000" default:"100"`
		}) (*struct{ Body ConnectionsOut }, error) {
			cl, err := s.mgr.Sys(in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			state := map[string]int{"open": 0, "closed": 1, "all": 2}[in.State]
			opts := map[string]any{
				"sort": in.Sort, "offset": 0, "limit": in.Offset + in.Limit, "state": state, "auth": true,
				"subscriptions": in.Subs, "acc": in.Account, "user": in.User, "filter_subject": in.Subject, "cid": in.CID,
			}
			if in.Sort == "stop" && state == 0 {
				opts["sort"] = "cid"
			}
			if in.Server != "" {
				opts["server_name"] = in.Server
			}
			if in.Cluster != "" {
				opts["cluster"] = in.Cluster
				opts["exact_match"] = true
			}
			res, err := cl.Ping(ctx, "CONNZ", opts, s.mgr.Options().RequestTimeout)
			if err != nil {
				return nil, apiErr(err)
			}
			out := ConnectionsOut{Offset: in.Offset, Limit: in.Limit, Connections: []ConnOut{}}
			for _, r := range res {
				if r.Error != nil {
					continue
				}
				var cz struct {
					Total int       `json:"total"`
					Conns []rawConn `json:"connections"`
				}
				if json.Unmarshal(r.Data, &cz) != nil {
					continue
				}
				out.Servers++
				out.Total += cz.Total
				for _, c := range cz.Conns {
					out.Connections = append(out.Connections, ConnOut{
						Server: r.Server.Name, ServerID: r.Server.ID, Cluster: r.Server.Cluster,
						CID: c.CID, Kind: c.Kind, Type: c.Type, IP: c.IP, Port: c.Port, Start: c.Start, LastActivity: c.LastActivity,
						Stop: c.Stop, Reason: c.Reason, RTT: c.RTT, Uptime: c.Uptime, Idle: c.Idle, PendingBytes: c.PendingBytes,
						InMsgs: c.InMsgs, OutMsgs: c.OutMsgs, InBytes: c.InBytes, OutBytes: c.OutBytes, Subscriptions: c.Subscriptions,
						Name: c.Name, Lang: c.Lang, Version: c.Version, TLSVersion: c.TLSVersion, AuthorizedUser: c.AuthorizedUser,
						Account: c.Account, NameTag: c.NameTag, Tags: c.Tags, SubsList: c.SubsList,
					})
				}
			}
			sortConns(out.Connections, in.Sort)
			if in.Offset >= len(out.Connections) {
				out.Connections = []ConnOut{}
			} else {
				end := min(in.Offset+in.Limit, len(out.Connections))
				out.Connections = out.Connections[in.Offset:end]
			}
			return &struct{ Body ConnectionsOut }{out}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/account-stats", id: "list-account-stats", summary: "Per-account traffic",
		desc: "Connections, subscriptions and message counts per NATS account, summed across servers.", tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			SystemPath
			IncludeUnused bool `query:"includeUnused"`
		}) (*struct{ Body []AccountStatOut }, error) {
			cl, err := s.mgr.Sys(in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			res, err := cl.PingAccount(ctx, "STATZ", map[string]any{"accounts": []string{}, "include_unused": in.IncludeUnused}, s.mgr.Options().RequestTimeout)
			if err != nil {
				return nil, apiErr(err)
			}
			agg := map[string]*AccountStatOut{}
			for _, r := range res {
				if r.Error != nil {
					continue
				}
				var az struct {
					Accounts []struct {
						Account    string `json:"acc"`
						Name       string `json:"name"`
						Conns      int    `json:"conns"`
						Leafnodes  int    `json:"leafnodes"`
						TotalConns int    `json:"total_conns"`
						NumSubs    uint32 `json:"num_subscriptions"`
						Sent       struct {
							Msgs  int64 `json:"msgs"`
							Bytes int64 `json:"bytes"`
						} `json:"sent"`
						Received struct {
							Msgs  int64 `json:"msgs"`
							Bytes int64 `json:"bytes"`
						} `json:"received"`
						SlowConsumers int64 `json:"slow_consumers"`
					} `json:"account_statz"`
				}
				if json.Unmarshal(r.Data, &az) != nil {
					continue
				}
				for _, a := range az.Accounts {
					o := agg[a.Account]
					if o == nil {
						o = &AccountStatOut{Account: a.Account, Name: a.Name, Servers: []string{}}
						agg[a.Account] = o
					}
					o.Conns += a.Conns
					o.Leafnodes += a.Leafnodes
					o.TotalConns += a.TotalConns
					o.Subscriptions += uint64(a.NumSubs)
					o.SentMsgs += a.Sent.Msgs
					o.SentBytes += a.Sent.Bytes
					o.ReceivedMsgs += a.Received.Msgs
					o.ReceivedBytes += a.Received.Bytes
					o.SlowConsumers += a.SlowConsumers
					o.Servers = append(o.Servers, r.Server.Name)
				}
			}
			out := []AccountStatOut{}
			for _, o := range agg {
				sort.Strings(o.Servers)
				out = append(out, *o)
			}
			sort.Slice(out, func(i, k int) bool { return out[i].Account < out[k].Account })
			return &struct{ Body []AccountStatOut }{out}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/interest", id: "get-interest", summary: "Who is subscribed to a subject",
		desc: "Finds subscriptions matching a subject on every server (SUBSZ test).", tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			SystemPath
			Subject string `query:"subject" required:"true" minLength:"1"`
			Account string `query:"account" doc:"NATS account name or public key; omit for the global account"`
		}) (*struct{ Body InterestOut }, error) {
			cl, err := s.mgr.Sys(in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			opts := map[string]any{"subscriptions": true, "test": in.Subject, "limit": 1024, "account": in.Account}
			res, err := cl.Ping(ctx, "SUBSZ", opts, s.mgr.Options().RequestTimeout)
			if err != nil {
				return nil, apiErr(err)
			}
			out := InterestOut{Subject: in.Subject, Account: in.Account, Servers: []ServerInterest{}}
			for _, r := range res {
				if r.Error != nil {
					continue
				}
				var sz struct {
					Total int `json:"total"`
					Subs  []struct {
						Account string `json:"account"`
						Subject string `json:"subject"`
						Queue   string `json:"qgroup"`
						SID     string `json:"sid"`
						Msgs    int64  `json:"msgs"`
						Max     int64  `json:"max"`
						CID     uint64 `json:"cid"`
					} `json:"subscriptions_list"`
				}
				if json.Unmarshal(r.Data, &sz) != nil {
					continue
				}
				si := ServerInterest{Server: r.Server.Name, Matches: len(sz.Subs), Subscriptions: []SubOut{}}
				for _, sub := range sz.Subs {
					// Hide natsmin's own and other internal reply subscriptions.
					if strings.HasPrefix(sub.Subject, "_INBOX.") || strings.HasPrefix(sub.Subject, "_R_.") {
						si.Matches--
						continue
					}
					si.Subscriptions = append(si.Subscriptions, SubOut{Account: sub.Account, Subject: sub.Subject, Queue: sub.Queue, SID: sub.SID, Msgs: sub.Msgs, Max: sub.Max, CID: sub.CID})
				}
				out.Total += si.Matches
				out.Servers = append(out.Servers, si)
			}
			sort.Slice(out.Servers, func(i, k int) bool { return out.Servers[i].Server < out.Servers[k].Server })
			return &struct{ Body InterestOut }{out}, nil
		})
}

func sortConns(c []ConnOut, by string) {
	less := func(i, k int) bool { return c[i].CID < c[k].CID }
	switch by {
	case "start":
		less = func(i, k int) bool { return c[i].Start.Before(c[k].Start) }
	case "subs":
		less = func(i, k int) bool { return c[i].Subscriptions > c[k].Subscriptions }
	case "pending":
		less = func(i, k int) bool { return c[i].PendingBytes > c[k].PendingBytes }
	case "msgs_to":
		less = func(i, k int) bool { return c[i].OutMsgs > c[k].OutMsgs }
	case "msgs_from":
		less = func(i, k int) bool { return c[i].InMsgs > c[k].InMsgs }
	case "bytes_to":
		less = func(i, k int) bool { return c[i].OutBytes > c[k].OutBytes }
	case "bytes_from":
		less = func(i, k int) bool { return c[i].InBytes > c[k].InBytes }
	case "last":
		less = func(i, k int) bool { return c[i].LastActivity.After(c[k].LastActivity) }
	case "uptime":
		less = func(i, k int) bool { return c[i].Start.Before(c[k].Start) }
	case "stop":
		less = func(i, k int) bool {
			if c[i].Stop == nil || c[k].Stop == nil {
				return c[k].Stop == nil && c[i].Stop != nil
			}
			return c[i].Stop.After(*c[k].Stop)
		}
	}
	sort.SliceStable(c, less)
}
