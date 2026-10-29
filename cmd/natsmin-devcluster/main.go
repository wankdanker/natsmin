// Command natsmin-devcluster runs a local NATS supercluster for developing
// and trying natsmin, without Docker.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wankdanker/natsmin/internal/devcluster"
)

func main() {
	dir := flag.String("dir", ".devcluster", "directory for configs and JetStream data")
	base := flag.Int("port", 4222, "first client port")
	clusters := flag.String("clusters", "east,west", "comma separated cluster names")
	size := flag.Int("size", 3, "servers per cluster")
	leaf := flag.Bool("leaf", true, "add a leafnode server with JetStream domain \"edge\"")
	demo := flag.Bool("demo", false, "seed streams, KV and objects in the APP account and generate traffic")
	flag.Parse()

	cfg := devcluster.Config{Dir: *dir, BasePort: *base, Clusters: strings.Split(*clusters, ","), ServersPerCluster: *size, Leaf: *leaf, Monitoring: true}
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
	if *demo {
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
