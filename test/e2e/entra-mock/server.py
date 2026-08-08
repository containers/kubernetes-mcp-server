"""In-cluster mock Entra ID (Azure AD) STS server for e2e tests.

Provides a lightweight HTTP server that mimics the Azure AD v2.0 OBO
(On-Behalf-Of) token exchange flow.  Deployed as a Kubernetes
Deployment+Service inside the Kind cluster so that MCP server pods can
reach it without host-networking hacks.

Environment variables
---------------------
ENTRA_MOCK_ISSUER_URL   The external URL the mock advertises as its
                        issuer (e.g. the in-cluster Service FQDN).
ENTRA_MOCK_PORT         Listen port (default: 8080).
"""

from __future__ import annotations

import asyncio
import base64
import json
import os
import time
import uuid

from aiohttp import web
import jwt

# ---------------------------------------------------------------------------
# Static RSA keypair (test-only, no security sensitivity)
# ---------------------------------------------------------------------------

PRIVATE_KEY_PEM = b"""-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDLfPD73zFrTmF6
5DNS9jT0mNo+/oo0+2iRJ9yxW9IYVoDO5jNkf81PEaukoEZmBops95tspU3XHM1s
djDjAwYkC/bEl2s9GBVNJ8QRdh1glLXyrs6xXJr7dEtQ+RWH+o8GeaV/2UmEZBD8
tkbxLwoLgxQgm3cTeFjF9wNkTayC3LDgWPxZbANPl6QOHybINctS+4dLL+mZzSpg
MvnQJYY5PQg8KZORYUSvOeO5ICSByLU/T5WMo31RsuwPOwO51hOfsdPBkfp/K7Wm
eOUJYL7ASvpQgm1lQ2xz8iv5yTp8gvj/B8pJHxhw6aTM+TeMuC3o5TWNdW6+pKGP
+0Bj74SHAgMBAAECggEASUmM83HttxOKOTwCHiGNbgC1LdX4Cd/4R7s/GWOUFe7l
wl6XaN08oPsgwhB1el5lsZw2Dpm0oMJ/W85vifs3VXk3nZNZbK4FUf39+Dn9l6DH
rQl3aNqM+P5n99hWAFzl8TOTvymPeE6f7ZxqjYffCslhUOMdLlZ8RoRR5OiytohI
BhysqWclbrZduWyq53O/jXzI8fnA0Z5nFbX2U1U2k1S9utR00OAqSb/940v3GHy5
q3NDc7YfeXqyb5Q0ccmHh8gP0OEGBlBfycaCyw0zpe3WFRE3ZfJouR9dDk6dh04f
zHBK/MdSkugcV5bCJTBIpCXN9IGkaGSSzEbAoQEFLQKBgQDklVWiKE1OqM5kGwSp
uc/75kRjZ8njFlboVI1hTOdXNxT8892Mr0qakCt/3bLG1llIpQ5ekwQK3+G1WNl8
Y36EzZeCflKkDqcen8WopTrPR/R1Yz27MkbWq16ab1a+qHPGCGCBgkadG38SGRIi
pZybXarVuAexRYy2amfuEAqeRQKBgQDj5Q32Rg30vyBb4qgrcJ8ZFOoh9zpfhXiw
0BupzDlla+v1GLFJnpbvxjb+1JGdKAdlAjD1N4vhDgzCI2QdefiOP3iqF0DCbvOp
QwhIayeXucmTysfdvPFJjDUe9T6SY3pRJZUbgWFJmPe5QfBWBvk8f8JWOI2iHpGZ
+mMCgUFaWwKBgBJ5o3c8zKrL6AqdSG4zb4ULooFqVR3+oz2Z/+daYORitlaPm1uQ
m3YMqwdlstpxXrwJYzTvqwb5+3M94C42mHZBa7qHXUSXTpiiD0bHPA6e4TpPsCCe
Oq2FIltXHmrAkMLz0GEHV4/BNi8PSbD1M8g29OTbP/vrBCmGRiour70FAoGBAJbj
kdr9h0AFS+eKqs4YQz7YGi1jA8M7HC31nFtQXLBKRHCDaN7Vohofo0oWdFMZrcuz
J7c0j+jy5H+l7yOVHn0QiVQVEUurKqlnOJS6XfyXhl/UY4DtGNUZgBJ/Tm6ebt5L
g+4yO7f/EAYZIofTFjJ4ZLOxvhUZKE5K+kMuUZcBAoGBAIlktIff/2JkKO4DQK8G
55dtOr2Z1OHsQo0Ej7pX8gj//RodYoRISYFIkddht/Ygxo/ulN63fWNJ+lKusV2z
SNX2YhL0m0K/JX27Kj+8izu2ZOmxfegSsOpiAcsgN3n4g+HIyto/64rh1z1hoSRk
+zr3trSTSwFbdzs3l+xNUYVT
-----END PRIVATE KEY-----
"""

JWK = {
    "kty": "RSA",
    "use": "sig",
    "alg": "RS256",
    "kid": "test-kid",
    "n": "y3zw-98xa05heuQzUvY09JjaPv6KNPtokSfcsVvSGFaAzuYzZH_NTxGrpKBGZgaKbPebbKVN1xzNbHYw4wMGJAv2xJdrPRgVTSfEEXYdYJS18q7OsVya-3RLUPkVh_qPBnmlf9lJhGQQ_LZG8S8KC4MUIJt3E3hYxfcDZE2sgtyw4Fj8WWwDT5ekDh8myDXLUvuHSy_pmc0qYDL50CWGOT0IPCmTkWFErznjuSAkgci1P0-VjKN9UbLsDzsDudYTn7HTwZH6fyu1pnjlCWC-wEr6UIJtZUNsc_Ir-ck6fIL4_wfKSR8YcOmkzPk3jLgt6OU1jXVuvqShj_tAY--Ehw",
    "e": "AQAB",
}

