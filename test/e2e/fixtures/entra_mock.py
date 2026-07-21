"""Mock Entra ID (Azure AD) STS server for e2e tests.

Provides a lightweight HTTP server that mimics the Azure AD v2.0 OBO
(On-Behalf-Of) token exchange flow.  Runs on the test host and is
reachable from Kind pods via the Docker bridge gateway IP.
"""

from __future__ import annotations

import base64
import socket
import subprocess
import time
import uuid
import warnings

import pytest
import jwt
import pytest_asyncio
from aiohttp import web
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa


def _generate_rsa_keypair():
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    public_key = private_key.public_key()
    public_numbers = public_key.public_numbers()

    def _b64_uint(n: int) -> str:
        length = (n.bit_length() + 7) // 8
        return base64.urlsafe_b64encode(n.to_bytes(length, "big")).rstrip(b"=").decode()

    jwk = {
        "kty": "RSA",
        "use": "sig",
        "alg": "RS256",
        "kid": "test-kid",
        "n": _b64_uint(public_numbers.n),
        "e": _b64_uint(public_numbers.e),
    }
    return private_pem, jwk


def _find_free_port() -> int:
    # TOCTOU: port can be stolen before bind, but acceptable for a
    # session-scoped fixture that runs once.
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("", 0))
        return s.getsockname()[1]


def _kind_gateway_ip() -> str:
    """Discover the Docker/podman bridge gateway IP so Kind pods can reach the host.

    Prefers IPv4 because the mock STS binds to 0.0.0.0 (IPv4 only).
    Uses JSON output (not Go templates) for reliable parsing across versions.
    """
    import json as _json

    # Docker: IPAM.Config[].Gateway
    for engine in ("docker", "podman"):
        try:
            raw = subprocess.check_output(
                [engine, "network", "inspect", "kind"],
                text=True, timeout=10,
            ).strip()
            data = _json.loads(raw)
        except (subprocess.CalledProcessError, FileNotFoundError,
                subprocess.TimeoutExpired, ValueError):
            continue

        gateways = []
        if isinstance(data, list) and data:
            item = data[0]
            # Docker format: .IPAM.Config[].Gateway
            for cfg in (item.get("IPAM", {}).get("Config") or []):
                gw = (cfg.get("Gateway") or "").strip()
                if gw:
                    gateways.append(gw)
            # Podman format: .subnets[].gateway
            for sub in (item.get("subnets") or []):
                gw = (sub.get("gateway") or "").strip()
                if gw:
                    gateways.append(gw)

        # Prefer IPv4
        for gw in gateways:
            if ":" not in gw:
                return gw
        if gateways:
            return gateways[0]

    warnings.warn(
        "Could not detect Kind gateway IP via docker/podman; "
        "falling back to 172.18.0.1 — mock STS may be unreachable from pods",
        stacklevel=2,
    )
    return "172.18.0.1"


