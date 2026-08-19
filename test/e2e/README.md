# End-to-end tests

Cluster-backed end-to-end tests for `kubernetes-mcp-server`, built on
[`sigs.k8s.io/e2e-framework`](https://github.com/kubernetes-sigs/e2e-framework).
Each test deploys the server (via its Helm chart) into a fresh namespace on a real
Minikube cluster, drives it through the MCP protocol, and cross-checks the results
against the Kubernetes API.

This document is **living documentation**: it describes what the suite covers today.
Update it when you add or change tests.

## Running

All e2e files carry the `//go:build e2e` tag, so a normal `go test ./...` skips them.
They run against a live cluster through the Makefile:

```bash
# One-time: create cluster, build+load the image, install cert-manager/Keycloak/Kuadrant
make e2e-full-setup

# Run the whole suite
make e2e-test

# Run a subset (forwarded to `go test -run`)
make e2e-test E2E_ARGS='-run TestSmoke'
make e2e-test E2E_ARGS='-run TestOAuth'
```

Lighter setups exist when you don't need every component:

| Target | Provisions |
|--------|-----------|
| `make e2e-setup` | cluster + image (enough for `TestSmoke`) |
| `make e2e-full-setup` | `e2e-setup` + cert-manager + Keycloak + Kuadrant |
| `make e2e-teardown` | delete the cluster |

`e2e-test` sets `KUBECONFIG` to `_output/kubeconfig` and passes `KUBECTL_PATH`,
`HELM_PATH`, and `MCP_SERVER_IMAGE` through to the tests. Overridable env vars:
`MCP_SERVER_IMAGE`, `CHART_PATH`, `HELM_PATH`, `KUBECTL_PATH`, and (for Kuadrant)
`GATEWAY_NAMESPACE`, `GATEWAY_SERVICE`, `GATEWAY_HOST`.

Keycloak fixtures (realm, clients, users, groups) live in
`dev/config/keycloak/realm-import.yaml`; RBAC bindings for OIDC identities live in
`dev/config/keycloak/rbac.yaml`. After editing either, run `make keycloak-install` to
re-import the realm (it restarts Keycloak so the ephemeral H2 database re-imports from
the updated ConfigMap) and re-apply the RBAC. `keycloak-install` also generates the D4
STS-assertion keypair on demand (`make keycloak-gen-sts-keypair`, idempotent) into the
gitignored `test/e2e/testdata/generated/`, and renders the matching cert into the
realm's `mcp-server-jwt` client — the private key is never committed.

Tests **skip** (not fail) when an optional dependency is absent — e.g.
`TestKeycloakOIDC` and every `TestOAuth*` test skip if the `keycloak/keycloak` service
is missing, and `TestKuadrantGatewayTools` skips without the gateway service.

## Layout

| File | Contents |
|------|----------|
| `doc.go`, `e2e_test.go` | package doc, `TestMain`, env/image/chart resolution |
| `setup_test.go` | clientset + `helm` shell helpers |
| `helpers_test.go` | `deployServer` + Helm/port-forward/healthz plumbing, value helpers |
| `helpers_unit_test.go` | pure unit tests (e.g. `TestImageSpec`) — compile-check the package without a cluster |
| `oauth_test.go` | OAuth toolkit: token/discovery/CA helpers, JWT minting, identity discriminator, `requireUnauthorized` |
| `oauth_flows_test.go` | OAuth flow tests: rejection surface, server modes, forwarded identity, STS variants, groups→RBAC |
| `smoke_test.go` | `TestSmoke` — baseline deploy + tool calls |
| `keycloak_test.go` | `TestKeycloakOIDC` — OIDC + STS token-exchange chain |
| `kuadrant_test.go` | `TestKuadrantGatewayTools` — server behind the Kuadrant MCP Gateway |

`TestImageSpec` is a fast way to force a compile of every `e2e`-tagged file without a
cluster: `go test -tags e2e -run TestImageSpec ./test/e2e/`.

## Harness

**`deployServer(ctx, t, cfg, name, opts...) *serverDeployment`** deploys the chart into
a generated namespace, port-forwards to the service, waits for `/healthz`, and registers
cleanup. The generated namespace doubles as the Helm release name and `fullnameOverride`
so cluster-scoped resources (e.g. ClusterRoleBindings) are unique per run. Options:

- `withConfig(toml)` — server config TOML, rendered into the chart's `config` value.
  (An `[http]` section is rejected — Helm's `toToml` mangles large ints, helm/helm#32040.)
- `withValues(map)` — extra Helm values, shallow-merged over the defaults.
- `withPreInstall(fn)` — hook run after namespace creation but **before** Helm install,
  for secrets/config the pod must mount on first start.

Value composition helpers: `mergeValues(...)` (shallow, top-level keys must be disjoint),
`viewClusterRoleBindingValues()` (bind the SA to the built-in `view` role).

## Helpers (`oauth_test.go`)

- `discoverOIDC(t, baseURL, realm)` — fetch `.well-known/openid-configuration`.
- `requestToken(t, tokenRequest)` / `fetchToken(t, tokenRequest)` — token endpoint call;
  `grantType` defaults to `password`; other grants are a field change.
- `fetchExchangedToken(...)` — perform an RFC 8693 exchange in-test to mint an
  `aud=openshift` token (the audience the apiserver requires), used by tests that forward
  a token the server will not itself exchange.
- `mintJWT(...)` — throwaway-RSA-key, RS256-signed JWT for negative tests. A future expiry
  fails only the online signature check; a past expiry fails offline validation on the
  expiry branch first.
- `requireUnauthorized(t, endpoint, token, wantErrorToken)` / `requireUnauthorizedRaw` —
  raw JSON-RPC `initialize` POST asserting `401` + `WWW-Authenticate: …, error="<token>"`.
  Empty `token` sends no `Authorization` header. Bypasses the go-sdk MCP client, which
  hides HTTP status.
- `requireWellKnown` + `requireHTTPStatus` — assert well-known metadata endpoints.
- `callResourcesListSecrets(...)` — identity discriminator: cluster-admin lists Secrets
  (`IsError=false`); a `view`-bound identity cannot (`IsError=true`).
- `oidcServerConfig(scopes)` — shared OIDC + STS server config (scopes overridable).
- `copyKeycloakCASecret` (`withPreInstall` hook) + `keycloakCAVolumeValues()` — copy the
  cert-manager self-signed CA into the namespace and mount it so the server trusts
  Keycloak's TLS.
- `copyKeycloakCAAndAssertionSecrets` (`withPreInstall` hook) + `oidcAssertionVolumeValues()`
  — copy the CA **and** the generated RFC 7523 keypair
  (`testdata/generated/sts-assertion.{crt,key}`, read at runtime) as Secrets, mounting both
  in one `extraVolumes`/`extraVolumeMounts` pair (used by `TestOAuthSTSAssertion`).
  `requireStsAssertionKeypair(t)` skips the test if the keypair has not been generated.

## Coverage

| Test | What it exercises |
|------|-------------------|
| `TestSmoke` | Deploy → `ListTools` → `namespaces_list` / `pods_list_in_namespace`, cross-checked against the K8s API |
| `TestKeycloakOIDC` | OIDC discovery; password-grant token issuance; full chain: user token → server → STS exchange (RFC 8693) → kube-apiserver OIDC → tool call. Includes a `missing_token` negative assertion |
| `TestKuadrantGatewayTools` | Server behind the Kuadrant MCP Gateway: `MCPServerRegistration`, tool-name prefixing, tool call through the gateway |
| `TestOAuthOIDCFlows` | OIDC-mode rejection surface (A1–A8) and unauthenticated well-known metadata (E1–E5, E7, E8), plus the scope-not-enforced guard (F1) |
| `TestOAuthServerModes` | Unprotected anonymous tool call (C1); passthrough rejects a missing token and hides well-known (C4, E6) |
| `TestOAuthForwardedIdentity` | Forwarded-token identity: `require_oauth=false` (C2) and passthrough (C3) act as the cluster-admin user, not the `view` SA (Secrets discriminator) |
| `TestOAuthSTSVariants` | STS variant D3: `rfc8693` strategy + `sts_auth_style=header` (Basic client auth) completes the exchange chain and preserves identity |
| `TestOAuthSTSAssertion` | STS variant D4: `sts_auth_style=assertion` — the server signs an RFC 7523 `client_assertion` (mounted throwaway cert/key) to the `mcp-server-jwt` `client-jwt` Keycloak client, completing the exchange and preserving cluster-admin identity |
| `TestOAuthGroupRBAC` | OIDC groups claim drives K8s RBAC (G1): `mcp-viewer` (group `mcp-viewers` → `view`) is denied Secrets while `mcp` (cluster-admin) is allowed, through the same server |

## Server auth contract (reference)

Grounded in `pkg/http/authorization.go` and `pkg/http/wellknown.go`. Key invariants:

- **All rejections are `401`** — never `403`. Scope is parsed but **not enforced**.
- `WWW-Authenticate: Bearer realm="Kubernetes MCP Server"[, audience="<aud>"], error="<token>"`
  where `<token>` ∈ `{missing_token, invalid_token, temporarily_unavailable}`.
- Decision tree when `require_oauth=true`:
  - missing/non-Bearer header → `missing_token`; empty token after trim → `invalid_token`.
  - `skip_jwt_verification=true` **and** no `authorization_url` → **passthrough** (forward
    to cluster, no local validation).
  - otherwise parse+validate the JWT: parse/offline failure (bad audience, expired) →
    `invalid_token`; provider configured but unreachable → `temporarily_unavailable`;
    provider validation failure (signature/issuer) → `invalid_token`.
  - `require_oauth=true` with neither `authorization_url` nor `skip_jwt_verification` →
    **`500`** (misconfig), no `WWW-Authenticate`.
- `require_oauth=false` → unprotected; a Bearer header, if present, is still forwarded.
- Well-known endpoints (`/.well-known/openid-configuration`,
  `/.well-known/oauth-authorization-server`, `/.well-known/oauth-protected-resource`) and
  infra paths (`/healthz`, metrics) **skip auth entirely**. Well-known returns `404` when
  no `authorization_url` is configured.

## Scenario matrix

Legend — ✅ covered · ⊘ not testable through a healthy e2e-deployed server (see notes).

### A. Rejection surface — `require_oauth=true`, OIDC mode (`requireUnauthorized`)

| # | Scenario | Input | Expect | |
|---|----------|-------|--------|---|
| A1 | No `Authorization` header | — | 401 `missing_token` | ✅ |
| A2 | Non-Bearer scheme | `Basic …` | 401 `missing_token` | ✅ |
| A3 | `Bearer ` empty | `Bearer ` | (arrives as `missing_token`) | ⊘ |
| A4 | Malformed / non-JWT | `Bearer not.a.jwt` | 401 `invalid_token` | ✅ |
| A5 | Wrong audience | token w/ scope `["openid"]` only | 401 `invalid_token`, header `audience="mcp-server"` | ✅ |
| A6 | Expired token | minted JWT, correct audience, past expiry | 401 `invalid_token` (offline *expiry* branch, before signature) | ✅ |
| A7 | Bad signature/issuer | JWT signed by a throwaway key | 401 `invalid_token` (online signature branch) | ✅ |
| A8 | Challenge carries audience | any A-row w/ `oauth_audience` | assert `audience="mcp-server"` | ✅ |

> A3 is ⊘: HTTP trims trailing header whitespace, so `Bearer ` arrives as `Bearer` and
> takes the `missing_token` path (same as A2). The server's empty-token → `invalid_token`
> branch is defensive code unreachable through a compliant HTTP client; it is covered by
> unit tests in `pkg/http/authorization_*_test.go`.

### B. Provider availability / misconfig

| # | Scenario | Config | Expect | |
|---|----------|--------|--------|---|
| B1 | Provider unreachable | `authorization_url` → dead endpoint | 401 `temporarily_unavailable` | ⊘ |
| B2 | Verification unconfigured | `require_oauth=true`, no `authorization_url`, `skip_jwt_verification=false` | **500**, no `WWW-Authenticate` | ⊘ |

> Both are ⊘: they are defensive branches guarded by config validation and fatal startup,
> so a server that passes `/healthz` can never take them.
> - **B2:** `validateSkipJWTVerification` (`pkg/config/config.go`) rejects
>   `require_oauth=true` with no `authorization_url` and `skip_jwt_verification=false`, and
>   `Validate` runs at startup (`cmd/root.go`). The pod fails config validation and never
>   becomes healthy.
> - **B1:** `CreateOIDCProviderAndClient` failure is fatal at startup. A running server with
>   `authorization_url` set always has a non-nil `OIDCProvider`, so the
>   `temporarily_unavailable` branch (provider nil while `authorization_url` set) only occurs
>   in a transient SIGHUP-reload race — not deterministically reproducible in e2e.
>
> Both branches are exercised at the unit level in `pkg/http/authorization_*_test.go`.

### C. Auth modes

| # | Scenario | Config | Expect | |
|---|----------|--------|--------|---|
| C1 | Unprotected | `require_oauth=false`, no token | 200, anonymous → SA-backed tool call | ✅ |
| C2 | Unprotected + forwarded token | `require_oauth=false` + Bearer | 200, header forwarded → acts as user | ✅ |
| C3 | Passthrough | `skip_jwt_verification=true`, no `authorization_url` | token forwarded unvalidated → acts as user | ✅ |
| C4 | Passthrough rejects missing token | as C3, no token | 401 `missing_token` | ✅ |
| C5 | OIDC happy path + STS + tool call | current Keycloak config | 200, full chain | ✅ |

> **Identity is asserted via a Secrets discriminator:** realm user `mcp` maps to
> cluster-admin (`dev/config/keycloak/rbac.yaml`), while the pod SA is bound to `view`,
> which excludes Secrets. `resources_list` of Secrets therefore distinguishes "acts as the
> user" (succeeds) from "acts as the SA" (forbidden). The apiserver requires
> `aud=openshift`, so C2/C3 forward an `openshift`-audience token minted in-test via
> `fetchExchangedToken` (the same exchange the server performs in C5).

### D. STS / token-exchange variants (positive chain)

| # | Scenario | Config | | |
|---|----------|--------|---|---|
| D1 | `rfc8693` + `params` (defaults) | current | implicit in C5 | ✅ |
| D3 | `rfc8693` strategy + `sts_auth_style="header"` | client creds in Basic header | `TestOAuthSTSVariants` | ✅ |
| D4 | `sts_auth_style="assertion"` (private-key-JWT, RFC 7523) | client cert/key + Keycloak `client-jwt` client | `TestOAuthSTSAssertion` | ✅ |
| D2 | `keycloak-v1` strategy | `token_exchange_strategy="keycloak-v1"` | needs legacy exchange feature | deferred |
| D5 | `sts_auth_style="federated"` (SPIRE JWT-SVID) | federated token file | needs real SPIRE | deferred |
| D6 | `entra-obo` strategy | Entra ID only | Azure-specific | ⊘ |

> **D4 uses a dedicated Keycloak client.** `sts_auth_style="assertion"` is private-key-JWT
> client authentication (RFC 7523): the server signs a `client_assertion` JWT with a private
> key instead of sending a client secret. A Keycloak client has exactly one
> `clientAuthenticatorType`, and `mcp-server` uses `client-secret` (relied on by
> C5/D3/`fetchExchangedToken`), so D4 adds a separate confidential client `mcp-server-jwt`
> (`clientAuthenticatorType: client-jwt`, `standard.token.exchange.enabled`) whose
> `jwt.credential.certificate` trusts a throwaway keypair generated on demand into the
> gitignored `testdata/generated/sts-assertion.{crt,key}` (read at runtime, mounted as a
> Secret; the private key is never committed). `make keycloak-install` generates the keypair
> and renders the matching cert DER into the realm placeholder `@@STS_ASSERTION_CERT_DER@@`,
> keeping key and cert a matched pair. No TLS client-cert handshake is involved. Because
> Keycloak's standard token exchange requires the
> exchanging client to be within the subject token's audience, `mcp-server-jwt` is modeled as
> its own resource server: a dedicated `mcp-server-jwt` client scope (audience mapper, offered
> to `mcp-client`) mints the user token with `aud=mcp-server-jwt`, and the test validates
> incoming tokens against `oauth_audience="mcp-server-jwt"`.
>
> **D2 (`keycloak-v1`) is deferred.** The `keycloak-v1` exchanger differs from `rfc8693` only
> by omitting `requested_token_type` and optionally sending `subject_issuer`
> (`keycloak_v1_exchanger.go`) — a same-realm internal exchange exercises a thin wire-format
> delta. It drives Keycloak's *legacy* token exchange, which requires `KC_FEATURES` to enable
> the preview feature on the Keycloak Deployment (`dev/config/keycloak/deployment.yaml`, args
> stay `start-dev --import-realm`) **and** fine-grained authorization permissions expressed in
> the realm JSON (not currently modeled). Enabling fine-grained authz can change authorization
> behavior for the existing standard-exchange clients that C5/D3/D4 depend on, so a dedicated
> client would be needed to contain the blast radius.
>
> **D5 (`federated`) is deferred.** `sts_auth_style="federated"` re-reads a JWT from
> `sts_federated_token_file` on every request and sends it as the `client_assertion`
> (`exchanger.go`). Against Keycloak's `client-jwt` authenticator the file JWT would have to be
> trusted (as in D4) *and* carry a single-use `jti` — but a static file is reused on the second
> exchange, which Keycloak rejects ("Token reuse detected"). Exercising this honestly needs
> real workload-identity infra (SPIRE) that rotates the token per request. The
> `AuthStyleFederated` branch is covered by unit tests
> (`TestInjectClientAuthWithFederated` in `pkg/tokenexchange`).
>
> **D6 (`entra-obo`) is ⊘** — Azure-specific.

### E. Well-known metadata (unauthenticated)

| # | Scenario | Expect | |
|---|----------|--------|---|
| E1 | `openid-configuration` | 200, `issuer` matches Keycloak svc | ✅ |
| E2 | `oauth-protected-resource` | 200, `authorization_servers` set, `bearer_methods_supported=["header"]` | ✅ |
| E3 | `oauth-authorization-server` | 200 | ✅ |
| E4 | `oauth_scopes` override | metadata `scopes_supported` == configured | ✅ |
| E5 | `disable_dynamic_client_registration` | `registration_endpoint` absent, `require_request_uri_registration=false` | ✅ |
| E6 | No `authorization_url` | 404 | ✅ |
| E7 | Reachable w/o token when `require_oauth=true` | 200 (proves middleware skip) | ✅ |
| E8 | CORS | `Access-Control-Allow-Origin: *` | ✅ |

### F. Non-enforcement guard

| # | Scenario | Expect | |
|---|----------|--------|---|
| F1 | Valid token, server advertises a scope the token lacks | **200** (scope not enforced) — regression guard | ✅ |

### G. Groups claim → Kubernetes RBAC

| # | Scenario | Expect | |
|---|----------|--------|---|
| G1 | User in group `mcp-viewers` (bound to `view`) vs `mcp` (cluster-admin), same server | group user: `namespaces_list` succeeds but Secrets forbidden; `mcp`: Secrets allowed | ✅ |

> G1 proves the OIDC `groups` claim drives RBAC. Realm fixtures define group `mcp-viewers`
> and user `mcp-viewer` (a member); `rbac.yaml` binds Group `mcp-viewers` → `view`. The
> exchanged (`aud=openshift`) token carries the `groups` claim because the `mcp-server` client
> (which performs the exchange) has `groups` as a default scope. The apiserver reads
> `--oidc-groups-claim=groups` with no group prefix, so the K8s group is the bare claim value
> `mcp-viewers`.

## Conventions

- Every file needs the `//go:build e2e` build tag.
- Prefer table-driven `Assess` steps sharing a single `deployServer` per config over one
  feature-per-row (one Helm install, many assertions).
- Skip (don't fail) when an optional dependency is absent.
- Assert against ground truth (the K8s API), not just tool self-consistency.
- Realm/client/user fixtures live in `dev/config/keycloak/realm-import.yaml`; add to it only
  when a specific flow needs it, and note the dependency in the test.
