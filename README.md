# Grammateus

Tenant management API server for [katastroma](https://github.com/katastroma).

Provisions tenant resources, manages tenant hierarchy, handles tenant source
event verification, and tenant resource queries.

## Authentication

**Open issue:** Auth architecture needs its own document. Key considerations:

- Initial tenant onboarding sets up tenant auth (IdP integration, tokens, etc.)
- Parent tenant auth can create child tenants (sub-tenant scoping)
- All grammateus API operations (onboarding, offboarding, child tenant creation,
  queries) are authenticated
- The auth mechanism determines who can operate on which tenant namespaces

## Queries

Tenant resource queryability is handled through grammateus's authenticated API.
Authorization determines what a tenant can query (including parent-to-child
tenant resource relationships) — this is an IdP/authorization concern (see
[Authentication](#authentication)).

Grammateus provides query endpoints for tenants to inspect their state:

- Resources provisioned in the cluster for a given tenant
- Namespace hierarchy and child tenants

Resource queries are backed by authorization — no separate state or inventory
tracking.
