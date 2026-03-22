# Grammateus

Tenant management API server for [katastroma](https://github.com/katastroma).

Manages tenant namespaces, their hierarchy, service accounts, webhook
verification, and tenant resource queries.

## Multi-Cluster

**Open issue:** Should be a straightforward solve but may have to track
ClusterIdentity inside the tenant namespace.

The architecture explained here should support this - during tenant onboarding,
the tenant will provide what cluster the platform should install resources to
and provide access to that cluster.

This would likely require bringing up the GitOps services (pharos, phortizo,
orpheus, and histia) to those clusters before hand(?). Otherwise tenant
onboarding could possibly do it given sufficient access/permissions.

Once those prerequisites are installed, provisioning to that cluster for that
GitOps identity is just a matter of passing the ClusterIdentity (cluster server
and credentials) to histia for it to provision.

## Multi-Tenancy

### Authentication

**Open issue:** Auth architecture needs its own document. Key considerations:

- Initial tenant onboarding sets up tenant auth (IdP integration, tokens, etc.)
- Parent tenant auth can create child tenants (sub-tenant scoping)
- All grammateus API operations (onboarding, offboarding, child tenant creation,
  queries) are authenticated
- The auth mechanism determines who can operate on which tenant namespaces

### Onboarding

1. Tenant registers through the API (or via
   [prora](https://github.com/katastroma/prora))
2. Grammateus creates a namespace for the tenant
3. Grammateus creates a deployer ServiceAccount in the tenant namespace with:
   - A ClusterRole granting `create`, `patch`, and `delete` on all resources
     (`*`). `create` and `patch` for SSA, `delete` for pruning. No read verbs.
   - A ClusterRoleBinding binding the SA to the ClusterRole.
   - Gatekeeper constrains where the SA can operate by prefix.
4. Grammateus generates a webhook secret and stores it in the tenant namespace.
   Tenant configures git webhooks on their repositories with this secret to send
   push events to [pharos](https://github.com/katastroma/pharos).
5. Tenant registers gitops identities and repository credentials through
   [phortizo](https://github.com/katastroma/phortizo).

All resources grammateus creates — namespace, ServiceAccount, ClusterRole,
ClusterRoleBinding, webhook secret — are labeled with the tenant identity.
Resources phortizo creates (gitops identities, repo credentials) are also
labeled with the tenant identity. This allows histia to find and prune all
tenant resources (including cluster-scoped ones) during offboarding.

### Offboarding

1. Grammateus walks the tenant hierarchy bottom-up, telling histia to prune all
   resources labeled with each child tenant's identity (deepest children first)
2. Grammateus tells histia to prune all resources labeled with the root tenant's
   identity

All tenant resources — namespaces, workload namespaces, ClusterRoles,
ClusterRoleBindings, Secrets, workloads — are deleted by the prune because they
are all labeled with the tenant identity.

### Namespace Hierarchy

Tenants can create child tenants as part of onboarding, authenticated and
authorized through grammateus's API. Child namespaces have an ownerReference
pointing to their parent namespace. Grammateus uses ownerReferences to track the
hierarchy for queries, offboarding, and authorization.

### Tenant SA Permissions

The deployer SA is used by histia for provisioning and pruning only. Its
ClusterRole grants `create`, `patch`, and `delete`. Cross-tenant read isolation
is enforced by the absence of RoleBindings.

### Tenant Workload SA Permissions

Tenants can provision SAs in their workload namespaces as part of their
manifests — for interactive access (bastion SAs), for workload identity, or any
other purpose. Histia deploys these like any other resource.

Any SA a tenant provisions is isolated by three layers:

1. Kubernetes RBAC — RoleBindings are namespace-scoped. An SA in `acme-prod` has
   zero permissions in any other namespace unless a RoleBinding exists for it
   there.
2. Gatekeeper prefix enforcement — the deployer SA can only create SAs and
   RoleBindings in the tenant's own prefixed namespaces. A tenant cannot
   provision RBAC in another tenant's namespace.
3. RBAC escalation prevention — no SA can be granted broader permissions than
   the deployer SA that created it.

### Webhook Verification

Grammateus verifies webhook HMAC signatures and returns the tenant identity.

When a push occurs, the git provider fires all webhooks configured on the repo.
Each webhook hits [pharos](https://github.com/katastroma/pharos) with its own
HMAC signature. Pharos verifies each signature through grammateus, identifying
the tenant.

## Queries

Tenant resource queryability is handled through grammateus's authenticated API.
Authorization determines what a tenant can query (including parent-to-child
tenant resource relationships) — this is an IdP/authorization concern (see
[Authentication](#authentication)).

Grammateus provides query endpoints for tenants to inspect their state:

- Resources provisioned in the cluster for a given tenant
- Namespace hierarchy and child tenants

Resource queries are backed by authorization using label selectors against the
cluster — no separate state or inventory tracking.

## GitOps Pipeline

1. Git push → git provider fires all webhooks configured on the repo
2. [Pharos](https://github.com/katastroma/pharos) receives an event with
   HMAC-SHA256 signature
3. Pharos asks grammateus to verify the signature and return the tenant identity
4. Pharos asks [phortizo](https://github.com/katastroma/phortizo) (retriever) to
   match the push against registered gitops identities — skips if no match
5. Pharos calls phortizo to fetch the source
6. Pharos calls [orpheus](https://github.com/katastroma/orpheus) (renderer) —
   renders manifests from the source
7. Pharos calls [histia](https://github.com/katastroma/histia) (provisioner) —
   applies manifests to the cluster impersonating the tenant's deployer SA,
   prunes resources no longer in the rendered output

## Tenant Isolation

### Impersonation

Histia provisions resources using native Kubernetes impersonation
(Impersonate-User HTTP headers). When applying manifests for a tenant, histia
impersonates the tenant's deployer SA. The impersonated SA has a ClusterRole
with `create`, `patch`, and `delete` on all resources, but Gatekeeper constrains
where those permissions apply based on namespace prefix. Kubernetes RBAC
escalation prevention ensures tenants cannot grant themselves broader
permissions than their SA has.

### Namespace Enforcement

Gatekeeper enforces namespace prefix conventions. Each tenant's deployer SA can
only create namespaces matching the tenant's prefix (e.g. `acme-*`). This
prevents namespace collisions between tenants.

The Gatekeeper policy derives the tenant prefix from the SA identity. The SA
username in Kubernetes is `system:serviceaccount:<namespace>:<name>`. The policy
validates:

- The SA namespace starts with `tenant-` (anchors the SA to a tenant namespace)
- The SA name prefix matches the namespace suffix (`tenant-acme` →
  `acme-deployer` → prefix `acme`)

This prevents rogue SAs outside `tenant-*` namespaces from getting prefix
enforcement. Grammateus must follow the naming convention `<prefix>-deployer`
when creating SAs.

**Open issue:** This relies on no entity other than grammateus being able to
create SAs in `tenant-*` namespaces. A Gatekeeper policy restricting SA creation
in `tenant-*` namespaces to grammateus's own SA would close this loop, but adds
another policy layer.

### Cluster-Scoped Resource Restriction

The deployer SA's ClusterRole uses `*` for resources so tenants can provision
CRs from any CRD. RBAC is additive only — there are no deny rules. Gatekeeper
blocks tenant deployer SAs from creating dangerous cluster-scoped resources:

- ClusterRoles
- ClusterRoleBindings

The policy checks: if the requesting SA is a tenant deployer (namespace starts
with `tenant-`) and the resource is a ClusterRole or ClusterRoleBinding —
reject.

**Open issue:** CRDs are cluster-scoped. Blocking tenant CRD creation prevents
tenants from installing operators or charts that include CRDs. Allowing it means
CRDs are visible cluster-wide to all tenants. CRD groups are defined by the CRD
spec itself — tenants cannot prefix third-party CRD groups without forking.
Mitigation TBD.

### What Gatekeeper prevents

- Any tenant deployer SA operating outside its namespace prefix → rejected
- Any tenant deployer SA creating ClusterRoles or ClusterRoleBindings → rejected
- globex-deployer creating namespace acme-prod → rejected (prefix mismatch)
- globex-deployer creating resources in acme-prod → rejected (prefix mismatch)

### Resource Ownership

Every tenant resource is labeled with the tenant identity. Resources provisioned
by histia are additionally labeled with the platform labels
([see `katartismos`](https://github.com/katastroma/katartismos)). These labels
are the ownership record — used for pruning, querying, and isolation
enforcement.

### Example Enforcement Flow

1. Grammateus onboards tenant ACME → creates tenant-acme namespace,
   acme-deployer SA with ClusterRole, webhook secret. ACME registers gitops
   identities and credentials through phortizo. Gatekeeper's cluster-wide policy
   automatically enforces the acme-\* prefix.
2. ACME pushes code → webhook fires → pharos receives event → ... → histia
   applies impersonating acme-deployer → resources labeled with tenant + GitOps
   identity
3. GLOBEX's deployer tries to create resources in acme-prod → Gatekeeper rejects
   (globex-deployer prefix doesn't match acme-\*)

### Diagram

```
CLUSTER
  ├── platform namespace
  │   ├── grammateus (tenant API server)
  │   ├── pharos (webhook server — receives git events)
  │   ├── phortizo (retriever — fetches source)
  │   ├── orpheus (renderer — renders manifests from source)
  │   ├── histia (provisioner — applies/prunes resources via impersonation)
  │   └── gatekeeper (admission control)
  │
  ├── CLUSTER-SCOPED RBAC (per tenant, created by grammateus)
  │   ├── ClusterRole: acme-deployer (create, patch, delete on *)
  │   ├── ClusterRoleBinding: acme-deployer → SA tenant-acme/acme-deployer
  │   ├── ClusterRole: acme-dev-deployer (create, patch, delete on *)
  │   ├── ClusterRoleBinding: acme-dev-deployer → SA tenant-acme-dev/acme-dev-deployer
  │   ├── ClusterRole: globex-deployer (create, patch, delete on *)
  │   └── ClusterRoleBinding: globex-deployer → SA tenant-globex/globex-deployer
  │
  ├── tenant-acme namespace (root tenant)
  │   ├── ServiceAccount: acme-deployer (grammateus)
  │   ├── Secret: webhook-secret (grammateus)
  │   ├── ConfigMap: gitops-identity (phortizo)
  │   └── Secret: repo-credentials (phortizo)
  │
  ├── tenant-acme-dev namespace (child, ownerRef → tenant-acme)
  │   ├── ServiceAccount: acme-dev-deployer (grammateus)
  │   ├── Secret: webhook-secret (grammateus)
  │   ├── ConfigMap: gitops-identity (phortizo)
  │   └── Secret: repo-credentials (phortizo)
  │
  ├── tenant-globex namespace (root tenant)
  │   ├── ServiceAccount: globex-deployer (grammateus)
  │   ├── Secret: webhook-secret (grammateus)
  │   ├── ConfigMap: gitops-identity (phortizo)
  │   └── Secret: repo-credentials (phortizo)
  │
  ├── acme-prod namespace (provisioned by histia from tenant manifests, impersonating acme-deployer)
  │   └── [acme's workload pods, services, etc.]
  │
  ├── acme-dev-staging namespace (provisioned by histia from tenant manifests, impersonating acme-dev-deployer)
  │   └── [acme-dev's workloads]
  │
  ├── globex-app namespace (provisioned by histia from tenant manifests, impersonating globex-deployer)
  │   └── [globex's workloads]
  │
  └── GATEKEEPER POLICIES (cluster-wide, not per-tenant)
      ├── Policy: SAs can only create namespaces matching their tenant prefix
      ├── Policy: resources in a namespace can only be modified by an SA whose prefix matches
      └── Policy: tenant deployer SAs cannot create ClusterRoles or ClusterRoleBindings
```

## Ecosystem

- **Grammateus** (this) — the tenant API server
- **[Prora](https://github.com/katastroma/prora)** — the tenant self-service
  frontend, consumes this API
