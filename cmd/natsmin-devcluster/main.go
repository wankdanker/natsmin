// Command natsmin-devcluster runs a local NATS supercluster for developing
// and trying natsmin, without Docker.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	clyops "github.com/wankdanker/clyops/packages/go"

	"github.com/wankdanker/natsmin/internal/devcluster"
)

func main() {
	cli := clyops.New()
	cli.SetDescription("Run a local NATS supercluster (and optionally demo traffic) for developing and trying natsmin.")
	cli.SetEffects("network")
	cli.Opt("DEVCLUSTER_DIR", "dir", "d", ".devcluster", "Directory for configs and JetStream data", "Cluster", "path")
	cli.Opt("DEVCLUSTER_PORT", "port", "p", "4222", "First client port; other ports are offset from it", "Cluster", "port")
	cli.Opt("DEVCLUSTER_CLUSTERS", "clusters", "c", "east,west", "Comma separated cluster names", "Cluster", `regex:^[A-Za-z0-9_-]+(,[A-Za-z0-9_-]+)*$`)
	cli.Opt("DEVCLUSTER_SIZE", "size", "s", "3", "Servers per cluster", "Cluster", "int:1-9")
	cli.Opt("DEVCLUSTER_LEAF", "leaf", "", "true", "Add a leafnode server with JetStream domain \"edge\"", "Cluster", "bool")
	cli.Opt("DEVCLUSTER_DEMO", "demo", "", "flag", "Seed streams, KV and objects in the APP account and generate traffic", "Demo")
	v := cli.Run()

	cfg := devcluster.Config{Dir: v.String("DEVCLUSTER_DIR"), BasePort: v.Int("DEVCLUSTER_PORT"),
		Clusters: strings.Split(v.String("DEVCLUSTER_CLUSTERS"), ","), ServersPerCluster: v.Int("DEVCLUSTER_SIZE"),
		Leaf: v.Bool("DEVCLUSTER_LEAF"), Monitoring: true}
	sc, err := devcluster.Start(cfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("supercluster is up")
	for _, n := range sc.Nodes {
		fmt.Printf("  %-8s cluster=%-6s %s  monitor %s\n", n.Name, n.Cluster, n.ClientURL, n.MonitorURL)
	}
	if sc.Leaf != nil {
		fmt.Printf("  %-8s leafnode       %s  monitor %s\n", sc.Leaf.Name, sc.Leaf.ClientURL, sc.Leaf.MonitorURL)
	}
	fmt.Printf("\nsystem account: %s / %s   app account: %s / %s   edge account: %s / %s\n",
		devcluster.SysUser, devcluster.SysPass, devcluster.AppUser, devcluster.AppPass, devcluster.EdgeUser, devcluster.EdgePass)
	fmt.Println("press ctrl-c to stop")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if v.Bool("DEVCLUSTER_DEMO") {
		go func() {
			if err := devcluster.Demo(ctx, sc.URLs()); err != nil {
				log.Printf("demo traffic: %v", err)
			}
		}()
		fmt.Println("generating demo traffic in the APP account")
	}
	<-ctx.Done()
	sc.Shutdown()
}
