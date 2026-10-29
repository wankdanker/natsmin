// Package devcluster starts a NATS supercluster in-process for development
// and integration tests: two 3-node clusters joined by gateways, plus a
// leafnode server attached to the first cluster.
package devcluster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// Config controls the topology.
type Config struct {
	// Dir holds config files and JetStream storage. Required.
	Dir string
	// BasePort is the first client port; other ports are offset from it.
	BasePort int
	// Clusters are the cluster names; each gets ServersPerCluster servers.
	Clusters          []string
	ServersPerCluster int
	// Leaf adds a standalone leafnode server (JetStream domain "edge").
	Leaf bool
	// Monitoring enables the HTTP monitoring port of each server.
	Monitoring bool
}

// Default is two clusters of three plus a leaf.
func Default(dir string) Config {
	return Config{Dir: dir, BasePort: 4222, Clusters: []string{"east", "west"}, ServersPerCluster: 3, Leaf: true, Monitoring: true}
}

// Credentials used by the dev cluster.
const (
	SysUser  = "sys"
	SysPass  = "sys"
	AppUser  = "app"
	AppPass  = "app"
	EdgeUser = "edge"
	EdgePass = "edge"
)

// Node is a running server.
type Node struct {
	Name       string
	Cluster    string
	ClientURL  string
	MonitorURL string
	Server     *server.Server
}

// Supercluster is the set of running servers.
type Supercluster struct {
	Nodes []*Node
	Leaf  *Node
}

// URLs returns the client URLs of the cluster servers (not the leaf).
func (s *Supercluster) URLs() []string {
	out := []string{}
	for _, n := range s.Nodes {
		out = append(out, n.ClientURL)
	}
	return out
}

// Shutdown stops every server.
func (s *Supercluster) Shutdown() {
	all := append([]*Node{}, s.Nodes...)
	if s.Leaf != nil {
		all = append(all, s.Leaf)
	}
	for _, n := range all {
		n.Server.Shutdown()
	}
	for _, n := range all {
		n.Server.WaitForShutdown()
	}
}

const accountsBlock = `
accounts {
  SYS { users [ { user: %q, password: %q } ] }
  APP {
    jetstream: enabled
    users [ { user: %q, password: %q } ]
  }
}
system_account: SYS
`

