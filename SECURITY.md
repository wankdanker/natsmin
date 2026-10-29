# Security policy

natsmin holds credentials for NATS systems and can change them, so security reports are taken seriously.

## Reporting a vulnerability

Please report vulnerabilities privately through [GitHub security advisories](https://github.com/wankdanker/natsmin/security/advisories/new). Do not open a public issue.

Include the natsmin version, how to reproduce, and the impact you see. You should get a reply within a few days.

## Supported versions

Fixes go into the latest release. Before 1.0, only the most recent minor version is supported.

## Hardening checklist for deployments

- Serve natsmin over HTTPS (`NATSMIN_TLS_CERT`/`NATSMIN_TLS_KEY` or a reverse proxy) and set `NATSMIN_SECURE_COOKIES=true`.
- Set `NATSMIN_ADMIN_USER`/`NATSMIN_ADMIN_PASSWORD` or `NATSMIN_ALLOW_SETUP=false` before exposing a fresh instance, so nobody else can claim the first admin account.
- Back up `NATSMIN_SECRET_KEY` (or `secret.key` in the data directory); stored NATS credentials cannot be decrypted without it.
- Give natsmin's NATS users only the permissions they need; a read-only system account user is enough for monitoring.
