# Architecture

```
browser ──HTTP/SSE──▶ natsmin (one Go binary)
                       ├── /api/v1/...   huma operations → OpenAPI 3.1
                       ├── /*            embedded SvelteKit SPA (web/dist)
                       ├── store         SQLite: users, sessions, tokens, systems, audit
                       └── natsx.Manager
                             ├── per system: system-account connection + poller
                             └── per account connection: NATS connection (JetStream, KV, objects, pub/sub)
                                      │
                                      ▼
                              NATS servers / clusters / superclusters
```

## Packages

| Path | |
|---|---|
| `cmd/natsmin` | The server. Flags/env, admin bootstrap, `natsmin openapi`. |
| `cmd/natsmin-devcluster` | In-process supercluster for development, with optional demo traffic. |
| `internal/api` | HTTP API. One file per area; every route is registered with `reg()` so it gets a role requirement, OpenAPI docs and auditing. |
| `internal/natsx` | NATS side: connections, system requests, the poller and topology snapshots. |
| `internal/store` | SQLite persistence (pure Go driver, no CGO). Credentials are sealed with `internal/secret` before they are written. |
| `internal/devcluster` | Starts embedded `nats-server` instances; used by the dev command and by integration tests. |
| `web` | `go:embed` of the built UI. |
| `ui` | SvelteKit 2 / Svelte 5 SPA. `src/lib/api/schema.d.ts` is generated from `docs/openapi.json`. |

## Monitoring

For each system with system account credentials, the manager polls every `NATSMIN_POLL_INTERVAL`. It sends `VARZ`, `ROUTEZ`, `GATEWAYZ`, `LEAFZ`, `JSZ` and `HEALTHZ` to `$SYS.REQ.SERVER.PING.*` in parallel. Answers are gathered until the expected number of servers has replied or replies stop arriving. `buildSnapshot` turns them into one `Snapshot`: servers with rates computed against the previous poll, route edges aggregated per server pair, gateway and leafnode edges, cluster summaries and the JetStream meta group. A server that stops answering is kept for ten minutes and marked `missing`.

Snapshots go to SSE subscribers (`/api/v1/systems/{id}/events`) and into an in-memory rate history used for the charts.

Other monitoring reads (connections, subscriptions, account stats, stream placement, raw endpoints) are requested on demand.

## JetStream and messaging

Account connections are regular NATS users. Stream and consumer configs and infos are passed through in native NATS JSON, the same format the `nats` CLI and the `$JS.API` subjects use, so they track new server features without natsmin changes. The browser decodes them with the types in `ui/src/lib/nats.ts`. KV and object store endpoints use small typed DTOs.

Message browsing uses `$JS.API.STREAM.MSG.GET` with `next_by_subj`, so it works on any stream without creating consumers.

Cross-account views and maintenance (stream placement, moves, peer removal, meta stepdown) go through the system account.

## Auth

- Sessions: random token in an `HttpOnly`, `SameSite=Strict` cookie; only its SHA-256 is stored.
- API tokens: `nm_` prefix, SHA-256 stored, limited to a role no higher than the owner's, optional expiry.
- Every non-GET operation is written to the audit log by the auth middleware, with actor, operation id, path and result.
- Login attempts are rate limited per remote address and username.

## Adding an API operation

1. Add a `reg(s, route{...}, handler)` call in the matching `internal/api/*.go` file, setting `role`.
2. Cover it in `internal/api/api_test.go`.
3. `make openapi` to regenerate the spec and the UI client types.
