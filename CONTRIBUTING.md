# Contributing

Thanks for helping. Issues and pull requests are welcome.

## Setup

Go 1.26+, Node 22+ and pnpm are needed. Docker is not.

```bash
make devcluster
```

```bash
make dev
```

```bash
cd ui && pnpm install && pnpm dev
```

## Workflow

- Pick an issue (the [roadmap issues](https://github.com/wankdanker/natsmin/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap) are the backlog) or open one first for larger changes.
- Branch from `main` (`feature/oidc-login`, `fix/connz-paging`) and open a pull request. `main` only changes through pull requests.
- CI must pass: Go tests and race detector, golangci-lint, svelte-check, a snapshot release build and the Docker build.
- Releases are cut from `main` by tagging; see [RELEASING.md](RELEASING.md).

## Before you open a pull request

```bash
make check
```

```bash
make test
```

- API changes need a test in `internal/api/api_test.go` and a regenerated spec (`make openapi`). CI fails if `docs/openapi.json` is stale.
- Keep JetStream configs in native NATS JSON rather than inventing new shapes, so natsmin keeps up with server releases.
- Destructive actions in the UI need a confirmation; irreversible ones (deleting streams, buckets, peers) use type-to-confirm.
- Follow the style of the surrounding code. Go is `gofmt`ed; Svelte uses runes.

## Reporting security issues

Please do not open public issues for vulnerabilities. Email the maintainer listed on the GitHub profile instead.
