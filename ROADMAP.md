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

- **OIDC login** (GitHub, Google, Okta, Keycloak) with group-to-role mapping
- **Alerting**: threshold rules on any snapshot metric (lagging replicas, slow consumers, consumer backlog, storage) with webhook, Slack and email targets, plus an alert history
- **Operator mode management**: browse operators, accounts and users from JWTs; create and revoke users and signing keys; push account JWTs to the resolver
- **Per-system and per-account permissions** for users and tokens
- **Stream backup and restore** to a file or object store bucket
- **Mirrors and sources editor** in the stream form; subject transform previews
- **Consumer message inspection**: show what is pending and redelivered for a consumer
- **Persisted metrics history** (optional) and a Prometheus `/metrics` endpoint for natsmin itself
- **Guided rolling upgrade**: lame-duck a server, wait for clients and replicas to move, verify health, continue
- **JetStream domains**: target a domain from an account connection (leafnode JetStream)
- **CLI** generated from the OpenAPI document, and a Terraform provider
- **MCP endpoint** so AI agents can operate systems through the same API and roles
