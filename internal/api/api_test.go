package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/wankdanker/natsmin/internal/api"
	"github.com/wankdanker/natsmin/internal/devcluster"
	"github.com/wankdanker/natsmin/internal/natsx"
	"github.com/wankdanker/natsmin/internal/secret"
	"github.com/wankdanker/natsmin/internal/store"
)

type natsMsg = nats.Msg

type client struct {
	t     *testing.T
	base  string
	hc    *http.Client
	token string
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, hc: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	ct := "application/json"
	switch b := body.(type) {
	case nil:
	case []byte:
		r = bytes.NewReader(b)
		ct = "application/octet-stream"
	case string:
		r = strings.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	if r != nil {
		req.Header.Set("Content-Type", ct)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(data) > 0 {
		if raw, ok := out.(*[]byte); ok {
			*raw = data
		} else if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %v: %s", method, path, err, data)
		}
	}
	if resp.StatusCode >= 400 && out != nil {
		c.t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, data)
	}
	return resp.StatusCode
}

func (c *client) must(method, path string, body any, out any, want int) {
	c.t.Helper()
	if got := c.do(method, path, body, out); got != want {
		c.t.Fatalf("%s %s: want %d, got %d", method, path, want, got)
	}
}

func TestAPIEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a supercluster")
	}
	dir := t.TempDir()
	sc, err := devcluster.Start(devcluster.Config{Dir: filepath.Join(dir, "nats"), BasePort: 30000,
		Clusters: []string{"east", "west"}, ServersPerCluster: 3})
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
	mgr := natsx.NewManager(st, natsx.Options{PollInterval: time.Second}, nil)
	defer mgr.Stop()
	srv := httptest.NewServer(api.New(api.Config{AllowSetup: true}, st, mgr, nil, nil))
	defer srv.Close()

	admin := newClient(t, srv.URL+"/api")

	// Setup and auth.
	var status api.StatusBody
	admin.must("GET", "/v1/status", nil, &status, 200)
	if !status.NeedsSetup {
		t.Fatal("expected setup to be needed")
	}
	admin.must("GET", "/v1/systems", nil, nil, 401)
	admin.must("POST", "/v1/setup", map[string]string{"username": "admin", "password": "supersecret"}, nil, 200)
	admin.must("POST", "/v1/setup", map[string]string{"username": "evil", "password": "supersecret"}, nil, 409)
	var me api.MeBody
	admin.must("GET", "/v1/auth/me", nil, &me, 200)
	if me.Role != store.RoleAdmin {
		t.Fatalf("want admin, got %s", me.Role)
	}

	// Register the supercluster with a system and an app account.
	var sys api.SystemOut
	admin.must("POST", "/v1/systems", map[string]any{
		"name": "Dev Supercluster", "urls": []string{sc.Nodes[0].ClientURL, sc.Nodes[3].ClientURL},
		"systemAuth": map[string]string{"type": "userpass", "username": "sys", "password": "sys"},
		"accounts":   []map[string]any{{"name": "App", "auth": map[string]string{"type": "userpass", "username": "app", "password": "app"}}},
	}, &sys, 201)
	if sys.ID != "dev-supercluster" || len(sys.Accounts) != 1 || sys.Accounts[0].ID != "app" {
		t.Fatalf("unexpected system: %+v", sys)
	}
	if sys.SystemAuth == nil || !sys.SystemAuth.HasPassword {
		t.Fatal("system auth not stored")
	}
	raw, _ := json.Marshal(sys)
	if strings.Contains(string(raw), `"sys"`) && strings.Contains(string(raw), "password\":\"sys") {
		t.Fatal("password leaked in response")
	}

	var test api.TestResult
	admin.must("POST", "/v1/systems/test", map[string]any{"systemId": sys.ID, "urls": []string{sc.Nodes[0].ClientURL},
		"auth": map[string]string{"type": "userpass", "username": "sys"}}, &test, 200)
	if !test.OK || !test.IsSystemAccount || test.ServersFound != 6 {
		t.Fatalf("unexpected test result: %+v", test)
	}

	// Snapshot.
	var snap natsx.Snapshot
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		admin.must("GET", "/v1/systems/dev-supercluster/snapshot?refresh=true", nil, &snap, 200)
		if snap.Totals.Servers == 6 && snap.JetStream != nil && snap.JetStream.MetaLeader != "" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if snap.Totals.Servers != 6 || len(snap.Gateways) != 6 {
		t.Fatalf("unexpected snapshot: servers=%d gateways=%d err=%s", snap.Totals.Servers, len(snap.Gateways), snap.Error)
	}

	js := "/v1/systems/dev-supercluster/accounts/app/jetstream"

	// Streams.
	var info map[string]any
	admin.must("POST", js+"/streams", map[string]any{"name": "ORDERS", "subjects": []string{"orders.>"}, "num_replicas": 3, "storage": "file",
		"placement": map[string]any{"cluster": "west"}}, &info, 201)
	if info["cluster"].(map[string]any)["name"] != "west" {
		t.Fatalf("stream not placed in west: %v", info["cluster"])
	}
	admin.must("POST", js+"/streams", map[string]any{"name": "ORDERS", "subjects": []string{"orders.>"}, "num_replicas": 3}, nil, 400)
	var streams []map[string]any
	admin.must("GET", js+"/streams", nil, &streams, 200)
	if len(streams) != 1 {
		t.Fatalf("want 1 stream, got %d", len(streams))
	}

	// Publish and browse.
	for i := 1; i <= 5; i++ {
		var pub api.PublishOut
		admin.must("POST", "/v1/systems/dev-supercluster/accounts/app/publish", map[string]any{
			"subject": fmt.Sprintf("orders.%d", i%2), "data": fmt.Sprintf(`{"n":%d}`, i), "jetstream": true,
			"headers": map[string][]string{"X-Test": {"yes"}}}, &pub, 200)
		if pub.Stream != "ORDERS" || pub.Seq != uint64(i) {
			t.Fatalf("unexpected publish ack: %+v", pub)
		}
	}
	var page api.MsgPage
	admin.must("GET", js+"/streams/ORDERS/messages?limit=10", nil, &page, 200)
	if len(page.Messages) != 5 || page.NextSeq != 0 || !page.Messages[0].IsJSON || page.Messages[0].Headers["X-Test"][0] != "yes" {
		t.Fatalf("unexpected page: %+v", page)
	}
	admin.must("GET", js+"/streams/ORDERS/messages?subject=orders.0&limit=10", nil, &page, 200)
	if len(page.Messages) != 2 {
		t.Fatalf("want 2 orders.0 messages, got %d", len(page.Messages))
	}
	admin.must("GET", js+"/streams/ORDERS/messages?limit=2&startSeq=2", nil, &page, 200)
	if len(page.Messages) != 2 || page.Messages[0].Seq != 2 || page.NextSeq != 4 {
		t.Fatalf("paging broken: %+v", page)
	}
	var msg api.MsgOut
	admin.must("GET", js+"/streams/ORDERS/last?subject=orders.1", nil, &msg, 200)
	if msg.Seq != 5 {
		t.Fatalf("want last orders.1 at 5, got %d", msg.Seq)
	}
	var repub api.PublishOut
	admin.must("POST", js+"/streams/ORDERS/messages/1/republish", map[string]any{}, &repub, 200)
	if repub.Seq != 6 {
		t.Fatalf("republish: %+v", repub)
	}
	admin.must("DELETE", js+"/streams/ORDERS/messages/6", nil, nil, 204)
	admin.must("GET", js+"/streams/ORDERS/messages/6", nil, nil, 404)

	// Consumers.
	admin.must("POST", js+"/streams/ORDERS/consumers", map[string]any{"durable_name": "billing", "ack_policy": "explicit", "filter_subject": "orders.1"}, &info, 201)
	var consumers []map[string]any
	admin.must("GET", js+"/streams/ORDERS/consumers", nil, &consumers, 200)
	if len(consumers) != 1 || consumers[0]["num_pending"].(float64) != 3 {
		t.Fatalf("unexpected consumers: %v", consumers)
	}
	admin.must("POST", js+"/streams/ORDERS/consumers/billing/pause", map[string]any{"until": time.Now().Add(time.Hour)}, nil, 200)
	admin.must("POST", js+"/streams/ORDERS/consumers/billing/resume", nil, nil, 200)

	// Placement view across accounts, via the system account.
	var placement []api.StreamPlacement
	admin.must("GET", "/v1/systems/dev-supercluster/jetstream/assets", nil, &placement, 200)
	if len(placement) != 1 || placement[0].Stream != "ORDERS" || len(placement[0].Replicas) != 3 || placement[0].Cluster != "west" {
		t.Fatalf("unexpected placement: %+v", placement)
	}

	// Stream stepdown changes the leader.
	oldLeader := placement[0].Leader
	admin.must("POST", js+"/streams/ORDERS/stepdown", nil, nil, 200)
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		admin.must("GET", "/v1/systems/dev-supercluster/jetstream/assets", nil, &placement, 200)
		if len(placement) == 1 && placement[0].Leader != "" && placement[0].Leader != oldLeader {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if placement[0].Leader == oldLeader {
		t.Errorf("stream leader did not change from %s", oldLeader)
	}

	// Purge.
	admin.must("POST", js+"/streams/ORDERS/purge", map[string]any{"subject": "orders.0"}, nil, 200)
	admin.must("GET", js+"/streams/ORDERS", nil, &info, 200)
	if info["state"].(map[string]any)["messages"].(float64) != 3 {
		t.Fatalf("purge failed: %v", info["state"])
	}

	// KV.
	kv := "/v1/systems/dev-supercluster/accounts/app/kv"
	admin.must("POST", kv, map[string]any{"bucket": "config", "history": 5, "replicas": 3}, nil, 201)
	var rev struct{ Revision uint64 }
	admin.must("PUT", kv+"/config/entry", map[string]any{"key": "app.mode", "value": "blue"}, &rev, 200)
	admin.must("PUT", kv+"/config/entry", map[string]any{"key": "app.mode", "value": "green", "expectRevision": rev.Revision}, &rev, 200)
	admin.must("PUT", kv+"/config/entry", map[string]any{"key": "app.mode", "value": "stale", "expectRevision": 1}, nil, 409)
	var entry api.KVEntryOut
	admin.must("GET", kv+"/config/entry?key=app.mode", nil, &entry, 200)
	if entry.Text == nil || *entry.Text != "green" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	var hist []api.KVEntryOut
	admin.must("GET", kv+"/config/history?key=app.mode", nil, &hist, 200)
	if len(hist) != 2 || *hist[0].Text != "green" {
		t.Fatalf("unexpected history: %+v", hist)
	}
	var keys struct{ Keys []string }
	admin.must("GET", kv+"/config/keys", nil, &keys, 200)
	if len(keys.Keys) != 1 {
		t.Fatalf("unexpected keys: %v", keys.Keys)
	}
	var buckets []api.KVStatusOut
	admin.must("GET", kv, nil, &buckets, 200)
	if len(buckets) != 1 || buckets[0].Replicas != 3 || buckets[0].History != 5 {
		t.Fatalf("unexpected buckets: %+v", buckets)
	}
	admin.must("DELETE", kv+"/config/entry?key=app.mode", nil, nil, 204)
	admin.must("GET", kv+"/config/entry?key=app.mode", nil, nil, 404)

	// Object store.
	obj := "/v1/systems/dev-supercluster/accounts/app/objects"
	admin.must("POST", obj, map[string]any{"bucket": "files"}, nil, 201)
	payload := bytes.Repeat([]byte("natsmin "), 100000)
	var oi api.ObjInfoOut
	admin.must("PUT", obj+"/files/upload?name=docs/readme.txt", payload, &oi, 200)
	if oi.Size != uint64(len(payload)) {
		t.Fatalf("unexpected object info: %+v", oi)
	}
	var got []byte
	admin.must("GET", obj+"/files/download?name=docs/readme.txt", nil, &got, 200)
	if !bytes.Equal(got, payload) {
		t.Fatalf("download mismatch: %d vs %d bytes", len(got), len(payload))
	}

	// Request/reply.
	var reply api.TailMsg
	nc, err := mgr.Account(context.Background(), "dev-supercluster", "app")
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := nc.Subscribe("echo", func(m *natsMsg) { _ = m.Respond(append([]byte("echo: "), m.Data...)) })
	defer sub.Unsubscribe()
	admin.must("POST", "/v1/systems/dev-supercluster/accounts/app/request", map[string]any{"subject": "echo", "data": base64.StdEncoding.EncodeToString([]byte("hi")), "encoding": "base64"}, &reply, 200)
	if reply.Text == nil || *reply.Text != "echo: hi" {
		t.Fatalf("unexpected reply: %+v", reply)
	}

	// Interest and connections.
	var interest api.InterestOut
	admin.must("GET", "/v1/systems/dev-supercluster/interest?subject=echo&account=APP", nil, &interest, 200)
	if interest.Total < 1 {
		t.Fatalf("expected interest in echo: %+v", interest)
	}
	var conns api.ConnectionsOut
	admin.must("GET", "/v1/systems/dev-supercluster/connections?account=APP", nil, &conns, 200)
	if conns.Total < 1 || conns.Servers != 6 {
		t.Fatalf("unexpected connections: total=%d servers=%d", conns.Total, conns.Servers)
	}
	var accStats []api.AccountStatOut
	admin.must("GET", "/v1/systems/dev-supercluster/account-stats", nil, &accStats, 200)
	if len(accStats) == 0 {
		t.Fatal("expected account stats")
	}
	var mon []api.MonitorResponse
	admin.must("GET", `/v1/systems/dev-supercluster/monitor/jsz?options={"accounts":true}`, nil, &mon, 200)
	if len(mon) != 6 {
		t.Fatalf("want 6 jsz answers, got %d", len(mon))
	}

	// RBAC: a viewer token can read but not write.
	admin.must("POST", "/v1/users", map[string]any{"username": "vera", "password": "viewerpass", "role": "viewer"}, nil, 201)
	viewer := newClient(t, srv.URL+"/api")
	viewer.must("POST", "/v1/auth/login", map[string]string{"username": "vera", "password": "wrong"}, nil, 401)
	viewer.must("POST", "/v1/auth/login", map[string]string{"username": "vera", "password": "viewerpass"}, nil, 200)
	var tok struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	viewer.must("POST", "/v1/tokens", map[string]any{"name": "ci", "role": "admin"}, &tok, 201)
	if tok.Role != "viewer" || !strings.HasPrefix(tok.Token, "nm_") {
		t.Fatalf("token role must be capped at viewer: %+v", tok)
	}
	bot := newClient(t, srv.URL+"/api")
	bot.token = tok.Token
	bot.must("GET", js+"/streams", nil, nil, 200)
	bot.must("DELETE", js+"/streams/ORDERS", nil, nil, 403)
	bot.must("POST", "/v1/tokens", map[string]any{"name": "escalate"}, nil, 403)
	bot.token = "nm_invalid"
	bot.must("GET", js+"/streams", nil, nil, 401)

	// Cleanup and audit.
	admin.must("DELETE", js+"/streams/ORDERS", nil, nil, 204)
	var audit []store.AuditEntry
	admin.must("GET", "/v1/audit?limit=500", nil, &audit, 200)
	actions := map[string]bool{}
	for _, e := range audit {
		actions[e.Action] = true
	}
	for _, want := range []string{"setup", "create-system", "create-stream", "delete-stream", "put-kv-entry", "upload-object", "login"} {
		if !actions[want] {
			t.Errorf("audit log is missing %q", want)
		}
	}
	viewer.must("GET", "/v1/audit", nil, nil, 403)
}
