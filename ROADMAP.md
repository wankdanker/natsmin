# Roadmap

## Done (0.1)

- System registry with encrypted credentials; system account and per-account connections
- Live overview, topology (clusters, routes, gateways, leafnodes), servers, raw monitoring
- Connections across servers with kick and lame-duck; per-account traffic; subject interest lookup
- JetStream: meta group, cross-account stream placement, stepdowns, stream moves, server evacuation, peer removal
- Streams, consumers, message browser with republish/delete, KV with history and watch, object store
- Publish, request/reply, live tail
- Users, roles, API tokens, audit log; OpenAPI 3.1 for every operation

## Next

The backlog lives in [GitHub issues](https://github.com/wankdanker/natsmin/issues). Issues labelled [`roadmap`](https://github.com/wankdanker/natsmin/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap) are the planned features:

- [#1](https://github.com/wankdanker/natsmin/issues/1) OIDC login with group-to-role mapping
- [#2](https://github.com/wankdanker/natsmin/issues/2) Alerting with webhook, Slack and email targets
- [#3](https://github.com/wankdanker/natsmin/issues/3) / [#4](https://github.com/wankdanker/natsmin/issues/4) Operator mode: browse, then manage accounts, users and signing keys
- [#5](https://github.com/wankdanker/natsmin/issues/5) Per-system and per-account permissions
- [#6](https://github.com/wankdanker/natsmin/issues/6) Stream backup and restore
- [#7](https://github.com/wankdanker/natsmin/issues/7) Mirrors, sources, republish and subject transforms in the stream form
- [#8](https://github.com/wankdanker/natsmin/issues/8) Consumer inspection
- [#9](https://github.com/wankdanker/natsmin/issues/9) Persisted metrics history, [#10](https://github.com/wankdanker/natsmin/issues/10) Prometheus `/metrics`
- [#11](https://github.com/wankdanker/natsmin/issues/11) Guided server maintenance and rolling upgrades
- [#12](https://github.com/wankdanker/natsmin/issues/12) JetStream domains
- [#13](https://github.com/wankdanker/natsmin/issues/13) CLI, [#14](https://github.com/wankdanker/natsmin/issues/14) Terraform provider, [#15](https://github.com/wankdanker/natsmin/issues/15) MCP endpoint
- [#18](https://github.com/wankdanker/natsmin/issues/18) Homebrew, Helm and install script