class EntraMockSTS:
    """Mock Azure AD v2.0 STS server."""

    CLIENT_ID = "mock-entra-client"
    CLIENT_SECRET = "mock-entra-secret"
    VALID_SCOPE = "api://default/.default"

    def __init__(self):
        self._port = _find_free_port()
        self._gateway_ip = _kind_gateway_ip()
        self._private_pem, self._jwk = _generate_rsa_keypair()
        self._app = web.Application()
        self._runner: web.AppRunner | None = None
        self.oauth_authz_server_enabled = True

        self._app.router.add_get(
            "/.well-known/oauth-authorization-server",
            self._handle_oauth_authorization_server,
        )
        self._app.router.add_get(
            "/.well-known/openid-configuration",
            self._handle_openid_configuration,
        )
        self._app.router.add_get(
            "/discovery/v2.0/keys", self._handle_jwks,
        )
        self._app.router.add_post(
            "/oauth2/v2.0/token", self._handle_token,
        )

    @property
    def issuer_url(self) -> str:
        host = f"[{self._gateway_ip}]" if ":" in self._gateway_ip else self._gateway_ip
        return f"http://{host}:{self._port}"

    @property
    def client_id(self) -> str:
        return self.CLIENT_ID

    @property
    def client_secret(self) -> str:
        return self.CLIENT_SECRET

    def get_user_token(self) -> str:
        """Generate a user token that the mock will accept as a subject token."""
        return jwt.encode(
            {
                "sub": "test-user",
                "aud": self.CLIENT_ID,
                "iss": self.issuer_url,
                "exp": int(time.time()) + 3600,
                "iat": int(time.time()),
                "jti": str(uuid.uuid4()),
            },
            self._private_pem,
            algorithm="RS256",
            headers={"kid": "test-kid"},
        )

    async def start(self):
        self._runner = web.AppRunner(self._app)
        await self._runner.setup()
        try:
            site = web.TCPSite(self._runner, "0.0.0.0", self._port)
            await site.start()
        except BaseException:
            try:
                await self._runner.cleanup()
            except Exception:
                pass
            self._runner = None
            raise

    async def stop(self):
        if self._runner:
            await self._runner.cleanup()

    # -- Handlers --

    def _discovery_response(self):
        return {
            "issuer": self.issuer_url,
            "token_endpoint": f"{self.issuer_url}/oauth2/v2.0/token",
            "jwks_uri": f"{self.issuer_url}/discovery/v2.0/keys",
            "authorization_endpoint": f"{self.issuer_url}/oauth2/v2.0/authorize",
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

    async def _handle_oauth_authorization_server(self, request: web.Request):
        if not self.oauth_authz_server_enabled:
            return web.Response(status=404)
        return web.json_response(self._discovery_response())

    async def _handle_openid_configuration(self, request: web.Request):
        return web.json_response(self._discovery_response())

    async def _handle_jwks(self, request: web.Request):
        return web.json_response({"keys": [self._jwk]})

    def _extract_client_credentials(self, request: web.Request, data):
        """Extract client_id/client_secret from params or Basic auth header."""
        auth_header = request.headers.get("Authorization", "")
        if auth_header.startswith("Basic "):
            try:
                decoded = base64.b64decode(auth_header[6:]).decode()
                cid, secret = decoded.split(":", 1)
                return cid, secret
            except Exception:
                return None, None
        return data.get("client_id", ""), data.get("client_secret", "")

    async def _handle_token(self, request: web.Request):
        data = await request.post()

        client_id, client_secret = self._extract_client_credentials(request, data)
        if client_id != self.CLIENT_ID or client_secret != self.CLIENT_SECRET:
            return web.json_response(
                {"error": "invalid_client"}, status=401,
            )

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
                "aud": data.get("scope", self.VALID_SCOPE),
                "iss": self.issuer_url,
                "exp": int(time.time()) + 3600,
                "iat": int(time.time()),
                "jti": str(uuid.uuid4()),
            },
            self._private_pem,
            algorithm="RS256",
            headers={"kid": "test-kid"},
        )

        return web.json_response({
            "access_token": exchanged_token,
            "token_type": "Bearer",
            "expires_in": 3600,
            "scope": data.get("scope", self.VALID_SCOPE),
        })


def _kind_node_can_reach(url: str, kind_node: str = "kubernetes-mcp-server-control-plane") -> bool:
    """Check if the Kind node container can reach a URL."""
    for engine in ("docker", "podman"):
        try:
            result = subprocess.run(
                [engine, "exec", kind_node, "curl", "-sf",
                 "--connect-timeout", "2", "--max-time", "3", url],
                capture_output=True, timeout=10,
            )
            return result.returncode == 0
        except (FileNotFoundError, subprocess.TimeoutExpired):
            continue
    return False


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def entra_mock():
    """Session-scoped mock Entra ID STS server.

    Skips all dependent tests if the mock is unreachable from Kind pods
    (e.g. rootless podman where the bridge gateway doesn't route to the host).
    """
    server = EntraMockSTS()
    await server.start()
    discovery = f"{server.issuer_url}/.well-known/openid-configuration"
    if not _kind_node_can_reach(discovery):
        await server.stop()
        pytest.skip(
            f"Entra mock STS unreachable from Kind node at {discovery} "
            "(common with rootless podman — bridge gateway doesn't route to host)"
        )
    yield server
    await server.stop()
