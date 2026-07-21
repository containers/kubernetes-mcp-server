"""E2e tests for MCP server feature flags.

Each test deploys the server via Helm with a specific TOML config and
verifies the resulting behavior through the MCP protocol.
"""

import pytest


async def test_read_only_mode(deploy_server):
    unrestricted = await deploy_server("unrestricted-baseline", "")
    async with unrestricted.connect_mcp() as session:
        all_tools = {t.name for t in (await session.list_tools()).tools}

    server = await deploy_server("read-only", """
read_only = true
""")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        ro_tools = {t.name for t in result.tools}

    assert len(ro_tools) > 0, "expected at least one tool in read-only mode"
    assert ro_tools < all_tools, (
        f"read-only mode should have strictly fewer tools than unrestricted mode; "
        f"leaked: {ro_tools - all_tools}"
    )
    write_tools = all_tools - ro_tools
    assert len(write_tools) >= 4, (
        f"expected at least 4 tools filtered in read-only mode, "
        f"only {len(write_tools)} filtered: {write_tools}"
    )


DESTRUCTIVE_TOOLS = {"delete_resource"}


async def test_disable_destructive(deploy_server):
    server = await deploy_server("no-destructive", """
disable_destructive = true
""")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        assert len(result.tools) > 0, "expected at least one tool"
        tool_names = {t.name for t in result.tools}
        present_destructive = tool_names & DESTRUCTIVE_TOOLS
        assert not present_destructive, (
            f"destructive tools should be absent: {present_destructive}"
        )


async def test_denied_resources(deploy_server):
    server = await deploy_server("denied-resources", """
[[denied_resources]]
group = ""
version = "v1"
kind = "Secret"
""")
    async with server.connect_mcp() as session:
        call_result = await session.call_tool(
            "resources_list",
            {"apiVersion": "v1", "kind": "Secret", "namespace": "default"},
        )
        assert call_result.isError, (
            f"expected error for denied resource, got success: {call_result.content}"
        )
        combined_text = " ".join(
            block.text for block in call_result.content if hasattr(block, "text")
        )
        assert any(
            keyword in combined_text.lower()
            for keyword in ("denied", "blocked", "not allowed")
        ), (
            f"expected denied/blocked error for secrets, got: {combined_text}"
        )


async def test_enabled_tools(deploy_server):
    server = await deploy_server("enabled-tools", """
enabled_tools = ["resources_get"]
""")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        tool_names = [t.name for t in result.tools]
        assert tool_names == ["resources_get"], (
            f"expected only resources_get, got: {tool_names}"
        )


async def test_disabled_tools(deploy_server):
    server = await deploy_server("disabled-tools", """
disabled_tools = ["delete_resource"]
""")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        tool_names = [t.name for t in result.tools]
        assert "delete_resource" not in tool_names, (
            "delete_resource should not be listed when it is disabled"
        )
        assert len(tool_names) > 0, (
            "expected other tools to still be present"
        )


async def test_stateless_mode(deploy_server):
    server = await deploy_server("stateless", """
stateless = true
""")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        assert len(result.tools) > 0, (
            "expected at least one tool in stateless mode"
        )
        caps = session.mcp_init_result.capabilities
        assert caps.tools is None or caps.tools.listChanged is not True, (
            f"stateless mode should disable tools listChanged, got: {caps.tools}"
        )
        assert caps.prompts is None or caps.prompts.listChanged is not True, (
            f"stateless mode should disable prompts listChanged, got: {caps.prompts}"
        )
        assert caps.resources is None or caps.resources.listChanged is not True, (
            f"stateless mode should disable resources listChanged, got: {caps.resources}"
        )


async def test_server_instructions(deploy_server):
    server = await deploy_server("instructions", """
server_instructions = "You are a test server"
""")
    async with server.connect_mcp() as session:
        assert session.mcp_init_result.instructions == "You are a test server", (
            f"expected server instructions to match, got: "
            f"{getattr(session.mcp_init_result, 'instructions', None)!r}"
        )


async def test_trust_proxy_headers(deploy_server):
    server = await deploy_server("trust-proxy", """
trust_proxy_headers = true
""")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        assert len(result.tools) > 0, (
            "expected at least one tool with trust_proxy_headers enabled"
        )
