"""Provider-strategy-agnostic multi-cluster e2e tests.

These tests assume that the environment provides N>1 clusters.  The
cluster provider strategy is configurable via environment variables so
that the same test file runs upstream (kubeconfig) and downstream (ACM
or any other registered provider).

Environment variables
---------------------
CLUSTER_PROVIDER_STRATEGY   Provider strategy (default: ``kubeconfig``).
MULTICLUSTER_KUBECONFIG     Path to a kubeconfig with N>1 contexts
                            (required for the kubeconfig strategy).
TARGET_PARAMETER_NAME       Override the target parameter name injected
                            into tool schemas (default: derived from
                            the strategy, e.g. ``context``).
TARGET_LIST_TOOL            Override the name of the tool that lists
                            available targets (default: derived from
                            the strategy).
AVAILABLE_TARGETS           Comma-separated list of target names for
                            non-kubeconfig strategies.
"""

from __future__ import annotations

import os

import pytest
import yaml

from conftest import (
    ca_namespace_setup,
    create_ca_cert_secret,
    create_file_secret,
    keycloak_oauth_toml as _oauth_toml,
    merge_extra_values,
)

pytestmark = pytest.mark.multicluster

STRATEGY_TARGET_PARAM = {
    "kubeconfig": "context",
    "kcp": "workspace",
}

STRATEGY_LIST_TOOL = {
    "kubeconfig": "configuration_contexts_list",
}

KUBECONFIG_MOUNT_PATH = "/etc/kubeconfig/config"


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="session")
def cluster_provider_strategy():
    return os.environ.get("CLUSTER_PROVIDER_STRATEGY", "kubeconfig")


@pytest.fixture(scope="session")
def target_parameter_name(cluster_provider_strategy):
    return os.environ.get(
        "TARGET_PARAMETER_NAME",
        STRATEGY_TARGET_PARAM.get(cluster_provider_strategy, "cluster"),
    )


@pytest.fixture(scope="session")
def target_list_tool(cluster_provider_strategy, target_parameter_name):
    return os.environ.get(
        "TARGET_LIST_TOOL",
        STRATEGY_LIST_TOOL.get(
            cluster_provider_strategy,
            f"{target_parameter_name}_list",
        ),
    )


@pytest.fixture(scope="session")
def multicluster_kubeconfig_path(cluster_provider_strategy):
    """Path to a kubeconfig with multiple contexts (provided by CI)."""
    path = os.environ.get("MULTICLUSTER_KUBECONFIG")
    if cluster_provider_strategy == "kubeconfig":
        if not path or not os.path.isfile(path):
            pytest.skip(
                "MULTICLUSTER_KUBECONFIG not set or not found "
                f"(strategy={cluster_provider_strategy})"
            )
    return path


@pytest.fixture(scope="session")
def available_targets(multicluster_kubeconfig_path, cluster_provider_strategy):
    """Discover available cluster targets from the environment."""
    if cluster_provider_strategy == "kubeconfig" and multicluster_kubeconfig_path:
        with open(multicluster_kubeconfig_path) as f:
            kc = yaml.safe_load(f)
        if not kc:
            pytest.skip(
                f"multicluster kubeconfig is empty or invalid: "
                f"{multicluster_kubeconfig_path}"
            )
        targets = [ctx["name"] for ctx in kc.get("contexts", [])]
        if len(targets) < 2:
            pytest.skip(
                f"need N>1 kubeconfig contexts, found {len(targets)}"
            )
        return targets

    raw = os.environ.get("AVAILABLE_TARGETS", "")
    if not raw:
        pytest.skip("AVAILABLE_TARGETS not set for non-kubeconfig strategy")
    targets = [t.strip() for t in raw.split(",") if t.strip()]
    if len(targets) < 2:
        pytest.skip(f"need N>1 targets, found {len(targets)}")
    return targets


# ---------------------------------------------------------------------------
# Deploy helpers
# ---------------------------------------------------------------------------


def _multicluster_toml(strategy, extra_lines=None):
    """Build TOML config for multi-cluster deployment."""
    lines = [f'cluster_provider_strategy = "{strategy}"']
    if strategy == "kubeconfig":
        lines.append(f'kubeconfig = "{KUBECONFIG_MOUNT_PATH}"')
    if extra_lines:
        lines.extend(extra_lines)
    return "\n".join(lines)


def _kubeconfig_extra_values():
    """Helm extra values to mount the multi-cluster kubeconfig Secret."""
    return {
        "extraVolumes": [
            {"name": "kubeconfig", "secret": {"secretName": "multicluster-kubeconfig"}},
        ],
        "extraVolumeMounts": [
            {"name": "kubeconfig", "mountPath": "/etc/kubeconfig", "readOnly": True},
        ],
    }




async def _create_kubeconfig_secret(core_v1, namespace, kubeconfig_path):
    """Create a K8s Secret containing the multi-cluster kubeconfig."""
    await create_file_secret(
        core_v1, namespace, kubeconfig_path,
        secret_name="multicluster-kubeconfig", data_key="config",
    )


def _kubeconfig_namespace_setup(kubeconfig_path):
    """Return an async callable that creates the kubeconfig Secret."""
    async def setup(core_v1, namespace):
        await _create_kubeconfig_secret(core_v1, namespace, kubeconfig_path)
    return setup


