# Grammateus

Tenant management API server for [katastroma](https://github.com/katastroma).
Manages tenant namespaces and attaches Applications and RepoCredentials to them.

## What It Does

Grammateus handles:

- Creating, updating, and deleting tenant namespaces
- Attaching and detaching Application resources to tenant namespaces
- Attaching and detaching RepoCredential resources to tenant namespaces
- Receiving webhooks from tenant repositories

CRD types are defined in [tropis](https://github.com/katastroma/tropis).
[Pedalion](https://github.com/katastroma/pedalion) watches and reconciles the
Application resources that grammateus creates.

## Ecosystem

- **Grammateus** (this) — the API server
- **[Prora](https://github.com/katastroma/prora)** — the tenant self-service
  frontend, consumes this API
