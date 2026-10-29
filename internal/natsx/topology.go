package natsx

import (
	"encoding/json"
	"sort"
	"time"
)

// Raw shapes of the monitoring payloads natsmin reads. Only the fields used
// are declared; the raw endpoints pass full payloads through untouched.

type varz struct {
	ID               string    `json:"server_id"`
	Name             string    `json:"server_name"`
	Version          string    `json:"version"`
	Host             string    `json:"host"`
	Port             int       `json:"port"`
	Start            time.Time `json:"start"`
	Now              time.Time `json:"now"`
	Uptime           string    `json:"uptime"`
	Mem              int64     `json:"mem"`
	Cores            int       `json:"cores"`
	CPU              float64   `json:"cpu"`
	Connections      int       `json:"connections"`
	TotalConnections uint64    `json:"total_connections"`
	Routes           int       `json:"routes"`
	Remotes          int       `json:"remotes"`
	Leafnodes        int       `json:"leafnodes"`
	InMsgs           int64     `json:"in_msgs"`
	OutMsgs          int64     `json:"out_msgs"`
	InBytes          int64     `json:"in_bytes"`
	OutBytes         int64     `json:"out_bytes"`
	SlowConsumers    int64     `json:"slow_consumers"`
	Subscriptions    uint32    `json:"subscriptions"`
	MaxConnections   int       `json:"max_connections"`
	MaxPayload       int       `json:"max_payload"`
	ConfigLoadTime   time.Time `json:"config_load_time"`
	Tags             []string  `json:"tags"`
	SystemAccount    string    `json:"system_account"`
	Cluster          struct {
		Name string   `json:"name"`
		URLs []string `json:"urls"`
	} `json:"cluster"`
	Gateway struct {
		Name string `json:"name"`
	} `json:"gateway"`
	JetStream struct {
		Config *struct {
			MaxMemory int64  `json:"max_memory"`
			MaxStore  int64  `json:"max_storage"`
			Domain    string `json:"domain"`
		} `json:"config"`
	} `json:"jetstream"`
}

