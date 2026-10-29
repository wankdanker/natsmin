package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Credential describes how to authenticate a NATS connection.
type Credential struct {
	// Type is one of none, userpass, token, creds, nkey.
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	// Creds is the content of a .creds file (JWT + nkey seed).
	Creds string `json:"creds,omitempty"`
	// NKeySeed is a user nkey seed (SU...).
	NKeySeed string `json:"nkeySeed,omitempty"`
}

// TLSConfig holds PEM encoded TLS material for a NATS connection.
type TLSConfig struct {
	CA                 string `json:"ca,omitempty"`
	Cert               string `json:"cert,omitempty"`
	Key                string `json:"key,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

// System is a set of connected NATS servers: a single server, a cluster or a
// supercluster, reached through one or more seed URLs.
type System struct {
	ID          string
	Name        string
	Description string
	URLs        []string
	TLS         *TLSConfig
	SystemAuth  *Credential
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Account is a named connection into one NATS account of a system, used for
// JetStream, KV, object store and message operations.
type Account struct {
	SystemID  string
	ID        string
	Name      string
	Auth      Credential
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) sealJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return s.box.Seal(data)
}

func (s *Store) openJSON(sealed string, v any) error {
	if sealed == "" {
		return nil
	}
	data, err := s.box.Open(sealed)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// ListSystems returns all registered systems.
func (s *Store) ListSystems(ctx context.Context) ([]System, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, urls, tls, system_auth, created_at, updated_at FROM systems ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []System{}
	for rows.Next() {
		sys, err := s.scanSystem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sys)
	}
	return out, rows.Err()
}

// GetSystem loads one system.
func (s *Store) GetSystem(ctx context.Context, id string) (*System, error) {
	return s.scanSystem(s.db.QueryRowContext(ctx, `SELECT id, name, description, urls, tls, system_auth, created_at, updated_at FROM systems WHERE id = ?`, id))
}

func (s *Store) scanSystem(row interface{ Scan(...any) error }) (*System, error) {
	var sys System
	var urls, tlsSealed, authSealed string
	var created, updated int64
	if err := row.Scan(&sys.ID, &sys.Name, &sys.Description, &urls, &tlsSealed, &authSealed, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(urls), &sys.URLs); err != nil {
		return nil, err
	}
	if tlsSealed != "" {
		sys.TLS = &TLSConfig{}
		if err := s.openJSON(tlsSealed, sys.TLS); err != nil {
			return nil, err
		}
	}
	if authSealed != "" {
		sys.SystemAuth = &Credential{}
		if err := s.openJSON(authSealed, sys.SystemAuth); err != nil {
			return nil, err
		}
	}
	sys.CreatedAt = time.Unix(created, 0).UTC()
	sys.UpdatedAt = time.Unix(updated, 0).UTC()
	return &sys, nil
}

// SaveSystem inserts or updates a system.
func (s *Store) SaveSystem(ctx context.Context, sys *System, create bool) error {
	urls, err := json.Marshal(sys.URLs)
	if err != nil {
		return err
	}
	var tlsSealed, authSealed string
	if sys.TLS != nil {
		if tlsSealed, err = s.sealJSON(sys.TLS); err != nil {
			return err
		}
	}
	if sys.SystemAuth != nil {
		if authSealed, err = s.sealJSON(sys.SystemAuth); err != nil {
			return err
		}
	}
	now := time.Now()
	if create {
		_, err = s.db.ExecContext(ctx, `INSERT INTO systems (id, name, description, urls, tls, system_auth, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			sys.ID, sys.Name, sys.Description, string(urls), tlsSealed, authSealed, now.Unix(), now.Unix())
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE systems SET name = ?, description = ?, urls = ?, tls = ?, system_auth = ?, updated_at = ? WHERE id = ?`,
		sys.Name, sys.Description, string(urls), tlsSealed, authSealed, now.Unix(), sys.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSystem removes a system and its accounts.
func (s *Store) DeleteSystem(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM systems WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAccounts returns the account connections of a system.
func (s *Store) ListAccounts(ctx context.Context, systemID string) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT system_id, id, name, auth, created_at, updated_at FROM accounts WHERE system_id = ? ORDER BY name`, systemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		a, err := s.scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetAccount loads one account connection.
func (s *Store) GetAccount(ctx context.Context, systemID, id string) (*Account, error) {
	return s.scanAccount(s.db.QueryRowContext(ctx, `SELECT system_id, id, name, auth, created_at, updated_at FROM accounts WHERE system_id = ? AND id = ?`, systemID, id))
}

func (s *Store) scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	var a Account
	var authSealed string
	var created, updated int64
	if err := row.Scan(&a.SystemID, &a.ID, &a.Name, &authSealed, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.openJSON(authSealed, &a.Auth); err != nil {
		return nil, err
	}
	a.CreatedAt = time.Unix(created, 0).UTC()
	a.UpdatedAt = time.Unix(updated, 0).UTC()
	return &a, nil
}

// SaveAccount inserts or updates an account connection.
func (s *Store) SaveAccount(ctx context.Context, a *Account, create bool) error {
	sealed, err := s.sealJSON(a.Auth)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if create {
		_, err = s.db.ExecContext(ctx, `INSERT INTO accounts (system_id, id, name, auth, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			a.SystemID, a.ID, a.Name, sealed, now, now)
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET name = ?, auth = ?, updated_at = ? WHERE system_id = ? AND id = ?`,
		a.Name, sealed, now, a.SystemID, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAccount removes an account connection.
func (s *Store) DeleteAccount(ctx context.Context, systemID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE system_id = ? AND id = ?`, systemID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
