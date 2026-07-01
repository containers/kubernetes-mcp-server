"""End-to-end tests for the kubernetes-mcp-server."""


async def test_list_tools(deploy_server):
    """Server should expose at least one tool."""
    server = await deploy_server("list-tools")
    async with server.connect_mcp() as session:
        result = await session.list_tools()
        assert len(result.tools) > 0, "expected at least one tool from the server"