type connInfo struct {
	CID      uint64 `json:"cid"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	RTT      string `json:"rtt"`
	InMsgs   int64  `json:"in_msgs"`
	OutMsgs  int64  `json:"out_msgs"`
	InBytes  int64  `json:"in_bytes"`
	OutBytes int64  `json:"out_bytes"`
	Uptime   string `json:"uptime"`
}

type routez struct {
	ID     string `json:"server_id"`
	Routes []struct {
		RemoteID     string `json:"remote_id"`
		RemoteName   string `json:"remote_name"`
		DidSolicit   bool   `json:"did_solicit"`
		IsConfigured bool   `json:"is_configured"`
		IP           string `json:"ip"`
		Port         int    `json:"port"`
		RTT          string `json:"rtt"`
		PendingSize  int    `json:"pending_size"`
		InMsgs       int64  `json:"in_msgs"`
		OutMsgs      int64  `json:"out_msgs"`
		InBytes      int64  `json:"in_bytes"`
		OutBytes     int64  `json:"out_bytes"`
		Account      string `json:"account"`
		Compression  string `json:"compression"`
	} `json:"routes"`
}

type gatewayz struct {
	ID       string `json:"server_id"`
	Name     string `json:"name"`
	Outbound map[string]*struct {
		Configured bool      `json:"configured"`
		Connection *connInfo `json:"connection"`
	} `json:"outbound_gateways"`
	Inbound map[string][]*struct {
		Configured bool      `json:"configured"`
		Connection *connInfo `json:"connection"`
	} `json:"inbound_gateways"`
}

type leafz struct {
	ID    string `json:"server_id"`
	Leafs []struct {
		ID          uint64 `json:"id"`
		Name        string `json:"name"`
		IsSpoke     bool   `json:"is_spoke"`
		Account     string `json:"account"`
		IP          string `json:"ip"`
		Port        int    `json:"port"`
		RTT         string `json:"rtt"`
		InMsgs      int64  `json:"in_msgs"`
		OutMsgs     int64  `json:"out_msgs"`
		InBytes     int64  `json:"in_bytes"`
		OutBytes    int64  `json:"out_bytes"`
		Subs        uint32 `json:"subscriptions"`
		Compression string `json:"compression"`
	} `json:"leafs"`
}

type jsz struct {
	ID              string `json:"server_id"`
	Disabled        bool   `json:"disabled"`
	Memory          uint64 `json:"memory"`
	Store           uint64 `json:"storage"`
	ReservedMemory  uint64 `json:"reserved_memory"`
	ReservedStore   uint64 `json:"reserved_storage"`
	Accounts        int    `json:"accounts"`
	HAAssets        int    `json:"ha_assets"`
	Streams         int    `json:"streams"`
	StreamsLeader   int    `json:"streams_leader"`
	Consumers       int    `json:"consumers"`
	ConsumersLeader int    `json:"consumers_leader"`
	Messages        uint64 `json:"messages"`
	Bytes           uint64 `json:"bytes"`
	API             struct {
		Level  int    `json:"level"`
		Total  uint64 `json:"total"`
		Errors uint64 `json:"errors"`
	} `json:"api"`
	Config struct {
		MaxMemory int64  `json:"max_memory"`
		MaxStore  int64  `json:"max_storage"`
		Domain    string `json:"domain"`
	} `json:"config"`
	Meta *struct {
		Name     string `json:"name"`
		Leader   string `json:"leader"`
		Peer     string `json:"peer"`
		Size     int    `json:"cluster_size"`
		Pending  int    `json:"pending"`
		Replicas []struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
			Offline bool   `json:"offline"`
			Active  int64  `json:"active"`
			Lag     uint64 `json:"lag"`
			Peer    string `json:"peer"`
		} `json:"replicas"`
	} `json:"meta_cluster"`
}

// ServerNode is one server in a snapshot.
type ServerNode struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Cluster          string    `json:"cluster,omitempty"`
	Domain           string    `json:"domain,omitempty"`
	Host             string    `json:"host"`
	Port             int       `json:"port"`
	Version          string    `json:"version"`
	Tags             []string  `json:"tags,omitempty"`
	Start            time.Time `json:"start"`
	Uptime           string    `json:"uptime"`
	ConfigLoadTime   time.Time `json:"configLoadTime"`
	CPU              float64   `json:"cpu"`
	Mem              int64     `json:"mem"`
	Cores            int       `json:"cores"`
	Connections      int       `json:"connections"`
	MaxConnections   int       `json:"maxConnections"`
	TotalConnections uint64    `json:"totalConnections"`
	Subscriptions    uint32    `json:"subscriptions"`
	SlowConsumers    int64     `json:"slowConsumers"`
	Routes           int       `json:"routes"`
	Gateways         int       `json:"gateways"`
	Leafnodes        int       `json:"leafnodes"`
	InMsgs           int64     `json:"inMsgs"`
	OutMsgs          int64     `json:"outMsgs"`
	InBytes          int64     `json:"inBytes"`
	OutBytes         int64     `json:"outBytes"`
	InMsgsRate       float64   `json:"inMsgsRate"`
	OutMsgsRate      float64   `json:"outMsgsRate"`
	InBytesRate      float64   `json:"inBytesRate"`
	OutBytesRate     float64   `json:"outBytesRate"`
	Healthy          bool      `json:"healthy"`
	HealthError      string    `json:"healthError,omitempty"`
	LastSeen         time.Time `json:"lastSeen"`
	// Missing is true when the server answered an earlier poll but not the latest.
	Missing   bool             `json:"missing"`
	JetStream *ServerJetStream `json:"jetstream,omitempty"`
}

// ServerJetStream is the JetStream usage of one server.
type ServerJetStream struct {
	Memory          uint64 `json:"memory"`
	Storage         uint64 `json:"storage"`
	MaxMemory       int64  `json:"maxMemory"`
	MaxStorage      int64  `json:"maxStorage"`
	Streams         int    `json:"streams"`
	StreamsLeader   int    `json:"streamsLeader"`
	Consumers       int    `json:"consumers"`
	ConsumersLeader int    `json:"consumersLeader"`
	Messages        uint64 `json:"messages"`
	Bytes           uint64 `json:"bytes"`
	HAAssets        int    `json:"haAssets"`
	APITotal        uint64 `json:"apiTotal"`
	APIErrors       uint64 `json:"apiErrors"`
	APILevel        int    `json:"apiLevel"`
	MetaLeader      bool   `json:"metaLeader"`
}

// RouteEdge connects two servers in the same cluster.
type RouteEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	ToName      string `json:"toName"`
	Connections int    `json:"connections"`
	RTT         string `json:"rtt"`
	Pending     int    `json:"pending"`
	InMsgs      int64  `json:"inMsgs"`
	OutMsgs     int64  `json:"outMsgs"`
	InBytes     int64  `json:"inBytes"`
	OutBytes    int64  `json:"outBytes"`
	Compression string `json:"compression,omitempty"`
}

// GatewayEdge is an outbound gateway connection from a server to a remote cluster.
type GatewayEdge struct {
	From        string `json:"from"`
	FromCluster string `json:"fromCluster"`
	ToCluster   string `json:"toCluster"`
	ToServer    string `json:"toServer,omitempty"`
	Configured  bool   `json:"configured"`
	Connected   bool   `json:"connected"`
	RTT         string `json:"rtt,omitempty"`
	InMsgs      int64  `json:"inMsgs"`
	OutMsgs     int64  `json:"outMsgs"`
	InBytes     int64  `json:"inBytes"`
	OutBytes    int64  `json:"outBytes"`
}

// LeafEdge is a leafnode connection on a server.
type LeafEdge struct {
	Server      string `json:"server"`
	CID         uint64 `json:"cid"`
	Name        string `json:"name"`
	Account     string `json:"account"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	IsSpoke     bool   `json:"isSpoke"`
	RTT         string `json:"rtt"`
	Subs        uint32 `json:"subscriptions"`
	InMsgs      int64  `json:"inMsgs"`
	OutMsgs     int64  `json:"outMsgs"`
	InBytes     int64  `json:"inBytes"`
	OutBytes    int64  `json:"outBytes"`
	Compression string `json:"compression,omitempty"`
}

