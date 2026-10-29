// Package natsx talks to NATS: it manages connections per registered system,
// polls the system account for topology and stats, and wraps JetStream.
package natsx

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"

	"github.com/wankdanker/natsmin/internal/store"
)

// Connect opens a NATS connection with the given TLS and credential settings.
func Connect(name string, urls []string, tlsCfg *store.TLSConfig, cred *store.Credential, timeout time.Duration) (*nats.Conn, error) {
	if len(urls) == 0 {
		return nil, errors.New("no server URLs configured")
	}
	opts := []nats.Option{
		nats.Name(name),
		nats.Timeout(timeout),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.RetryOnFailedConnect(false),
	}
	if tlsCfg != nil && (tlsCfg.CA != "" || tlsCfg.Cert != "" || tlsCfg.InsecureSkipVerify) {
		cfg, err := buildTLS(tlsCfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, nats.Secure(cfg))
	}
	if cred != nil {
		o, err := credOptions(cred)
		if err != nil {
			return nil, err
		}
		opts = append(opts, o...)
	}
	return nats.Connect(strings.Join(urls, ","), opts...)
}

func buildTLS(c *store.TLSConfig) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.InsecureSkipVerify} //nolint:gosec // opt-in per system
	if c.CA != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(c.CA)) {
			return nil, errors.New("tls: could not parse CA certificate")
		}
		cfg.RootCAs = pool
	}
	if c.Cert != "" || c.Key != "" {
		cert, err := tls.X509KeyPair([]byte(c.Cert), []byte(c.Key))
		if err != nil {
			return nil, fmt.Errorf("tls: client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func credOptions(c *store.Credential) ([]nats.Option, error) {
	switch c.Type {
	case "", "none":
		return nil, nil
	case "userpass":
		return []nats.Option{nats.UserInfo(c.Username, c.Password)}, nil
	case "token":
		return []nats.Option{nats.Token(c.Token)}, nil
	case "creds":
		userJWT, err := jwt.ParseDecoratedJWT([]byte(c.Creds))
		if err != nil {
			return nil, fmt.Errorf("creds: %w", err)
		}
		kp, err := nkeys.ParseDecoratedNKey([]byte(c.Creds))
		if err != nil {
			return nil, fmt.Errorf("creds: %w", err)
		}
		seed, err := kp.Seed()
		if err != nil {
			return nil, fmt.Errorf("creds: %w", err)
		}
		return []nats.Option{nats.UserJWTAndSeed(userJWT, string(seed))}, nil
	case "nkey":
		kp, err := nkeys.FromSeed([]byte(strings.TrimSpace(c.NKeySeed)))
		if err != nil {
			return nil, fmt.Errorf("nkey: %w", err)
		}
		pub, err := kp.PublicKey()
		if err != nil {
			return nil, err
		}
		return []nats.Option{nats.Nkey(pub, kp.Sign)}, nil
	}
	return nil, fmt.Errorf("unknown credential type %q", c.Type)
}
