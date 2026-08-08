"""Keycloak token operations for e2e tests."""

from __future__ import annotations

import asyncio
import ssl

import aiohttp
import pytest
import pytest_asyncio


KEYCLOAK_URL = "https://keycloak.127-0-0-1.sslip.io:8443"
REALM = "openshift"
MCP_CLIENT_ID = "mcp-client"
MCP_SERVER_CLIENT_ID = "mcp-server"
TEST_USER = "mcp"
TEST_PASSWORD = "mcp"
ADMIN_USER = "admin"
ADMIN_PASSWORD = "admin"


class KeycloakClient:
    """Helper for obtaining tokens from a Keycloak instance."""

    def __init__(self, ca_cert_path: str):
        self._ca_cert_path = ca_cert_path
        self._ssl_context = ssl.create_default_context(cafile=ca_cert_path)
        self._client_secret: str | None = None
        self._session: aiohttp.ClientSession | None = None

    async def _get_session(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession(
                timeout=aiohttp.ClientTimeout(total=30),
            )
        return self._session

    async def close(self):
        if self._session and not self._session.closed:
            await self._session.close()

    @property
    def issuer_url(self) -> str:
        return f"{KEYCLOAK_URL}/realms/{REALM}"

    @property
    def token_endpoint(self) -> str:
        return f"{KEYCLOAK_URL}/realms/{REALM}/protocol/openid-connect/token"

    async def get_user_token(
        self,
        client_id: str = MCP_CLIENT_ID,
        scope: str | None = None,
    ) -> str:
        """Obtain a token via resource owner password grant."""
        data = {
            "grant_type": "password",
            "client_id": client_id,
            "username": TEST_USER,
            "password": TEST_PASSWORD,
        }
        if scope:
            data["scope"] = scope

        session = await self._get_session()
        async with session.post(
            self.token_endpoint, data=data, ssl=self._ssl_context,
        ) as resp:
            resp.raise_for_status()
            body = await resp.json()
            return body["access_token"]

    async def get_user_token_for_audience(self, audience: str) -> str:
        """Obtain a token that includes a specific audience claim.

        The Keycloak setup maps the 'mcp-server' scope to a 'mcp-server'
        audience via a protocol mapper.  Requesting that scope with the
        mcp-client public client yields a token whose ``aud`` includes
        'mcp-server'.
        """
        return await self.get_user_token(
            client_id=MCP_CLIENT_ID, scope=f"openid {audience}",
        )

    async def get_mcp_server_client_secret(self) -> str:
        """Retrieve the mcp-server client secret from the Keycloak admin API."""
        if self._client_secret is not None:
            return self._client_secret

        admin_token = await self._get_admin_token()
        url = (
            f"{KEYCLOAK_URL}/admin/realms/{REALM}/clients"
            f"?clientId={MCP_SERVER_CLIENT_ID}"
        )
        session = await self._get_session()
        async with session.get(
            url,
            headers={"Authorization": f"Bearer {admin_token}"},
            ssl=self._ssl_context,
        ) as resp:
            resp.raise_for_status()
            clients = await resp.json()

        if not clients:
            raise RuntimeError(
                f"Client '{MCP_SERVER_CLIENT_ID}' not found in Keycloak"
            )
        internal_id = clients[0]["id"]

        secret_url = (
            f"{KEYCLOAK_URL}/admin/realms/{REALM}"
            f"/clients/{internal_id}/client-secret"
        )
        async with session.get(
            secret_url,
            headers={"Authorization": f"Bearer {admin_token}"},
            ssl=self._ssl_context,
        ) as resp:
            resp.raise_for_status()
            body = await resp.json()
            self._client_secret = body["value"]
            return self._client_secret

    async def _get_admin_token(self) -> str:
        data = {
            "grant_type": "password",
            "client_id": "admin-cli",
            "username": ADMIN_USER,
            "password": ADMIN_PASSWORD,
        }
        admin_url = (
            f"{KEYCLOAK_URL}/realms/master/protocol/openid-connect/token"
        )
        session = await self._get_session()
        async with session.post(
            admin_url, data=data, ssl=self._ssl_context,
        ) as resp:
            resp.raise_for_status()
            body = await resp.json()
            return body["access_token"]


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def keycloak(keycloak_ca_cert_path):
    """Session-scoped Keycloak client.

    Skips all dependent tests if Keycloak is not reachable.
    """
    client = KeycloakClient(keycloak_ca_cert_path)
    try:
        await client.get_user_token()
    except (aiohttp.ClientError, ssl.SSLError, OSError, asyncio.TimeoutError) as exc:
        await client.close()
        pytest.skip(
            f"Keycloak not reachable at {KEYCLOAK_URL}: {exc}"
        )
    yield client
    await client.close()
