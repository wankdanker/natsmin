package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/wankdanker/natsmin/internal/natsx"
	"github.com/wankdanker/natsmin/internal/store"
)

var tagSystems = []string{"Systems"}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// CredentialIn sets how natsmin authenticates to NATS. On update, leaving a
// secret field empty keeps the stored value.
type CredentialIn struct {
	Type     string `json:"type" enum:"none,userpass,token,creds,nkey"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
	Creds    string `json:"creds,omitempty" doc:"Contents of a .creds file"`
	NKeySeed string `json:"nkeySeed,omitempty" doc:"User nkey seed (SU...)"`
}

// CredentialOut describes stored credentials without revealing secrets.
type CredentialOut struct {
	Type        string `json:"type"`
	Username    string `json:"username,omitempty"`
	HasPassword bool   `json:"hasPassword"`
	HasToken    bool   `json:"hasToken"`
	HasCreds    bool   `json:"hasCreds"`
	HasNKey     bool   `json:"hasNKey"`
	// CredsUser is the user name or public key from the creds JWT.
	CredsUser    string `json:"credsUser,omitempty"`
	CredsAccount string `json:"credsAccount,omitempty"`
}

// TLSIn sets TLS material. On update, an empty key keeps the stored key.
type TLSIn struct {
	CA                 string `json:"ca,omitempty" doc:"PEM CA bundle"`
	Cert               string `json:"cert,omitempty" doc:"PEM client certificate"`
	Key                string `json:"key,omitempty" doc:"PEM client key"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

// TLSOut describes TLS settings without the private key.
type TLSOut struct {
	CA                 string `json:"ca,omitempty"`
	Cert               string `json:"cert,omitempty"`
	HasKey             bool   `json:"hasKey"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

// AccountOut is an account connection.
type AccountOut struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Auth      CredentialOut `json:"auth"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// SystemStatus is the live state of a system.
type SystemStatus struct {
	Monitored   bool       `json:"monitored" doc:"True when system account credentials are configured"`
	Connected   bool       `json:"connected"`
	Error       string     `json:"error,omitempty"`
	LastPoll    *time.Time `json:"lastPoll,omitempty"`
	Servers     int        `json:"servers"`
	Clusters    int        `json:"clusters"`
	Healthy     int        `json:"healthy"`
	Versions    []string   `json:"versions"`
	JetStream   bool       `json:"jetstream"`
	Conns       int        `json:"connections"`
	InMsgsRate  float64    `json:"inMsgsRate"`
	OutMsgsRate float64    `json:"outMsgsRate"`
}

// SystemOut is a registered NATS system.
type SystemOut struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	URLs        []string       `json:"urls"`
	TLS         *TLSOut        `json:"tls,omitempty"`
	SystemAuth  *CredentialOut `json:"systemAuth,omitempty"`
	Accounts    []AccountOut   `json:"accounts"`
	Status      SystemStatus   `json:"status"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// AccountIn creates or updates an account connection.
type AccountIn struct {
	ID   string        `json:"id,omitempty" doc:"Slug; derived from name when empty"`
	Name string        `json:"name" minLength:"1" maxLength:"100"`
	Auth *CredentialIn `json:"auth,omitempty"`
}

// SystemIn creates or updates a system. On update, omitted objects keep
// their stored values.
type SystemIn struct {
	ID               string        `json:"id,omitempty" doc:"Slug used in URLs; derived from name when empty. Cannot change."`
	Name             string        `json:"name" minLength:"1" maxLength:"100"`
	Description      string        `json:"description,omitempty" maxLength:"1000"`
	URLs             []string      `json:"urls" minItems:"1"`
	TLS              *TLSIn        `json:"tls,omitempty"`
	RemoveTLS        bool          `json:"removeTls,omitempty"`
	SystemAuth       *CredentialIn `json:"systemAuth,omitempty" doc:"System account credentials; required for topology, monitoring and maintenance"`
	RemoveSystemAuth bool          `json:"removeSystemAuth,omitempty"`
	Accounts         []AccountIn   `json:"accounts,omitempty" doc:"Account connections to create (create only)"`
}

func credOut(c *store.Credential) CredentialOut {
	o := CredentialOut{Type: c.Type, Username: c.Username, HasPassword: c.Password != "", HasToken: c.Token != "", HasCreds: c.Creds != "", HasNKey: c.NKeySeed != ""}
	if o.Type == "" {
		o.Type = "none"
	}
	if c.Creds != "" {
		if tok, err := jwt.ParseDecoratedJWT([]byte(c.Creds)); err == nil {
			if uc, err := jwt.DecodeUserClaims(tok); err == nil {
				o.CredsUser = uc.Name
				if o.CredsUser == "" {
					o.CredsUser = uc.Subject
				}
				o.CredsAccount = uc.IssuerAccount
				if o.CredsAccount == "" {
					o.CredsAccount = uc.Issuer
				}
			}
		}
	}
	return o
}

func mergeCred(old *store.Credential, in *CredentialIn) store.Credential {
	c := store.Credential{Type: in.Type, Username: in.Username, Password: in.Password, Token: in.Token, Creds: in.Creds, NKeySeed: in.NKeySeed}
	if old != nil && old.Type == in.Type {
		if c.Password == "" {
			c.Password = old.Password
		}
		if c.Token == "" {
			c.Token = old.Token
		}
		if c.Creds == "" {
			c.Creds = old.Creds
		}
		if c.NKeySeed == "" {
			c.NKeySeed = old.NKeySeed
		}
	}
	// Keep only the fields the type uses.
	switch c.Type {
	case "userpass":
		c.Token, c.Creds, c.NKeySeed = "", "", ""
	case "token":
		c.Username, c.Password, c.Creds, c.NKeySeed = "", "", "", ""
	case "creds":
		c.Username, c.Password, c.Token, c.NKeySeed = "", "", "", ""
	case "nkey":
		c.Username, c.Password, c.Token, c.Creds = "", "", "", ""
	default:
		c = store.Credential{Type: "none"}
	}
	return c
}

func validateCred(c store.Credential) error {
	switch c.Type {
	case "userpass":
		if c.Username == "" {
			return errors.New("username is required")
		}
	case "token":
		if c.Token == "" {
			return errors.New("token is required")
		}
	case "creds":
		if c.Creds == "" {
			return errors.New("creds file content is required")
		}
	case "nkey":
		if c.NKeySeed == "" {
			return errors.New("nkey seed is required")
		}
	}
	return nil
}

func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 63 {
		out = out[:63]
	}
	if out == "" {
		out = "system"
	}
	return out
}

func (s *Server) systemOut(ctx context.Context, sys *store.System) (SystemOut, error) {
	o := SystemOut{ID: sys.ID, Name: sys.Name, Description: sys.Description, URLs: sys.URLs, Accounts: []AccountOut{}, CreatedAt: sys.CreatedAt, UpdatedAt: sys.UpdatedAt}
	if sys.TLS != nil {
		o.TLS = &TLSOut{CA: sys.TLS.CA, Cert: sys.TLS.Cert, HasKey: sys.TLS.Key != "", InsecureSkipVerify: sys.TLS.InsecureSkipVerify}
	}
	if sys.SystemAuth != nil {
		c := credOut(sys.SystemAuth)
		o.SystemAuth = &c
	}
	accs, err := s.store.ListAccounts(ctx, sys.ID)
	if err != nil {
		return o, err
	}
	for _, a := range accs {
		o.Accounts = append(o.Accounts, AccountOut{ID: a.ID, Name: a.Name, Auth: credOut(&a.Auth), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt})
	}
	o.Status = SystemStatus{Monitored: sys.SystemAuth != nil, Versions: []string{}}
	if snap := s.mgr.Snapshot(sys.ID); snap != nil {
		t := snap.Time
		o.Status.LastPoll = &t
		o.Status.Connected = snap.Connected
		o.Status.Error = snap.Error
		o.Status.Servers = snap.Totals.Servers
		o.Status.Clusters = snap.Totals.Clusters
		o.Status.Versions = snap.Totals.Versions
		o.Status.JetStream = snap.JetStream != nil
		o.Status.Conns = snap.Totals.Connections
		o.Status.InMsgsRate = snap.Totals.InMsgsRate
		o.Status.OutMsgsRate = snap.Totals.OutMsgsRate
		for _, srv := range snap.Servers {
			if srv.Healthy && !srv.Missing {
				o.Status.Healthy++
			}
		}
		if o.Status.Versions == nil {
			o.Status.Versions = []string{}
		}
	}
	return o, nil
}

// SystemPath identifies a system.
type SystemPath struct {
	System string `path:"system" doc:"System id"`
}

// AccountPath identifies an account connection of a system.
type AccountPath struct {
	System  string `path:"system" doc:"System id"`
	Account string `path:"account" doc:"Account connection id"`
}

// TestResult reports a connection test.
type TestResult struct {
	OK              bool     `json:"ok"`
	Error           string   `json:"error,omitempty"`
	ServerName      string   `json:"serverName,omitempty"`
	ServerVersion   string   `json:"serverVersion,omitempty"`
	Cluster         string   `json:"cluster,omitempty"`
	ConnectedURL    string   `json:"connectedUrl,omitempty"`
	RTT             string   `json:"rtt,omitempty"`
	IsSystemAccount bool     `json:"isSystemAccount"`
	ServersFound    int      `json:"serversFound"`
	JetStream       bool     `json:"jetstream"`
	JetStreamDomain string   `json:"jetstreamDomain,omitempty"`
	DiscoveredURLs  []string `json:"discoveredUrls,omitempty"`
}

func (s *Server) registerSystems() {
	reg(s, route{method: http.MethodGet, path: "/v1/systems", id: "list-systems", summary: "List systems", tags: tagSystems, role: store.RoleViewer},
		func(ctx context.Context, _ *struct{}) (*struct{ Body []SystemOut }, error) {
			systems, err := s.store.ListSystems(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			out := []SystemOut{}
			for i := range systems {
				o, err := s.systemOut(ctx, &systems[i])
				if err != nil {
					return nil, apiErr(err)
				}
				out = append(out, o)
			}
			return &struct{ Body []SystemOut }{out}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}", id: "get-system", summary: "Get a system", tags: tagSystems, role: store.RoleViewer},
		func(ctx context.Context, in *SystemPath) (*struct{ Body SystemOut }, error) {
			sys, err := s.store.GetSystem(ctx, in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			o, err := s.systemOut(ctx, sys)
			return &struct{ Body SystemOut }{o}, apiErr(err)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems", id: "create-system", summary: "Register a system", tags: tagSystems, role: store.RoleAdmin, status: http.StatusCreated},
		func(ctx context.Context, in *struct{ Body SystemIn }) (*struct{ Body SystemOut }, error) {
			b := in.Body
			id := b.ID
			if id == "" {
				id = slugify(b.Name)
			}
			if !slugRe.MatchString(id) {
				return nil, huma.Error422UnprocessableEntity("id must be lowercase letters, digits and dashes")
			}
			sys := store.System{ID: id, Name: b.Name, Description: b.Description, URLs: cleanURLs(b.URLs)}
			if b.TLS != nil {
				sys.TLS = &store.TLSConfig{CA: b.TLS.CA, Cert: b.TLS.Cert, Key: b.TLS.Key, InsecureSkipVerify: b.TLS.InsecureSkipVerify}
			}
			if b.SystemAuth != nil {
				c := mergeCred(nil, b.SystemAuth)
				if err := validateCred(c); err != nil {
					return nil, huma.Error422UnprocessableEntity("systemAuth: " + err.Error())
				}
				sys.SystemAuth = &c
			}
			accounts := []store.Account{}
			for _, a := range b.Accounts {
				acc, err := accountFromIn(id, a, nil)
				if err != nil {
					return nil, err
				}
				accounts = append(accounts, acc)
			}
			if err := s.store.SaveSystem(ctx, &sys, true); err != nil {
				return nil, apiErr(err)
			}
			for i := range accounts {
				if err := s.store.SaveAccount(ctx, &accounts[i], true); err != nil {
					return nil, apiErr(err)
				}
			}
			saved, err := s.store.GetSystem(ctx, id)
			if err != nil {
				return nil, apiErr(err)
			}
			s.mgr.Reload(*saved)
			if saved.SystemAuth != nil {
				_, _ = s.mgr.PollNow(ctx, id)
			}
			o, err := s.systemOut(ctx, saved)
			return &struct{ Body SystemOut }{o}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/systems/{system}", id: "update-system", summary: "Update a system", tags: tagSystems, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			SystemPath
			Body SystemIn
		}) (*struct{ Body SystemOut }, error) {
			sys, err := s.store.GetSystem(ctx, in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			b := in.Body
			sys.Name, sys.Description, sys.URLs = b.Name, b.Description, cleanURLs(b.URLs)
			switch {
			case b.RemoveTLS:
				sys.TLS = nil
			case b.TLS != nil:
				key := b.TLS.Key
				if key == "" && sys.TLS != nil {
					key = sys.TLS.Key
				}
				sys.TLS = &store.TLSConfig{CA: b.TLS.CA, Cert: b.TLS.Cert, Key: key, InsecureSkipVerify: b.TLS.InsecureSkipVerify}
			}
			switch {
			case b.RemoveSystemAuth:
				sys.SystemAuth = nil
			case b.SystemAuth != nil:
				c := mergeCred(sys.SystemAuth, b.SystemAuth)
				if err := validateCred(c); err != nil {
					return nil, huma.Error422UnprocessableEntity("systemAuth: " + err.Error())
				}
				sys.SystemAuth = &c
			}
			if err := s.store.SaveSystem(ctx, sys, false); err != nil {
				return nil, apiErr(err)
			}
			saved, err := s.store.GetSystem(ctx, sys.ID)
			if err != nil {
				return nil, apiErr(err)
			}
			s.mgr.Reload(*saved)
			o, err := s.systemOut(ctx, saved)
			return &struct{ Body SystemOut }{o}, apiErr(err)
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/systems/{system}", id: "delete-system", summary: "Remove a system", tags: tagSystems, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *SystemPath) (*struct{}, error) {
			if err := s.store.DeleteSystem(ctx, in.System); err != nil {
				return nil, apiErr(err)
			}
			s.mgr.Remove(in.System)
			return nil, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/test", id: "test-connection", summary: "Test connection settings",
		desc: "Connects with the given settings without saving them. Pass systemId (and accountId) to reuse stored secrets for fields left empty.",
		tags: tagSystems, role: store.RoleAdmin, noAudit: true},
		func(ctx context.Context, in *struct {
			Body struct {
				SystemID  string        `json:"systemId,omitempty"`
				AccountID string        `json:"accountId,omitempty"`
				URLs      []string      `json:"urls" minItems:"1"`
				TLS       *TLSIn        `json:"tls,omitempty"`
				Auth      *CredentialIn `json:"auth,omitempty"`
			}
		}) (*struct{ Body TestResult }, error) {
			b := in.Body
			var oldCred *store.Credential
			var tlsCfg *store.TLSConfig
			if b.SystemID != "" {
				sys, err := s.store.GetSystem(ctx, b.SystemID)
				if err != nil {
					return nil, apiErr(err)
				}
				tlsCfg = sys.TLS
				oldCred = sys.SystemAuth
				if b.AccountID != "" {
					acc, err := s.store.GetAccount(ctx, b.SystemID, b.AccountID)
					if err != nil {
						return nil, apiErr(err)
					}
					oldCred = &acc.Auth
				}
			}
			if b.TLS != nil {
				key := b.TLS.Key
				if key == "" && tlsCfg != nil {
					key = tlsCfg.Key
				}
				tlsCfg = &store.TLSConfig{CA: b.TLS.CA, Cert: b.TLS.Cert, Key: key, InsecureSkipVerify: b.TLS.InsecureSkipVerify}
			}
			var cred *store.Credential
			if b.Auth != nil {
				c := mergeCred(oldCred, b.Auth)
				cred = &c
			}
			return &struct{ Body TestResult }{s.testConnection(ctx, cleanURLs(b.URLs), tlsCfg, cred)}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/systems/{system}/accounts", id: "list-accounts", summary: "List account connections", tags: tagSystems, role: store.RoleViewer},
		func(ctx context.Context, in *SystemPath) (*struct{ Body []AccountOut }, error) {
			if _, err := s.store.GetSystem(ctx, in.System); err != nil {
				return nil, apiErr(err)
			}
			accs, err := s.store.ListAccounts(ctx, in.System)
			if err != nil {
				return nil, apiErr(err)
			}
			out := []AccountOut{}
			for _, a := range accs {
				out = append(out, AccountOut{ID: a.ID, Name: a.Name, Auth: credOut(&a.Auth), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt})
			}
			return &struct{ Body []AccountOut }{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/systems/{system}/accounts", id: "create-account", summary: "Add an account connection", tags: tagSystems, role: store.RoleAdmin, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			SystemPath
			Body AccountIn
		}) (*struct{ Body AccountOut }, error) {
			if _, err := s.store.GetSystem(ctx, in.System); err != nil {
				return nil, apiErr(err)
			}
			acc, err := accountFromIn(in.System, in.Body, nil)
			if err != nil {
				return nil, err
			}
			if err := s.store.SaveAccount(ctx, &acc, true); err != nil {
				return nil, apiErr(err)
			}
			saved, err := s.store.GetAccount(ctx, in.System, acc.ID)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body AccountOut }{AccountOut{ID: saved.ID, Name: saved.Name, Auth: credOut(&saved.Auth), CreatedAt: saved.CreatedAt, UpdatedAt: saved.UpdatedAt}}, nil
		})

	reg(s, route{method: http.MethodPut, path: "/v1/systems/{system}/accounts/{account}", id: "update-account", summary: "Update an account connection", tags: tagSystems, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			AccountPath
			Body AccountIn
		}) (*struct{ Body AccountOut }, error) {
			old, err := s.store.GetAccount(ctx, in.System, in.Account)
			if err != nil {
				return nil, apiErr(err)
			}
			in.Body.ID = in.Account
			acc, err := accountFromIn(in.System, in.Body, &old.Auth)
			if err != nil {
				return nil, err
			}
			if err := s.store.SaveAccount(ctx, &acc, false); err != nil {
				return nil, apiErr(err)
			}
			s.mgr.ReloadAccount(in.System, in.Account)
			saved, err := s.store.GetAccount(ctx, in.System, in.Account)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body AccountOut }{AccountOut{ID: saved.ID, Name: saved.Name, Auth: credOut(&saved.Auth), CreatedAt: saved.CreatedAt, UpdatedAt: saved.UpdatedAt}}, nil
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/systems/{system}/accounts/{account}", id: "delete-account", summary: "Remove an account connection", tags: tagSystems, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *AccountPath) (*struct{}, error) {
			if err := s.store.DeleteAccount(ctx, in.System, in.Account); err != nil {
				return nil, apiErr(err)
			}
			s.mgr.ReloadAccount(in.System, in.Account)
			return nil, nil
		})
}

func accountFromIn(systemID string, a AccountIn, old *store.Credential) (store.Account, error) {
	id := a.ID
	if id == "" {
		id = slugify(a.Name)
	}
	if !slugRe.MatchString(id) {
		return store.Account{}, huma.Error422UnprocessableEntity("account id must be lowercase letters, digits and dashes")
	}
	acc := store.Account{SystemID: systemID, ID: id, Name: a.Name, Auth: store.Credential{Type: "none"}}
	if old != nil {
		acc.Auth = *old
	}
	if a.Auth != nil {
		acc.Auth = mergeCred(old, a.Auth)
		if err := validateCred(acc.Auth); err != nil {
			return store.Account{}, huma.Error422UnprocessableEntity("auth: " + err.Error())
		}
	}
	return acc, nil
}

func cleanURLs(in []string) []string {
	out := []string{}
	for _, u := range in {
		for _, p := range strings.Split(u, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func (s *Server) testConnection(ctx context.Context, urls []string, tlsCfg *store.TLSConfig, cred *store.Credential) TestResult {
	nc, err := natsx.Connect("natsmin (test)", urls, tlsCfg, cred, 5*time.Second)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	defer nc.Close()
	r := TestResult{OK: true, ServerName: nc.ConnectedServerName(), ServerVersion: nc.ConnectedServerVersion(),
		Cluster: nc.ConnectedClusterName(), ConnectedURL: nc.ConnectedUrlRedacted(), DiscoveredURLs: nc.DiscoveredServers()}
	if rtt, err := nc.RTT(); err == nil {
		r.RTT = rtt.String()
	}
	if resp, err := natsx.NewSysClient(nc).Ping(ctx, "IDZ", nil, 1500*time.Millisecond); err == nil && len(resp) > 0 {
		r.IsSystemAccount = true
		r.ServersFound = len(resp)
	}
	if js, err := jetstream.New(nc); err == nil {
		actx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if info, err := js.AccountInfo(actx); err == nil {
			r.JetStream = true
			r.JetStreamDomain = info.Domain
		}
	}
	return r
}
