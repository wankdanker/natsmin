// Command natsmin serves the natsmin API and UI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/wankdanker/natsmin/internal/api"
	"github.com/wankdanker/natsmin/internal/auth"
	"github.com/wankdanker/natsmin/internal/natsx"
	"github.com/wankdanker/natsmin/internal/secret"
	"github.com/wankdanker/natsmin/internal/store"
	"github.com/wankdanker/natsmin/internal/version"
	"github.com/wankdanker/natsmin/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "openapi":
			dumpOpenAPI()
			return
		case "version", "--version", "-v":
			fmt.Println("natsmin", version.Version, version.Commit)
			return
		}
	}

	addr := flag.String("addr", env("NATSMIN_ADDR", ":8080"), "listen address (NATSMIN_ADDR)")
	dataDir := flag.String("data", env("NATSMIN_DATA", "./data"), "data directory for the database and secret key (NATSMIN_DATA)")
	poll := flag.Duration("poll", envDur("NATSMIN_POLL_INTERVAL", 5*time.Second), "monitoring poll interval (NATSMIN_POLL_INTERVAL)")
	secure := flag.Bool("secure-cookies", envBool("NATSMIN_SECURE_COOKIES", false), "mark session cookies Secure; enable behind HTTPS (NATSMIN_SECURE_COOKIES)")
	allowSetup := flag.Bool("allow-setup", envBool("NATSMIN_ALLOW_SETUP", true), "let the first visitor create the admin when no users exist (NATSMIN_ALLOW_SETUP)")
	tlsCert := flag.String("tls-cert", env("NATSMIN_TLS_CERT", ""), "serve HTTPS with this certificate (NATSMIN_TLS_CERT)")
	tlsKey := flag.String("tls-key", env("NATSMIN_TLS_KEY", ""), "key for -tls-cert (NATSMIN_TLS_KEY)")
	logJSON := flag.Bool("log-json", envBool("NATSMIN_LOG_JSON", false), "log as JSON (NATSMIN_LOG_JSON)")
	flag.Parse()

	var handler slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if *logJSON {
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	log := slog.New(handler)
	if err := run(log, *addr, *dataDir, *poll, *secure || *tlsCert != "", *allowSetup, *tlsCert, *tlsKey); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, addr, dataDir string, poll time.Duration, secure, allowSetup bool, tlsCert, tlsKey string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	box, err := secret.Load(os.Getenv("NATSMIN_SECRET_KEY"), filepath.Join(dataDir, "secret.key"))
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

	if err := bootstrapAdmin(ctx, st, log); err != nil {
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
func bootstrapAdmin(ctx context.Context, st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	user, pass := os.Getenv("NATSMIN_ADMIN_USER"), os.Getenv("NATSMIN_ADMIN_PASSWORD")
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
