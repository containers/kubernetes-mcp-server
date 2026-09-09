# Configuration Changes

This document records configuration changes that may require action when upgrading between releases. Add newer release transitions as separate sections so upgrade guidance remains available without cluttering the current [configuration reference](configuration.md).

## Upgrading from 0.0.66

The legacy top-level `token_exchange_strategy` and `sts_*` settings were removed and are rejected at startup. Migrate them as follows:

| Legacy setting | Replacement |
|----------------|-------------|
| `token_exchange_strategy` | `token_exchange.strategy` |
| `sts_audience` | `token_exchange.audience` |
| `sts_scopes` | `token_exchange.scopes` |
| `sts_subject_token_type` | `token_exchange.subject_token_type` |
| `sts_requested_token_type` | `token_exchange.requested_token_type` |
| `sts_client_id` | `token_exchange.client_auth.client_id` |
| `sts_client_secret` | `token_exchange.client_auth.client_secret` |
| `sts_auth_style` | `token_exchange.client_auth.method` |
| `sts_client_cert_file` | `token_exchange.client_auth.certificate_file` |
| `sts_client_key_file` | `token_exchange.client_auth.private_key_file` |
| `sts_federated_token_file` | `token_exchange.client_auth.token_file` |

Map legacy `sts_auth_style` values as follows: `params` to `client_secret_post`, `header` to `client_secret_basic`, `assertion` to `private_key_jwt`, and `federated` to `jwt_file`. Configurations that omitted `token_exchange_strategy` used the built-in STS path and should use `client_secret_basic` to preserve HTTP Basic client authentication.