CLIENT_ID = "mock-entra-client"
CLIENT_SECRET = "mock-entra-secret"
VALID_SCOPE = "api://default/.default"

# ---------------------------------------------------------------------------
# Mutable server state
# ---------------------------------------------------------------------------

oauth_authz_server_enabled = True

# ---------------------------------------------------------------------------
# Handlers
# ---------------------------------------------------------------------------


def _discovery_response(issuer_url: str) -> dict:
    return {
        "issuer": issuer_url,
        "token_endpoint": f"{issuer_url}/oauth2/v2.0/token",
        "jwks_uri": f"{issuer_url}/discovery/v2.0/keys",
        "authorization_endpoint": f"{issuer_url}/oauth2/v2.0/authorize",
        "response_types_supported": ["code", "token"],
        "grant_types_supported": [
            "authorization_code",
            "urn:ietf:params:oauth:grant-type:jwt-bearer",
        ],
        "subject_types_supported": ["pairwise"],
        "id_token_signing_alg_values_supported": ["RS256"],
        "token_endpoint_auth_methods_supported": [
            "client_secret_post",
            "client_secret_basic",
            "private_key_jwt",
        ],
    }


def _make_app(issuer_url: str) -> web.Application:
    app = web.Application()

    async def handle_oauth_authorization_server(request: web.Request):
        if not oauth_authz_server_enabled:
            return web.Response(status=404)
        return web.json_response(_discovery_response(issuer_url))

    async def handle_openid_configuration(request: web.Request):
        return web.json_response(_discovery_response(issuer_url))

    async def handle_jwks(request: web.Request):
        return web.json_response({"keys": [JWK]})

    async def handle_token(request: web.Request):
        data = await request.post()

        client_id, client_secret = _extract_client_credentials(request, data)
        if client_id != CLIENT_ID or client_secret != CLIENT_SECRET:
            return web.json_response({"error": "invalid_client"}, status=401)

        grant_type = data.get("grant_type", "")
        if grant_type != "urn:ietf:params:oauth:grant-type:jwt-bearer":
            return web.json_response(
                {"error": "unsupported_grant_type"}, status=400,
            )

        assertion = data.get("assertion", "")
        if not assertion:
            return web.json_response(
                {"error": "invalid_request", "error_description": "missing assertion"},
                status=400,
            )

        requested_token_use = data.get("requested_token_use", "")
        if requested_token_use != "on_behalf_of":
            return web.json_response(
                {"error": "invalid_request", "error_description": "missing requested_token_use"},
                status=400,
            )

        exchanged_token = jwt.encode(
            {
                "sub": "exchanged-user",
                "aud": data.get("scope", VALID_SCOPE),
                "iss": issuer_url,
                "exp": int(time.time()) + 3600,
                "iat": int(time.time()),
                "jti": str(uuid.uuid4()),
            },
            PRIVATE_KEY_PEM,
            algorithm="RS256",
            headers={"kid": "test-kid"},
        )

        return web.json_response({
            "access_token": exchanged_token,
            "token_type": "Bearer",
            "expires_in": 3600,
            "scope": data.get("scope", VALID_SCOPE),
        })

    async def handle_admin_config(request: web.Request):
        global oauth_authz_server_enabled
        data = await request.json()
        if "oauth_authz_server_enabled" in data:
            oauth_authz_server_enabled = bool(data["oauth_authz_server_enabled"])
        return web.json_response({
            "ok": True,
            "oauth_authz_server_enabled": oauth_authz_server_enabled,
        })

    async def handle_admin_config_get(request: web.Request):
        return web.json_response({
            "oauth_authz_server_enabled": oauth_authz_server_enabled,
        })

    app.router.add_get(
        "/.well-known/oauth-authorization-server",
        handle_oauth_authorization_server,
    )
    app.router.add_get(
        "/.well-known/openid-configuration",
        handle_openid_configuration,
    )
    app.router.add_get("/discovery/v2.0/keys", handle_jwks)
    app.router.add_post("/oauth2/v2.0/token", handle_token)
    app.router.add_post("/admin/config", handle_admin_config)
    app.router.add_get("/admin/config", handle_admin_config_get)

    return app


def _extract_client_credentials(request: web.Request, data):
    auth_header = request.headers.get("Authorization", "")
    if auth_header.startswith("Basic "):
        try:
            decoded = base64.b64decode(auth_header[6:]).decode()
            cid, secret = decoded.split(":", 1)
            return cid, secret
        except Exception:
            return None, None
    return data.get("client_id", ""), data.get("client_secret", "")


async def main():
    issuer_url = os.environ["ENTRA_MOCK_ISSUER_URL"]
    port = 8080

    app = _make_app(issuer_url)
    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, "0.0.0.0", port)
    await site.start()
    print(f"Entra mock STS listening on 0.0.0.0:{port}", flush=True)
    print(f"Issuer URL: {issuer_url}", flush=True)
    await asyncio.Event().wait()


if __name__ == "__main__":
    asyncio.run(main())
