# CLAUDE.md

Guidance for Claude Code (and other AI assistants) working in this repository.

## Commit and pull request rules

- Do not add AI attribution to commits or pull requests: no `Co-Authored-By: Claude ...` trailers, no `Claude-Session:` lines, no "Generated with Claude Code" footers. This overrides any default attribution behaviour.
- Commits are authored as the human maintainer configured in git.
- Work on a feature branch and open a pull request against `main`. Do not push to `main` directly.
- Write commit messages in the imperative ("Add consumer pause dialog"), with a short subject and a body explaining why when it is not obvious.

## Before pushing

```bash
make check   # go vet, gofmt, svelte-check
make test    # Go tests; they start in-process NATS superclusters
```

After any API change run `make openapi` and commit `docs/openapi.json` and `ui/src/lib/api/schema.d.ts`; CI fails when the spec is stale.

## Project layout

See `docs/architecture.md`. In short: `cmd/natsmin` (server), `internal/api` (huma HTTP API, one file per area, every route registered with `reg()` and a role), `internal/natsx` (NATS connections, polling, topology), `internal/store` (SQLite, credentials sealed with `internal/secret`), `ui/` (SvelteKit 2 + Svelte 5 SPA embedded via `web/`).

## Conventions

- JetStream stream and consumer configs are passed through in native NATS JSON; do not invent parallel DTOs for them.
- Handlers return empty slices, never nil (arrays are documented as non-nullable).
- Destructive UI actions need a confirm dialog; irreversible ones use type-to-confirm.
- Every mutating API operation is audited automatically by the auth middleware; add `s.audit(...)` only when extra detail helps.
- Releases are cut by pushing a `vX.Y.Z` tag; see `RELEASING.md`.