// ClusterSummary groups servers by cluster name.
type ClusterSummary struct {
	Name        string   `json:"name"`
	Servers     []string `json:"servers"`
	Connections int      `json:"connections"`
	Healthy     int      `json:"healthy"`
}

// MetaPeer is a member of the JetStream meta group.
type MetaPeer struct {
	Name    string `json:"name"`
	Peer    string `json:"peer"`
	Current bool   `json:"current"`
	Offline bool   `json:"offline"`
	Lag     uint64 `json:"lag"`
	Active  int64  `json:"activeNanos"`
	Leader  bool   `json:"leader"`
}

// JetStreamSummary describes JetStream across the system.
type JetStreamSummary struct {
	Enabled     bool       `json:"enabled"`
	MetaLeader  string     `json:"metaLeader"`
	MetaSize    int        `json:"metaSize"`
	MetaPending int        `json:"metaPending"`
	Peers       []MetaPeer `json:"peers"`
	Memory      uint64     `json:"memory"`
	Storage     uint64     `json:"storage"`
	MaxMemory   int64      `json:"maxMemory"`
	MaxStorage  int64      `json:"maxStorage"`
	// Streams and Consumers count assets led by each server, so replicas are
	// not double counted.
	Streams   int `json:"streams"`
	Consumers int `json:"consumers"`
	// Messages and Bytes are summed over servers and include replicas.
	Messages  uint64 `json:"messages"`
	Bytes     uint64 `json:"bytes"`
	APITotal  uint64 `json:"apiTotal"`
	APIErrors uint64 `json:"apiErrors"`
}

// Totals are system-wide counters.
type Totals struct {
	Servers       int      `json:"servers"`
	Clusters      int      `json:"clusters"`
	Connections   int      `json:"connections"`
	Subscriptions uint64   `json:"subscriptions"`
	SlowConsumers int64    `json:"slowConsumers"`
	Leafnodes     int      `json:"leafnodes"`
	InMsgs        int64    `json:"inMsgs"`
	OutMsgs       int64    `json:"outMsgs"`
	InBytes       int64    `json:"inBytes"`
	OutBytes      int64    `json:"outBytes"`
	InMsgsRate    float64  `json:"inMsgsRate"`
	OutMsgsRate   float64  `json:"outMsgsRate"`
	InBytesRate   float64  `json:"inBytesRate"`
	OutBytesRate  float64  `json:"outBytesRate"`
	Versions      []string `json:"versions"`
}

