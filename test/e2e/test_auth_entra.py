"""E2e tests for Entra ID (Azure AD) OBO token exchange.

Uses a mock Entra ID STS server by default.  Set ``AZURE_TENANT_ID``,
``AZURE_CLIENT_ID``, and ``AZURE_CLIENT_SECRET`` environment variables
to run against a real Azure AD tenant instead.
"""

from __future__ import annotations

import os

import pytest

from conftest import _toml_escape, oauth_toml

pytestmark = pytest.mark.entra


# ---------------------------------------------------------------------------
# Config helpers
# ---------------------------------------------------------------------------

def _entra_oauth_toml(
    issuer_url: str,
    client_id: str,
    client_secret: str,
    audience: str = "",
    **extra,
) -> str:
    base_lines = [
        'require_oauth = true',
        f'authorization_url = "{_toml_escape(issuer_url)}"',
        f'oauth_audience = "{_toml_escape(audience)}"',
        'skip_jwt_verification = true',
        'token_exchange_strategy = "entra-obo"',
        f'sts_client_id = "{_toml_escape(client_id)}"',
        f'sts_client_secret = "{_toml_escape(client_secret)}"',
    ]
    return oauth_toml(base_lines, **extra)


def _is_real_azure() -> bool:
    return bool(os.environ.get("AZURE_TENANT_ID"))


# ---------------------------------------------------------------------------
# Tests using mock STS
# ---------------------------------------------------------------------------


async def test_entra_obo_exchange(deploy_server, entra_mock):
    """OBO exchange flow completes and server accepts the exchanged token."""
    token = entra_mock.get_user_token()
    server = await deploy_server(
        "entra-obo",
        _entra_oauth_toml(
            entra_mock.issuer_url,
            entra_mock.client_id,
            entra_mock.client_secret,
            sts_audience=entra_mock.VALID_SCOPE,
        ),
    )
    async with server.connect_mcp_with_auth(token) as session:
        result = await session.list_tools()
        assert len(result.tools) > 0, "expected tools after OBO exchange"


@pytest.mark.parametrize("sts_auth_style", ["params", "header"])
async def test_entra_obo_auth_style(deploy_server, entra_mock, sts_auth_style):
    token = entra_mock.get_user_token()
    server = await deploy_server(
        f"entra-{sts_auth_style}",
        _entra_oauth_toml(
            entra_mock.issuer_url,
            entra_mock.client_id,
            entra_mock.client_secret,
            sts_auth_style=sts_auth_style,
            sts_audience=entra_mock.VALID_SCOPE,
        ),
    )
    async with server.connect_mcp_with_auth(token) as session:
        result = await session.list_tools()
        assert len(result.tools) > 0


async def test_entra_oidc_fallback(deploy_server, entra_mock):
    """Server falls back to openid-configuration when oauth-authorization-server returns 404."""
    entra_mock.oauth_authz_server_enabled = False
    try:
        token = entra_mock.get_user_token()
        server = await deploy_server(
            "entra-fallback",
            _entra_oauth_toml(
                entra_mock.issuer_url,
                entra_mock.client_id,
                entra_mock.client_secret,
                sts_audience=entra_mock.VALID_SCOPE,
            ),
        )
        async with server.connect_mcp_with_auth(token) as session:
            result = await session.list_tools()
            assert len(result.tools) > 0
    finally:
        entra_mock.oauth_authz_server_enabled = True


async def test_entra_oauth_without_obo(deploy_server, entra_mock):
    """OAuth with Entra tokens for MCP auth, kubeconfig for K8s access (no OBO)."""
    token = entra_mock.get_user_token()
    config = "\n".join([
        'require_oauth = true',
        f'authorization_url = "{_toml_escape(entra_mock.issuer_url)}"',
        'skip_jwt_verification = true',
        'oauth_audience = ""',
    ])
    server = await deploy_server("entra-no-obo", config)
    async with server.connect_mcp_with_auth(token) as session:
        result = await session.list_tools()
        assert len(result.tools) > 0


# ---------------------------------------------------------------------------
# Tests that only run against real Azure AD
# ---------------------------------------------------------------------------


@pytest.mark.skipif(not _is_real_azure(), reason="requires real Azure AD credentials")
async def test_entra_invalid_client(deploy_server, entra_mock):
    """Invalid sts_client_id is rejected by Azure AD (AADSTS700038)."""
    issuer = f"https://login.microsoftonline.com/{os.environ['AZURE_TENANT_ID']}/v2.0"
    client_id = "00000000-0000-0000-0000-000000000000"
    client_secret = os.environ["AZURE_CLIENT_SECRET"]

    token = entra_mock.get_user_token()
    server = await deploy_server(
        "entra-bad-client",
        _entra_oauth_toml(issuer, client_id, client_secret),
    )
    status = await server.raw_mcp_request(token)
    assert status in (401, 403), (
        f"expected auth failure for invalid client, got HTTP {status}"
    )


@pytest.mark.skipif(not _is_real_azure(), reason="requires real Azure AD credentials")
async def test_entra_unauthorized_scope(deploy_server, entra_mock):
    """Unauthorized scope is rejected by Azure AD (consent_required)."""
    issuer = f"https://login.microsoftonline.com/{os.environ['AZURE_TENANT_ID']}/v2.0"
    client_id = os.environ["AZURE_CLIENT_ID"]
    client_secret = os.environ["AZURE_CLIENT_SECRET"]
    scope = "api://unauthorized-scope/.default"

    token = entra_mock.get_user_token()
    server = await deploy_server(
        "entra-bad-scope",
        _entra_oauth_toml(
            issuer, client_id, client_secret,
            sts_scopes=[scope],
        ),
    )
    status = await server.raw_mcp_request(token)
    assert status in (400, 401, 403), (
        f"expected auth failure for unauthorized scope, got HTTP {status}"
    )
