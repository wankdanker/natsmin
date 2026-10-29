package natsx_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wankdanker/natsmin/internal/devcluster"
	"github.com/wankdanker/natsmin/internal/natsx"
	"github.com/wankdanker/natsmin/internal/secret"
	"github.com/wankdanker/natsmin/internal/store"
)

func TestSnapshotOfSupercluster(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a supercluster")
	}
	dir := t.TempDir()
	sc, err := devcluster.Start(devcluster.Config{Dir: filepath.Join(dir, "nats"), BasePort: 20000,
		Clusters: []string{"east", "west"}, ServersPerCluster: 3, Leaf: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Shutdown()

	box, _ := secret.New(make([]byte, 32))
	st, err := store.Open(filepath.Join(dir, "db.sqlite"), box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sys := store.System{ID: "dev", Name: "dev", URLs: sc.URLs()[:1],
		SystemAuth: &store.Credential{Type: "userpass", Username: devcluster.SysUser, Password: devcluster.SysPass}}
	if err := st.SaveSystem(context.Background(), &sys, true); err != nil {
		t.Fatal(err)
	}

	m := natsx.NewManager(st, natsx.Options{PollInterval: time.Hour}, nil)
	defer m.Stop()
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var snap *natsx.Snapshot
	for i := 0; i < 20; i++ {
		snap, err = m.PollNow(context.Background(), "dev")
		if err != nil {
			t.Fatal(err)
		}
		if snap.Totals.Servers == 6 && snap.JetStream != nil && snap.JetStream.MetaLeader != "" {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if snap.Error != "" {
		t.Fatalf("snapshot error: %s", snap.Error)
	}
	if snap.Totals.Servers != 6 || snap.Totals.Clusters != 2 {
		t.Fatalf("want 6 servers in 2 clusters, got %d in %d", snap.Totals.Servers, snap.Totals.Clusters)
	}
	// Each of 6 servers routes to its 2 cluster peers.
	if len(snap.Routes) != 12 {
		t.Errorf("want 12 route edges, got %d", len(snap.Routes))
	}
	// Each server has one outbound gateway to the other cluster.
	if len(snap.Gateways) != 6 {
		t.Errorf("want 6 gateway edges, got %d", len(snap.Gateways))
	}
	if len(snap.Leafnodes) != 1 {
		t.Errorf("want 1 leafnode, got %d", len(snap.Leafnodes))
	}
	if snap.JetStream == nil || snap.JetStream.MetaSize != 6 || len(snap.JetStream.Peers) != 6 {
		t.Errorf("unexpected jetstream summary: %+v", snap.JetStream)
	}
	for _, s := range snap.Servers {
		if !s.Healthy {
			t.Errorf("%s unhealthy: %s", s.Name, s.HealthError)
		}
	}

	// A second poll yields rates and a history point per poll.
	if _, err := m.PollNow(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	if h := m.History("dev"); len(h) < 2 {
		t.Errorf("want history points, got %d", len(h))
	}
}
