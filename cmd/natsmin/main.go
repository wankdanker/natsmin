// Command natsmin serves the natsmin API and UI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	clyops "github.com/wankdanker/clyops/packages/go"

	"github.com/wankdanker/natsmin/internal/api"
	"github.com/wankdanker/natsmin/internal/auth"
	"github.com/wankdanker/natsmin/internal/natsx"
	"github.com/wankdanker/natsmin/internal/secret"
	"github.com/wankdanker/natsmin/internal/store"
	"github.com/wankdanker/natsmin/internal/version"
	"github.com/wankdanker/natsmin/web"
)

// cli declares the natsmin commands and options. Option variables double as the
// environment variables that set them.
func cli() *clyops.Cli {
	c := clyops.New()
	c.SetDescription("natsmin: management UI and HTTP API for NATS.")

	srv := c.Command("serve", "Serve the natsmin UI and API")
	srv.SetEffects("network")
	srv.Opt("NATSMIN_ADDR", "addr", "a", ":8080", "Listen address", "Server")
	srv.Opt("NATSMIN_DATA", "data", "d", "./data", "Directory for the database and secret key", "Server", "path")
	srv.Opt("NATSMIN_POLL_INTERVAL", "poll", "", "5s", "Monitoring poll interval (Go duration)", "Server", `regex:^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`)
	srv.Opt("NATSMIN_TLS_CERT", "tls-cert", "", "optional", "Serve HTTPS with this certificate", "TLS", "file:readable")
	srv.Opt("NATSMIN_TLS_KEY", "tls-key", "", "optional", "Key for --tls-cert", "TLS", "file:readable")
	srv.Opt("NATSMIN_SECURE_COOKIES", "secure-cookies", "", "flag", "Mark session cookies Secure (enable behind HTTPS)", "Security")
	srv.Opt("NATSMIN_ALLOW_SETUP", "allow-setup", "", "true", "Let the first visitor create the admin when no users exist", "Security", "bool")
	srv.Opt("NATSMIN_SECRET_KEY", "secret-key", "", "optional", "Base64 32-byte key for stored credentials; generated in the data directory when unset", "Security", "secret")
	srv.Opt("NATSMIN_ADMIN_USER", "admin-user", "", "optional", "Create this admin on startup when no users exist", "Security")
	srv.Opt("NATSMIN_ADMIN_PASSWORD", "admin-password", "", "optional", "Password for --admin-user", "Security", "secret")
	srv.Opt("NATSMIN_LOG_JSON", "log-json", "", "flag", "Log as JSON", "Logging")
	srv.Requires("tls-cert", "tls-key")
	srv.Requires("tls-key", "tls-cert")
	srv.Requires("admin-user", "admin-password")

	c.Command("openapi", "Print the OpenAPI document of the natsmin API").
		SetEffects("read-only").
		SetStdout("OpenAPI 3.1 document", "application/json")
	c.Command("version", "Print the natsmin version").
		SetEffects("read-only").
		SetStdout("Version and commit", "application/json")
	return c
}

func main() {
	c := cli()
	v := c.Run()
	switch strings.Join(c.CommandPath(), " ") {
	case "openapi":
		dumpOpenAPI()
		return
	case "version":
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"version": version.Version, "commit": version.Commit})
		return
	}

	poll, err := time.ParseDuration(v.String("NATSMIN_POLL_INTERVAL"))
	if err != nil || poll <= 0 {
		fmt.Fprintf(os.Stderr, "natsmin: --poll must be a positive duration, got %q\n", v.String("NATSMIN_POLL_INTERVAL"))
		os.Exit(1)
	}
	var handler slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if v.Bool("NATSMIN_LOG_JSON") {
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	log := slog.New(handler)
	cfg := config{
		addr:       v.String("NATSMIN_ADDR"),
		dataDir:    v.String("NATSMIN_DATA"),
		poll:       poll,
		tlsCert:    v.String("NATSMIN_TLS_CERT"),
		tlsKey:     v.String("NATSMIN_TLS_KEY"),
		allowSetup: v.Bool("NATSMIN_ALLOW_SETUP"),
		secretKey:  v.String("NATSMIN_SECRET_KEY"),
		adminUser:  v.String("NATSMIN_ADMIN_USER"),
		adminPass:  v.String("NATSMIN_ADMIN_PASSWORD"),
	}
	cfg.secure = v.Bool("NATSMIN_SECURE_COOKIES") || cfg.tlsCert != ""
	if err := run(log, cfg); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

type config struct {
	addr, dataDir, tlsCert, tlsKey  string
	poll                            time.Duration
	secure, allowSetup              bool
	secretKey, adminUser, adminPass string
}

func run(log *slog.Logger, c config) error {
	addr, dataDir, poll, secure, allowSetup, tlsCert, tlsKey := c.addr, c.dataDir, c.poll, c.secure, c.allowSetup, c.tlsCert, c.tlsKey
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	box, err := secret.Load(c.secretKey, filepath.Join(dataDir, "secret.key"))
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	st, err := store.Open(filepath.Join(dataDir, "natsmin.db"), box)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bootstrapAdmin(ctx, st, log, c.adminUser, c.adminPass); err != nil {
		return err
	}

	mgr := natsx.NewManager(st, natsx.Options{PollInterval: poll}, log)
	if err := mgr.Start(ctx); err != nil {
		return err
	}
	defer mgr.Stop()

	ui := web.FS()
	if ui == nil {
		log.Warn("UI not built; serving the API only (run `make ui`)")
	}
	h := api.New(api.Config{SecureCookies: secure, AllowSetup: allowSetup}, st, mgr, ui, log)
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = st.PruneSessions(ctx)
			}
		}
	}()

	errc := make(chan error, 1)
	go func() {
		log.Info("natsmin listening", "addr", addr, "version", version.Version, "tls", tlsCert != "")
		if tlsCert != "" {
			errc <- srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}
	return nil
}

// bootstrapAdmin creates the admin from NATSMIN_ADMIN_USER/PASSWORD when no
// users exist yet.
func bootstrapAdmin(ctx context.Context, st *store.Store, log *slog.Logger, user, pass string) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if user == "" || pass == "" {
		log.Info("no users yet: open the UI to create the first admin, or set NATSMIN_ADMIN_USER and NATSMIN_ADMIN_PASSWORD")
		return nil
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, user, hash, store.RoleAdmin); err != nil {
		return err
	}
	log.Info("created admin user from environment", "user", user)
	return nil
}

func dumpOpenAPI() {
	h := api.New(api.Config{}, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		fmt.Fprintln(os.Stderr, "openapi:", err, rec.Body.String())
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
