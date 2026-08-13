# e2e test fixtures

## `generated/sts-assertion.crt` / `generated/sts-assertion.key`

A **throwaway** RSA-2048 keypair used only by `TestOAuthSTSAssertion` (D4) for
RFC 7523 private-key-JWT client authentication during the STS token exchange.
These are dev/test credentials — the same trust class as `mcp-server-dev-secret`
in `dev/config/keycloak/realm-import.yaml`. Do **not** reuse them anywhere real.

**The private key is never committed.** The keypair is generated on demand into
`test/e2e/testdata/generated/`, which is gitignored. Generate it with:

```bash
make keycloak-gen-sts-keypair
```

`make keycloak-install` also runs this automatically, then renders the matching
certificate into Keycloak's `mcp-server-jwt` client: the `jwt.credential.certificate`
attribute in `realm-import.yaml` holds the placeholder `@@STS_ASSERTION_CERT_DER@@`,
substituted at install time with the base64 DER of the generated cert. This keeps the
private key (mounted into the server pod) and the public cert (trusted by Keycloak) a
matched pair.

- `sts-assertion.key` — PKCS#8 private key PEM (`-----BEGIN PRIVATE KEY-----`). The
  server signs client assertions with it; the test mounts it into the pod as a Secret.
- `sts-assertion.crt` — self-signed certificate PEM. Keycloak validates the assertion
  signature against it.

`TestOAuthSTSAssertion` reads these files at runtime and **skips** (not fails) if they
are absent, pointing at `make keycloak-gen-sts-keypair`.

### Regenerating

```bash
dev/config/keycloak/gen-sts-assertion-keypair.sh --force
make keycloak-install   # re-renders the realm so Keycloak trusts the new cert
```

Regenerating the key **requires** re-running `make keycloak-install` so the realm
re-renders the matching certificate — otherwise the server's assertions will no longer
verify against the (stale) cert Keycloak holds.