def _kubeconfig_and_ca_namespace_setup(kubeconfig_path, ca_cert_path):
    """Return an async callable that creates both the kubeconfig and CA Secrets."""
    async def setup(core_v1, namespace):
        await _create_kubeconfig_secret(core_v1, namespace, kubeconfig_path)
        await create_ca_cert_secret(core_v1, namespace, ca_cert_path)
    return setup


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def _deploy_multicluster(deploy_server, strategy, kubeconfig_path, name):
    """Deploy a multicluster server, deriving extra_values/ns_setup from strategy."""
    extra_values = None
    ns_setup = None
    if strategy == "kubeconfig":
        extra_values = _kubeconfig_extra_values()
        ns_setup = _kubeconfig_namespace_setup(kubeconfig_path)

    config = _multicluster_toml(strategy)
    return await deploy_server(
        name,
        config,
        extra_values=extra_values,
        namespace_setup=ns_setup,
    )


async def test_list_targets(
    deploy_server,
    cluster_provider_strategy,
    multicluster_kubeconfig_path,
    available_targets,
    target_list_tool,
):
    """Server discovers and lists N>1 targets."""
    server = await _deploy_multicluster(
        deploy_server, cluster_provider_strategy,
        multicluster_kubeconfig_path, "mc-list-targets",
    )
    async with server.connect_mcp() as session:
        result = await session.call_tool(target_list_tool, {})
        assert not result.isError, f"target list tool failed: {result.content}"
        text = result.content[0].text
        for target in available_targets:
            assert target in text, (
                f"expected target {target!r} in tool output: {text}"
            )


async def test_cross_cluster_get_resource(
    deploy_server,
    cluster_provider_strategy,
    multicluster_kubeconfig_path,
    available_targets,
    target_parameter_name,
):
    """resources_list with explicit target succeeds on each cluster."""
    server = await _deploy_multicluster(
        deploy_server, cluster_provider_strategy,
        multicluster_kubeconfig_path, "mc-cross-cluster",
    )
    async with server.connect_mcp() as session:
        for target in available_targets:
            result = await session.call_tool(
                "resources_list",
                {"apiVersion": "v1", "kind": "Namespace", target_parameter_name: target},
            )
            assert not result.isError, (
                f"resources_list failed on target {target!r}: {result.content}"
            )
            assert result.content[0].text, (
                f"empty response from target {target!r}"
            )


async def test_default_target_access(
    deploy_server,
    cluster_provider_strategy,
    multicluster_kubeconfig_path,
    available_targets,
):
    """resources_list without explicit target uses the default cluster."""
    server = await _deploy_multicluster(
        deploy_server, cluster_provider_strategy,
        multicluster_kubeconfig_path, "mc-default-target",
    )
    async with server.connect_mcp() as session:
        result = await session.call_tool(
            "resources_list", {"apiVersion": "v1", "kind": "Namespace"},
        )
        assert not result.isError, (
            f"resources_list without target failed: {result.content}"
        )


async def test_invalid_target_rejected(
    deploy_server,
    cluster_provider_strategy,
    multicluster_kubeconfig_path,
    available_targets,
    target_parameter_name,
):
    """Non-existent target returns an error."""
    server = await _deploy_multicluster(
        deploy_server, cluster_provider_strategy,
        multicluster_kubeconfig_path, "mc-invalid-target",
    )
    async with server.connect_mcp() as session:
        result = await session.call_tool(
            "resources_list",
            {"apiVersion": "v1", "kind": "Namespace", target_parameter_name: "nonexistent-cluster-xyz"},
        )
        assert result.isError, (
            "expected error for non-existent target"
        )


@pytest.mark.keycloak
async def test_oauth_cross_cluster(
    deploy_server,
    keycloak,
    keycloak_ca_cert_path,
    keycloak_extra_values,
    cluster_provider_strategy,
    multicluster_kubeconfig_path,
    available_targets,
    target_parameter_name,
):
    """OAuth + token exchange: authenticated resources_list succeeds on each cluster."""
    extra_values = keycloak_extra_values
    ns_setup_fn = ca_namespace_setup(keycloak_ca_cert_path)

    if cluster_provider_strategy == "kubeconfig":
        extra_values = merge_extra_values(
            _kubeconfig_extra_values(), keycloak_extra_values,
        )
        ns_setup_fn = _kubeconfig_and_ca_namespace_setup(
            multicluster_kubeconfig_path, keycloak_ca_cert_path,
        )

    client_secret = await keycloak.get_mcp_server_client_secret()
    oauth_config = _oauth_toml(
        keycloak.issuer_url,
        token_exchange_strategy="rfc8693",
        sts_client_id="mcp-server",
        sts_client_secret=client_secret,
        sts_audience="openshift",
        sts_scopes=["mcp:openshift"],
    )
    config = _multicluster_toml(
        cluster_provider_strategy, extra_lines=oauth_config.splitlines(),
    )

    server = await deploy_server(
        "mc-oauth",
        config,
        extra_values=extra_values,
        namespace_setup=ns_setup_fn,
    )
    token = await keycloak.get_user_token_for_audience("mcp-server")
    async with server.connect_mcp_with_auth(token) as session:
        for target in available_targets:
            result = await session.call_tool(
                "resources_list",
                {"apiVersion": "v1", "kind": "Namespace", target_parameter_name: target},
            )
            assert not result.isError, (
                f"OAuth resources_list failed on target {target!r}: {result.content}"
            )