// Snapshot is the state of a system at one poll.
type Snapshot struct {
	SystemID  string            `json:"systemId"`
	Time      time.Time         `json:"time"`
	Connected bool              `json:"connected"`
	Error     string            `json:"error,omitempty"`
	Servers   []ServerNode      `json:"servers"`
	Clusters  []ClusterSummary  `json:"clusters"`
	Routes    []RouteEdge       `json:"routes"`
	Gateways  []GatewayEdge     `json:"gateways"`
	Leafnodes []LeafEdge        `json:"leafnodes"`
	Totals    Totals            `json:"totals"`
	JetStream *JetStreamSummary `json:"jetstream,omitempty"`
}

// RatePoint is one sample in a system's rate history.
type RatePoint struct {
	Time          time.Time `json:"t"`
	InMsgsRate    float64   `json:"inMsgs"`
	OutMsgsRate   float64   `json:"outMsgs"`
	InBytesRate   float64   `json:"inBytes"`
	OutBytesRate  float64   `json:"outBytes"`
	Connections   int       `json:"connections"`
	Subscriptions uint64    `json:"subscriptions"`
	Servers       int       `json:"servers"`
}

type pollResult struct {
	varz     []APIResponse
	routez   []APIResponse
	gatewayz []APIResponse
	leafz    []APIResponse
	jsz      []APIResponse
	healthz  []APIResponse
}