// Start writes the configs and boots every server, waiting until the
// supercluster has formed.
func Start(cfg Config) (*Supercluster, error) {
	if cfg.ServersPerCluster <= 0 {
		cfg.ServersPerCluster = 3
	}
	if len(cfg.Clusters) == 0 {
		cfg.Clusters = []string{"east"}
	}
	if cfg.BasePort == 0 {
		cfg.BasePort = 4222
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	total := len(cfg.Clusters) * cfg.ServersPerCluster
	// Port layout: client base+i, cluster base+2000+i, gateway base+3000+i,
	// leafnode base+3200+i, monitor base+4000+i.
	port := func(kind, i int) int { return cfg.BasePort + kind + i }
	const (
		pClient  = 0
		pCluster = 2000
		pGateway = 3000
		pLeaf    = 3200
		pMonitor = 4000
	)

	gatewayURLs := map[string][]string{}
	for ci, cname := range cfg.Clusters {
		for k := 0; k < cfg.ServersPerCluster; k++ {
			i := ci*cfg.ServersPerCluster + k
			gatewayURLs[cname] = append(gatewayURLs[cname], fmt.Sprintf("nats://127.0.0.1:%d", port(pGateway, i)))
		}
	}

	sc := &Supercluster{}
	for ci, cname := range cfg.Clusters {
		var routes []string
		for k := 0; k < cfg.ServersPerCluster; k++ {
			routes = append(routes, fmt.Sprintf("nats://127.0.0.1:%d", port(pCluster, ci*cfg.ServersPerCluster+k)))
		}
		for k := 0; k < cfg.ServersPerCluster; k++ {
			i := ci*cfg.ServersPerCluster + k
			name := fmt.Sprintf("%s-%d", cname, k+1)
			var b strings.Builder
			fmt.Fprintf(&b, "server_name: %q\nlisten: \"127.0.0.1:%d\"\n", name, port(pClient, i))
			if cfg.Monitoring {
				fmt.Fprintf(&b, "http: \"127.0.0.1:%d\"\n", port(pMonitor, i))
			}
			fmt.Fprintf(&b, "server_tags: [\"region:%s\", \"az:%d\"]\n", cname, k+1)
			fmt.Fprintf(&b, "jetstream { store_dir: %q, max_memory_store: 256MB, max_file_store: 2GB }\n", filepath.Join(cfg.Dir, name, "js"))
			fmt.Fprintf(&b, accountsBlock, SysUser, SysPass, AppUser, AppPass)
			if total > 1 {
				fmt.Fprintf(&b, "cluster { name: %q, listen: \"127.0.0.1:%d\", routes: [%s] }\n", cname, port(pCluster, i), quoteList(routes))
			}
			if len(cfg.Clusters) > 1 {
				var gws []string
				for _, other := range cfg.Clusters {
					gws = append(gws, fmt.Sprintf("{ name: %q, urls: [%s] }", other, quoteList(gatewayURLs[other])))
				}
				fmt.Fprintf(&b, "gateway { name: %q, listen: \"127.0.0.1:%d\", gateways: [%s] }\n", cname, port(pGateway, i), strings.Join(gws, ", "))
			}
			if cfg.Leaf && ci == 0 {
				fmt.Fprintf(&b, "leafnodes { listen: \"127.0.0.1:%d\" }\n", port(pLeaf, i))
			}
			n, err := startNode(cfg.Dir, name, b.String())
			if err != nil {
				sc.Shutdown()
				return nil, err
			}
			n.Cluster = cname
			n.ClientURL = fmt.Sprintf("nats://127.0.0.1:%d", port(pClient, i))
			if cfg.Monitoring {
				n.MonitorURL = fmt.Sprintf("http://127.0.0.1:%d", port(pMonitor, i))
			}
			sc.Nodes = append(sc.Nodes, n)
		}
	}

	if cfg.Leaf {
		i := total
		var remotes []string
		for k := 0; k < cfg.ServersPerCluster; k++ {
			remotes = append(remotes, fmt.Sprintf("nats-leaf://%s:%s@127.0.0.1:%d", AppUser, AppPass, port(pLeaf, k)))
		}
		var b strings.Builder
		fmt.Fprintf(&b, "server_name: \"edge-1\"\nlisten: \"127.0.0.1:%d\"\n", port(pClient, i))
		if cfg.Monitoring {
			fmt.Fprintf(&b, "http: \"127.0.0.1:%d\"\n", port(pMonitor, i))
		}
		fmt.Fprintf(&b, "jetstream { store_dir: %q, domain: \"edge\", max_memory_store: 128MB, max_file_store: 1GB }\n", filepath.Join(cfg.Dir, "edge-1", "js"))
		fmt.Fprintf(&b, accountsBlock, SysUser, SysPass, EdgeUser, EdgePass)
		fmt.Fprintf(&b, "leafnodes { remotes: [ { urls: [%s], account: \"APP\" } ] }\n", quoteList(remotes))
		n, err := startNode(cfg.Dir, "edge-1", b.String())
		if err != nil {
			sc.Shutdown()
			return nil, err
		}
		n.ClientURL = fmt.Sprintf("nats://127.0.0.1:%d", port(pClient, i))
		if cfg.Monitoring {
			n.MonitorURL = fmt.Sprintf("http://127.0.0.1:%d", port(pMonitor, i))
		}
		sc.Leaf = n
	}

	if err := sc.waitFormed(total, len(cfg.Clusters), 30*time.Second); err != nil {
		sc.Shutdown()
		return nil, err
	}
	return sc, nil
}

func startNode(dir, name, conf string) (*Node, error) {
	path := filepath.Join(dir, name+".conf")
	if err := os.WriteFile(path, []byte(conf), 0o644); err != nil {
		return nil, err
	}
	opts, err := server.ProcessConfigFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	opts.NoSigs = true
	s, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	go s.Start()
	if !s.ReadyForConnections(15 * time.Second) {
		s.Shutdown()
		return nil, fmt.Errorf("%s: not ready for connections", name)
	}
	return &Node{Name: name, Server: s}, nil
}

func (s *Supercluster) waitFormed(total, clusters int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	perCluster := total / clusters
	for time.Now().Before(deadline) {
		ok := true
		for _, n := range s.Nodes {
			if n.Server.NumRoutes() < perCluster-1 {
				ok = false
				break
			}
			if clusters > 1 && n.Server.NumOutboundGateways() < clusters-1 {
				ok = false
				break
			}
			if !n.Server.JetStreamIsCurrent() {
				ok = false
				break
			}
		}
		if ok && s.Leaf != nil && s.Leaf.Server.NumLeafNodes() == 0 {
			ok = false
		}
		if ok {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("supercluster did not form within %s", timeout)
}

func quoteList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}
