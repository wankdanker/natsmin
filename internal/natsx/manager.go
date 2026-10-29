package natsx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/wankdanker/natsmin/internal/store"
)

// ErrNoSystemAuth is returned for monitoring calls on a system registered
// without system account credentials.
var ErrNoSystemAuth = errors.New("this system has no system account credentials configured")

// Options configure a Manager.
type Options struct {
	PollInterval   time.Duration
	HistoryPoints  int
	RequestTimeout time.Duration
	ClientName     string
}

// Manager owns the NATS connections for every registered system and keeps
// the latest snapshot and rate history of each.
type Manager struct {
	store *store.Store
	opts  Options
	log   *slog.Logger

	mu      sync.Mutex
	systems map[string]*systemState
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

type systemState struct {
	mu       sync.Mutex
	sys      store.System
	sysConn  *nats.Conn
	sysCl    *SysClient
	accounts map[string]*nats.Conn
	snapshot *Snapshot
	history  []RatePoint
	subs     map[chan *Snapshot]struct{}
	cancel   context.CancelFunc
	lastErr  string
}

// NewManager creates a Manager.
func NewManager(st *store.Store, opts Options, log *slog.Logger) *Manager {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.HistoryPoints <= 0 {
		opts.HistoryPoints = 720
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 3 * time.Second
	}
	if opts.ClientName == "" {
		opts.ClientName = "natsmin"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{store: st, opts: opts, log: log, systems: map[string]*systemState{}, ctx: ctx, cancel: cancel}
}

// Options returns the manager's settings.
func (m *Manager) Options() Options { return m.opts }

// Start begins polling every registered system.
func (m *Manager) Start(ctx context.Context) error {
	systems, err := m.store.ListSystems(ctx)
	if err != nil {
		return err
	}
	for _, s := range systems {
		m.Reload(s)
	}
	return nil
}

// Stop closes all connections.
func (m *Manager) Stop() {
	m.cancel()
	m.mu.Lock()
	for id, st := range m.systems {
		st.close()
		delete(m.systems, id)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// Reload (re)starts a system after it is created or its settings change.
func (m *Manager) Reload(sys store.System) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.systems[sys.ID]; ok {
		old.close()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	st := &systemState{sys: sys, accounts: map[string]*nats.Conn{}, subs: map[chan *Snapshot]struct{}{}, cancel: cancel}
	if old, ok := m.systems[sys.ID]; ok {
		// Keep history and subscribers across a settings change.
		old.mu.Lock()
		st.history = old.history
		for ch := range old.subs {
			st.subs[ch] = struct{}{}
		}
		old.mu.Unlock()
	}
	m.systems[sys.ID] = st
	if sys.SystemAuth != nil {
		m.wg.Add(1)
		go m.pollLoop(ctx, st)
	}
}

// Remove stops and forgets a system.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.systems[id]; ok {
		st.close()
		delete(m.systems, id)
	}
}

// ReloadAccount drops a cached account connection so it reconnects with new settings.
func (m *Manager) ReloadAccount(systemID, accountID string) {
	st := m.state(systemID)
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if nc, ok := st.accounts[accountID]; ok {
		nc.Close()
		delete(st.accounts, accountID)
	}
}

func (st *systemState) close() {
	st.cancel()
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.sysConn != nil {
		st.sysConn.Close()
		st.sysConn = nil
		st.sysCl = nil
	}
	for id, nc := range st.accounts {
		nc.Close()
		delete(st.accounts, id)
	}
}

func (m *Manager) state(id string) *systemState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.systems[id]
}

// Snapshot returns the latest snapshot of a system, or nil before the first poll.
func (m *Manager) Snapshot(id string) *Snapshot {
	st := m.state(id)
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.snapshot
}

// Status reports whether a system is connected and its last error.
func (m *Manager) Status(id string) (connected bool, lastErr string) {
	st := m.state(id)
	if st == nil {
		return false, ""
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.sysConn != nil && st.sysConn.IsConnected(), st.lastErr
}

// History returns the rate history of a system.
func (m *Manager) History(id string) []RatePoint {
	st := m.state(id)
	if st == nil {
		return []RatePoint{}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]RatePoint, len(st.history))
	copy(out, st.history)
	return out
}

// Subscribe returns a channel that receives each new snapshot of a system.
func (m *Manager) Subscribe(id string) (<-chan *Snapshot, func()) {
	ch := make(chan *Snapshot, 4)
	st := m.state(id)
	if st == nil {
		close(ch)
		return ch, func() {}
	}
	st.mu.Lock()
	st.subs[ch] = struct{}{}
	st.mu.Unlock()
	return ch, func() {
		// Look the state up again: Reload may have replaced it.
		if cur := m.state(id); cur != nil {
			cur.mu.Lock()
			delete(cur.subs, ch)
			cur.mu.Unlock()
		}
		st.mu.Lock()
		delete(st.subs, ch)
		st.mu.Unlock()
	}
}

// Sys returns a system account client, connecting if needed.
func (m *Manager) Sys(id string) (*SysClient, error) {
	st := m.state(id)
	if st == nil {
		return nil, store.ErrNotFound
	}
	return m.sysClient(st)
}

func (m *Manager) sysClient(st *systemState) (*SysClient, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.sys.SystemAuth == nil {
		return nil, ErrNoSystemAuth
	}
	if st.sysCl != nil {
		return st.sysCl, nil
	}
	nc, err := Connect(m.opts.ClientName+" (system)", st.sys.URLs, st.sys.TLS, st.sys.SystemAuth, m.opts.RequestTimeout)
	if err != nil {
		st.lastErr = err.Error()
		return nil, err
	}
	st.sysConn = nc
	st.sysCl = NewSysClient(nc)
	st.sysCl.Expected = func() int {
		st.mu.Lock()
		defer st.mu.Unlock()
		if st.snapshot == nil {
			return 0
		}
		return st.snapshot.Totals.Servers
	}
	return st.sysCl, nil
}

// Account returns a connection into the named account of a system.
func (m *Manager) Account(ctx context.Context, systemID, accountID string) (*nats.Conn, error) {
	st := m.state(systemID)
	if st == nil {
		return nil, store.ErrNotFound
	}
	st.mu.Lock()
	if nc, ok := st.accounts[accountID]; ok && !nc.IsClosed() {
		st.mu.Unlock()
		return nc, nil
	}
	sys := st.sys
	st.mu.Unlock()

	acc, err := m.store.GetAccount(ctx, systemID, accountID)
	if err != nil {
		return nil, err
	}
	nc, err := Connect(fmt.Sprintf("%s (%s)", m.opts.ClientName, acc.Name), sys.URLs, sys.TLS, &acc.Auth, m.opts.RequestTimeout)
	if err != nil {
		return nil, err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if existing, ok := st.accounts[accountID]; ok && !existing.IsClosed() {
		nc.Close()
		return existing, nil
	}
	st.accounts[accountID] = nc
	return nc, nil
}

// JetStream returns a JetStream context for an account.
func (m *Manager) JetStream(ctx context.Context, systemID, accountID string) (jetstream.JetStream, error) {
	nc, err := m.Account(ctx, systemID, accountID)
	if err != nil {
		return nil, err
	}
	return jetstream.New(nc)
}

func (m *Manager) pollLoop(ctx context.Context, st *systemState) {
	defer m.wg.Done()
	t := time.NewTicker(m.opts.PollInterval)
	defer t.Stop()
	for {
		m.pollOnce(ctx, st)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PollNow runs one poll immediately and returns the new snapshot.
func (m *Manager) PollNow(ctx context.Context, id string) (*Snapshot, error) {
	st := m.state(id)
	if st == nil {
		return nil, store.ErrNotFound
	}
	if st.sys.SystemAuth == nil {
		return nil, ErrNoSystemAuth
	}
	return m.pollOnce(ctx, st), nil
}

func (m *Manager) pollOnce(ctx context.Context, st *systemState) *Snapshot {
	now := time.Now()
	cl, err := m.sysClient(st)
	var snap *Snapshot
	if err != nil {
		snap = m.failedSnapshot(st, now, err)
	} else {
		var res pollResult
		var wg sync.WaitGroup
		timeout := m.opts.RequestTimeout
		reqs := []struct {
			kind string
			opts any
			dst  *[]APIResponse
		}{
			{"VARZ", nil, &res.varz},
			{"ROUTEZ", nil, &res.routez},
			{"GATEWAYZ", nil, &res.gatewayz},
			{"LEAFZ", nil, &res.leafz},
			{"JSZ", nil, &res.jsz},
			{"HEALTHZ", nil, &res.healthz},
		}
		var errMu sync.Mutex
		var firstErr error
		for _, r := range reqs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := cl.Ping(ctx, r.kind, r.opts, timeout)
				*r.dst = out
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			}()
		}
		wg.Wait()
		if ctx.Err() != nil {
			return nil
		}
		if len(res.varz) == 0 {
			if firstErr == nil {
				firstErr = errors.New("no servers answered; check that the credentials belong to the system account")
				if !cl.Conn().IsConnected() {
					firstErr = errors.New("not connected: " + cl.Conn().Status().String())
				}
			}
			snap = m.failedSnapshot(st, now, firstErr)
		} else {
			st.mu.Lock()
			prev := st.snapshot
			st.mu.Unlock()
			snap = buildSnapshot(st.sys.ID, now, res, prev)
		}
	}

	st.mu.Lock()
	st.snapshot = snap
	st.lastErr = snap.Error
	if snap.Connected {
		st.history = append(st.history, RatePoint{
			Time: now, InMsgsRate: snap.Totals.InMsgsRate, OutMsgsRate: snap.Totals.OutMsgsRate,
			InBytesRate: snap.Totals.InBytesRate, OutBytesRate: snap.Totals.OutBytesRate,
			Connections: snap.Totals.Connections, Subscriptions: snap.Totals.Subscriptions, Servers: snap.Totals.Servers,
		})
		if over := len(st.history) - m.opts.HistoryPoints; over > 0 {
			st.history = append([]RatePoint(nil), st.history[over:]...)
		}
	}
	for ch := range st.subs {
		select {
		case ch <- snap:
		default:
		}
	}
	st.mu.Unlock()
	return snap
}

func (m *Manager) failedSnapshot(st *systemState, now time.Time, err error) *Snapshot {
	st.mu.Lock()
	prev := st.snapshot
	st.mu.Unlock()
	snap := &Snapshot{SystemID: st.sys.ID, Time: now, Error: err.Error(),
		Servers: []ServerNode{}, Clusters: []ClusterSummary{}, Routes: []RouteEdge{}, Gateways: []GatewayEdge{}, Leafnodes: []LeafEdge{}}
	if prev != nil {
		// Keep the last known layout so the UI can still show it, marked stale.
		for _, s := range prev.Servers {
			s.Missing = true
			s.Healthy = false
			snap.Servers = append(snap.Servers, s)
		}
		snap.Clusters = prev.Clusters
	}
	if m.log != nil {
		m.log.Warn("poll failed", "system", st.sys.ID, "err", err)
	}
	return snap
}