// buildSnapshot turns poll responses into a Snapshot, computing rates against
// the previous snapshot.
func buildSnapshot(systemID string, now time.Time, res pollResult, prev *Snapshot) *Snapshot {
	snap := &Snapshot{SystemID: systemID, Time: now, Connected: true, Totals: Totals{Versions: []string{}},
		Servers: []ServerNode{}, Clusters: []ClusterSummary{}, Routes: []RouteEdge{}, Gateways: []GatewayEdge{}, Leafnodes: []LeafEdge{}}
	prevByID := map[string]*ServerNode{}
	if prev != nil {
		for i := range prev.Servers {
			prevByID[prev.Servers[i].ID] = &prev.Servers[i]
		}
	}

	nodes := map[string]*ServerNode{}
	for _, r := range res.varz {
		if r.Error != nil {
			continue
		}
		var v varz
		if err := json.Unmarshal(r.Data, &v); err != nil {
			continue
		}
		n := &ServerNode{
			ID: v.ID, Name: v.Name, Cluster: r.Server.Cluster, Domain: r.Server.Domain, Host: v.Host, Port: v.Port,
			Version: v.Version, Tags: v.Tags, Start: v.Start, Uptime: v.Uptime, ConfigLoadTime: v.ConfigLoadTime,
			CPU: v.CPU, Mem: v.Mem, Cores: v.Cores, Connections: v.Connections, MaxConnections: v.MaxConnections,
			TotalConnections: v.TotalConnections, Subscriptions: v.Subscriptions, SlowConsumers: v.SlowConsumers,
			Routes: v.Routes, Gateways: v.Remotes, Leafnodes: v.Leafnodes,
			InMsgs: v.InMsgs, OutMsgs: v.OutMsgs, InBytes: v.InBytes, OutBytes: v.OutBytes,
			Healthy: true, LastSeen: now,
		}
		if n.Cluster == "" {
			n.Cluster = v.Cluster.Name
		}
		if p, ok := prevByID[n.ID]; ok && !p.Missing {
			dt := now.Sub(p.LastSeen).Seconds()
			if dt > 0 {
				n.InMsgsRate = rate(n.InMsgs, p.InMsgs, dt)
				n.OutMsgsRate = rate(n.OutMsgs, p.OutMsgs, dt)
				n.InBytesRate = rate(n.InBytes, p.InBytes, dt)
				n.OutBytesRate = rate(n.OutBytes, p.OutBytes, dt)
			}
		}
		nodes[n.ID] = n
	}

	for _, r := range res.healthz {
		n := nodes[r.Server.ID]
		if n == nil {
			continue
		}
		if r.Error != nil {
			n.Healthy = false
			n.HealthError = r.Error.Description
			continue
		}
		var h struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if json.Unmarshal(r.Data, &h) == nil && h.Status != "" && h.Status != "ok" {
			n.Healthy = false
			n.HealthError = h.Error
		}
	}

	var js *JetStreamSummary
	peerSeen := map[string]bool{}
	for _, r := range res.jsz {
		n := nodes[r.Server.ID]
		if n == nil || r.Error != nil {
			continue
		}
		var j jsz
		if err := json.Unmarshal(r.Data, &j); err != nil || j.Disabled {
			continue
		}
		n.JetStream = &ServerJetStream{
			Memory: j.Memory, Storage: j.Store, MaxMemory: j.Config.MaxMemory, MaxStorage: j.Config.MaxStore,
			Streams: j.Streams, StreamsLeader: j.StreamsLeader, Consumers: j.Consumers, ConsumersLeader: j.ConsumersLeader,
			Messages: j.Messages, Bytes: j.Bytes, HAAssets: j.HAAssets,
			APITotal: j.API.Total, APIErrors: j.API.Errors, APILevel: j.API.Level,
		}
		if js == nil {
			js = &JetStreamSummary{Enabled: true, Peers: []MetaPeer{}}
		}
		js.Memory += j.Memory
		js.Storage += j.Store
		js.MaxMemory += j.Config.MaxMemory
		js.MaxStorage += j.Config.MaxStore
		js.APITotal += j.API.Total
		js.APIErrors += j.API.Errors
		js.Messages += j.Messages
		js.Bytes += j.Bytes
		if j.Meta == nil {
			// Standalone server: it leads everything it holds.
			js.Streams += j.Streams
			js.Consumers += j.Consumers
			n.JetStream.MetaLeader = true
			js.MetaLeader = n.Name
			js.MetaSize = 1
			continue
		}
		js.Streams += j.StreamsLeader
		js.Consumers += j.ConsumersLeader
		if j.Meta.Leader == n.Name {
			n.JetStream.MetaLeader = true
		}
		if j.Meta.Leader != "" {
			js.MetaLeader = j.Meta.Leader
		}
		if j.Meta.Size > js.MetaSize {
			js.MetaSize = j.Meta.Size
		}
		if j.Meta.Leader == n.Name {
			js.MetaPending = j.Meta.Pending
			// The leader's replica list describes every other peer.
			js.Peers = append(js.Peers, MetaPeer{Name: n.Name, Peer: j.Meta.Peer, Current: true, Leader: true})
			peerSeen[n.Name] = true
			for _, p := range j.Meta.Replicas {
				if peerSeen[p.Name] {
					continue
				}
				peerSeen[p.Name] = true
				js.Peers = append(js.Peers, MetaPeer{Name: p.Name, Peer: p.Peer, Current: p.Current, Offline: p.Offline, Lag: p.Lag, Active: p.Active})
			}
		}
	}
	if js != nil {
		if js.Streams == 0 {
			// Servers older than 2.10 do not report leader counts; the largest
			// per-server count is a lower bound.
			for _, n := range nodes {
				if n.JetStream != nil {
					js.Streams = max(js.Streams, n.JetStream.Streams)
					js.Consumers = max(js.Consumers, n.JetStream.Consumers)
				}
			}
		}
		sort.Slice(js.Peers, func(i, k int) bool { return js.Peers[i].Name < js.Peers[k].Name })
		snap.JetStream = js
	}

	// Routes, aggregated per server pair (route pooling opens several).
	type pair struct{ a, b string }
	routeIdx := map[pair]int{}
	for _, r := range res.routez {
		if nodes[r.Server.ID] == nil || r.Error != nil {
			continue
		}
		var rz routez
		if err := json.Unmarshal(r.Data, &rz); err != nil {
			continue
		}
		for _, rt := range rz.Routes {
			k := pair{r.Server.ID, rt.RemoteID}
			i, ok := routeIdx[k]
			if !ok {
				snap.Routes = append(snap.Routes, RouteEdge{From: r.Server.ID, To: rt.RemoteID, ToName: rt.RemoteName, RTT: rt.RTT, Compression: rt.Compression})
				i = len(snap.Routes) - 1
				routeIdx[k] = i
			}
			e := &snap.Routes[i]
			e.Connections++
			e.Pending += rt.PendingSize
			e.InMsgs += rt.InMsgs
			e.OutMsgs += rt.OutMsgs
			e.InBytes += rt.InBytes
			e.OutBytes += rt.OutBytes
		}
	}

	for _, r := range res.gatewayz {
		n := nodes[r.Server.ID]
		if n == nil || r.Error != nil {
			continue
		}
		var gz gatewayz
		if err := json.Unmarshal(r.Data, &gz); err != nil {
			continue
		}
		for name, og := range gz.Outbound {
			e := GatewayEdge{From: n.ID, FromCluster: gz.Name, ToCluster: name, Configured: og.Configured}
			if og.Connection != nil {
				e.Connected = true
				e.ToServer = og.Connection.Name
				e.RTT = og.Connection.RTT
				e.InMsgs, e.OutMsgs = og.Connection.InMsgs, og.Connection.OutMsgs
				e.InBytes, e.OutBytes = og.Connection.InBytes, og.Connection.OutBytes
			}
			snap.Gateways = append(snap.Gateways, e)
		}
	}

	for _, r := range res.leafz {
		n := nodes[r.Server.ID]
		if n == nil || r.Error != nil {
			continue
		}
		var lz leafz
		if err := json.Unmarshal(r.Data, &lz); err != nil {
			continue
		}
		for _, l := range lz.Leafs {
			snap.Leafnodes = append(snap.Leafnodes, LeafEdge{Server: n.ID, CID: l.ID, Name: l.Name, Account: l.Account, IP: l.IP, Port: l.Port,
				IsSpoke: l.IsSpoke, RTT: l.RTT, Subs: l.Subs, InMsgs: l.InMsgs, OutMsgs: l.OutMsgs, InBytes: l.InBytes, OutBytes: l.OutBytes, Compression: l.Compression})
		}
	}

	// Carry forward servers that stopped answering so the UI can flag them.
	for id, p := range prevByID {
		if _, ok := nodes[id]; ok {
			continue
		}
		if now.Sub(p.LastSeen) > 10*time.Minute {
			continue
		}
		m := *p
		m.Missing = true
		m.Healthy = false
		m.HealthError = "server did not answer the last poll"
		m.InMsgsRate, m.OutMsgsRate, m.InBytesRate, m.OutBytesRate = 0, 0, 0, 0
		nodes[id] = &m
	}

	clusters := map[string]*ClusterSummary{}
	versions := map[string]bool{}
	for _, n := range nodes {
		snap.Servers = append(snap.Servers, *n)
		if n.Missing {
			continue
		}
		t := &snap.Totals
		t.Servers++
		t.Connections += n.Connections
		t.Subscriptions += uint64(n.Subscriptions)
		t.SlowConsumers += n.SlowConsumers
		t.Leafnodes += n.Leafnodes
		t.InMsgs += n.InMsgs
		t.OutMsgs += n.OutMsgs
		t.InBytes += n.InBytes
		t.OutBytes += n.OutBytes
		t.InMsgsRate += n.InMsgsRate
		t.OutMsgsRate += n.OutMsgsRate
		t.InBytesRate += n.InBytesRate
		t.OutBytesRate += n.OutBytesRate
		versions[n.Version] = true
		cname := n.Cluster
		if cname == "" {
			cname = n.Name
		}
		c := clusters[cname]
		if c == nil {
			c = &ClusterSummary{Name: cname}
			clusters[cname] = c
		}
		c.Servers = append(c.Servers, n.ID)
		c.Connections += n.Connections
		if n.Healthy {
			c.Healthy++
		}
	}
	for _, c := range clusters {
		sort.Strings(c.Servers)
		snap.Clusters = append(snap.Clusters, *c)
	}
	for v := range versions {
		snap.Totals.Versions = append(snap.Totals.Versions, v)
	}
	sort.Strings(snap.Totals.Versions)
	snap.Totals.Clusters = len(snap.Clusters)
	sort.Slice(snap.Servers, func(i, k int) bool {
		if snap.Servers[i].Cluster != snap.Servers[k].Cluster {
			return snap.Servers[i].Cluster < snap.Servers[k].Cluster
		}
		return snap.Servers[i].Name < snap.Servers[k].Name
	})
	sort.Slice(snap.Clusters, func(i, k int) bool { return snap.Clusters[i].Name < snap.Clusters[k].Name })
	return snap
}

func rate(cur, prev int64, dt float64) float64 {
	if cur < prev {
		// Counter reset (server restart).
		return 0
	}
	return float64(cur-prev) / dt
}
