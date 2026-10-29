package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wankdanker/natsmin/internal/secret"
	"github.com/wankdanker/natsmin/internal/store"
)

func open(t *testing.T) (*store.Store, string) {
	t.Helper()
	box, err := secret.New([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "db.sqlite")
	st, err := store.Open(path, box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, path
}

func TestRoles(t *testing.T) {
	if !store.RoleAdmin.Allows(store.RoleOperator) || store.RoleViewer.Allows(store.RoleOperator) {
		t.Fatal("role ordering is wrong")
	}
	if store.Min(store.RoleAdmin, store.RoleViewer) != store.RoleViewer {
		t.Fatal("Min is wrong")
	}
	if store.Role("root").Valid() {
		t.Fatal("unknown role reported valid")
	}
}

func TestSystemSecretsAreEncrypted(t *testing.T) {
	st, path := open(t)
	ctx := context.Background()
	sys := store.System{ID: "prod", Name: "Prod", URLs: []string{"nats://a:4222"},
		SystemAuth: &store.Credential{Type: "userpass", Username: "sys", Password: "hunter2-very-secret"}}
	if err := st.SaveSystem(ctx, &sys, true); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSystem(ctx, &sys, true); err != store.ErrConflict {
		t.Fatalf("want conflict, got %v", err)
	}
	got, err := st.GetSystem(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.SystemAuth == nil || got.SystemAuth.Password != "hunter2-very-secret" {
		t.Fatalf("credentials did not round-trip: %+v", got.SystemAuth)
	}
	st.Close()

	// The plaintext must not appear anywhere in the database files.
	for _, f := range []string{path, path + "-wal"} {
		data, _ := readFile(f)
		if strings.Contains(string(data), "hunter2-very-secret") {
			t.Fatalf("plaintext password found in %s", f)
		}
	}
}

func TestTokensCappedAndExpire(t *testing.T) {
	st, _ := open(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "op", "hash", store.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateToken(ctx, u.ID, "ci", "nm_abc", "h1", store.RoleAdmin, nil); err != nil {
		t.Fatal(err)
	}
	_, role, err := st.TokenUser(ctx, "h1")
	if err != nil || role != store.RoleOperator {
		t.Fatalf("token role must be capped at the user's role, got %s %v", role, err)
	}
	past := time.Now().Add(-time.Hour)
	if _, err := st.CreateToken(ctx, u.ID, "old", "nm_def", "h2", store.RoleViewer, &past); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.TokenUser(ctx, "h2"); err != store.ErrNotFound {
		t.Fatalf("expired token accepted: %v", err)
	}
	u.Disabled = true
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.TokenUser(ctx, "h1"); err != store.ErrNotFound {
		t.Fatalf("token of disabled user accepted: %v", err)
	}
}

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }
