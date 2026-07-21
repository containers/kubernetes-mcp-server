"""End-to-end tests for Keycloak OAuth and token-exchange scenarios."""

import jwt
import pytest

from conftest import (
    ca_namespace_setup as _ca_namespace_setup,
    keycloak_oauth_toml as _oauth_toml,
)


def _make_unsigned_jwt() -> str:
    """Craft a minimal unsigned JWT for skip_jwt_verification tests."""
    return jwt.encode(
        {"sub": "test", "aud": "test", "exp": 9999999999},
        "",
        algorithm="none",
    )


# -- Tests ----------------------------------------------------------------


async def test_no_auth_connect(deploy_server):
    """Server without require_oauth allows unauthenticated MCP connections."""
    server = await deploy_server("no-auth")
    async with server.connect_mcp() as session:
        tools = await session.list_tools()
        assert len(tools.tools) > 0


async def test_no_auth_ignores_bearer(deploy_server):
    """Server without require_oauth ignores unsolicited Bearer tokens."""
    server = await deploy_server("no-auth-bearer")
    async with server.connect_mcp_with_auth("random-invalid-token") as session:
        tools = await session.list_tools()
        assert len(tools.tools) > 0


@pytest.mark.keycloak
async def test_oauth_connect(deploy_server, keycloak, keycloak_ca_cert_path, keycloak_extra_values):
    """OAuth-enabled server rejects unauthenticated requests and accepts valid tokens."""
    server = await deploy_server(
        "oauth-connect",
        _oauth_toml(keycloak.issuer_url),
        extra_values=keycloak_extra_values,
        namespace_setup=_ca_namespace_setup(keycloak_ca_cert_path),
    )
    # Unauthenticated request must be rejected
    status = await server.raw_mcp_request()
    assert status == 401

    # Authenticated request with valid token must succeed
    token = await keycloak.get_user_token_for_audience("mcp-server")
    async with server.connect_mcp_with_auth(token) as session:
        tools = await session.list_tools()
        assert len(tools.tools) > 0


@pytest.mark.keycloak
async def test_oauth_rejects_wrong_audience(
    deploy_server, keycloak, keycloak_ca_cert_path, keycloak_extra_values
):
    """OAuth-enabled server rejects tokens that do not carry the required audience."""
    server = await deploy_server(
        "oauth-wrong-aud",
        _oauth_toml(keycloak.issuer_url, audience="mcp-server"),
        extra_values=keycloak_extra_values,
        namespace_setup=_ca_namespace_setup(keycloak_ca_cert_path),
    )
    # Token from mcp-client default scopes does not include mcp-server audience
    token = await keycloak.get_user_token()
    status = await server.raw_mcp_request(token=token)
    assert status in (401, 403)


@pytest.mark.keycloak
async def test_empty_audience_disables_check(
    deploy_server, keycloak, keycloak_ca_cert_path, keycloak_extra_values
):
    """Setting oauth_audience to empty string disables audience validation."""
    server = await deploy_server(
        "oauth-empty-aud",
        _oauth_toml(keycloak.issuer_url, audience=""),
        extra_values=keycloak_extra_values,
        namespace_setup=_ca_namespace_setup(keycloak_ca_cert_path),
    )
    token = await keycloak.get_user_token()
    async with server.connect_mcp_with_auth(token) as session:
        tools = await session.list_tools()
        assert len(tools.tools) > 0


async def test_skip_jwt_verification(deploy_server):
    """With skip_jwt_verification the server accepts tokens without signature checks."""
    config_toml = "\n".join([
        'require_oauth = true',
        'skip_jwt_verification = true',
    ])
    server = await deploy_server("skip-jwt", config_toml)
    fake_jwt = _make_unsigned_jwt()

    async with server.connect_mcp_with_auth(fake_jwt) as session:
        tools = await session.list_tools()
        assert len(tools.tools) > 0


@pytest.mark.keycloak
@pytest.mark.parametrize("strategy", ["rfc8693", "keycloak-v1"])
async def test_token_exchange(
    deploy_server, keycloak, keycloak_ca_cert_path, keycloak_extra_values, strategy
):
    """Token exchange lets the server call the K8s API on behalf of the user."""
    client_secret = await keycloak.get_mcp_server_client_secret()
    config_toml = _oauth_toml(
        keycloak.issuer_url,
        token_exchange_strategy=strategy,
        sts_client_id="mcp-server",
        sts_client_secret=client_secret,
        sts_audience="openshift",
        sts_scopes=["mcp:openshift"],
    )
    server = await deploy_server(
        f"te-{strategy}",
        config_toml,
        extra_values=keycloak_extra_values,
        namespace_setup=_ca_namespace_setup(keycloak_ca_cert_path),
    )
    token = await keycloak.get_user_token_for_audience("mcp-server")
    async with server.connect_mcp_with_auth(token) as session:
        result = await session.call_tool(
            "resources_list", {"apiVersion": "v1", "kind": "Namespace"},
        )
        assert not result.isError
        assert len(result.content) > 0
        assert result.content[0].text


@pytest.mark.keycloak
@pytest.mark.parametrize("sts_auth_style", ["params", "header"])
async def test_sts_auth_style(
    deploy_server, keycloak, keycloak_ca_cert_path, keycloak_extra_values, sts_auth_style
):
    """Token exchange works with each sts_auth_style value."""
    client_secret = await keycloak.get_mcp_server_client_secret()
    config_toml = _oauth_toml(
        keycloak.issuer_url,
        token_exchange_strategy="rfc8693",
        sts_client_id="mcp-server",
        sts_client_secret=client_secret,
        sts_audience="openshift",
        sts_scopes=["mcp:openshift"],
        sts_auth_style=sts_auth_style,
    )
    server = await deploy_server(
        f"sts-{sts_auth_style}",
        config_toml,
        extra_values=keycloak_extra_values,
        namespace_setup=_ca_namespace_setup(keycloak_ca_cert_path),
    )
    token = await keycloak.get_user_token_for_audience("mcp-server")
    async with server.connect_mcp_with_auth(token) as session:
        result = await session.call_tool(
            "resources_list", {"apiVersion": "v1", "kind": "Namespace"},
        )
        assert not result.isError
        assert len(result.content) > 0
        assert result.content[0].text
