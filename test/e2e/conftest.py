"""Shared fixtures for kubernetes-mcp-server e2e tests."""

from __future__ import annotations

import asyncio
import os
import socket
import subprocess
import tempfile
import time
import tomllib
import urllib.error
import urllib.request
from contextlib import asynccontextmanager
from pathlib import Path

import pytest
import pytest_asyncio
import yaml
from kubernetes_asyncio import config as k8s_config
from kubernetes_asyncio.client import (
    ApiClient,
    CoreV1Api,
    V1Namespace,
    V1ObjectMeta,
)
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client

SERVER_PORT = 8080


# ---------------------------------------------------------------------------
# Session-scoped sync fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="session")
def kubeconfig():
    """Path to the kubeconfig for the test cluster."""
    path = os.environ.get("KUBECONFIG", os.path.expanduser("~/.kube/config"))
    if not os.path.isfile(path):
        pytest.skip(f"Kubeconfig not found: {path}")
    return path


@pytest.fixture(scope="session")
def chart_path():
    """Path to the Helm chart directory."""
    path = os.environ.get("CHART_PATH")
    if not path:
        path = str(
            Path(__file__).resolve().parent.parent.parent
            / "charts"
            / "kubernetes-mcp-server"
        )
    if not os.path.isdir(path):
        pytest.skip(f"Helm chart not found: {path}")
    return path


@pytest.fixture(scope="session")
def server_image():
    """Container image for the MCP server."""
    return os.environ.get("MCP_SERVER_IMAGE", "localhost/kubernetes-mcp-server:e2e")


# ---------------------------------------------------------------------------
# Server deployment
# ---------------------------------------------------------------------------


class ServerDeployment:
    """An MCP server deployed to the cluster via Helm."""

    def __init__(self, name: str, namespace: str, server_url: str):
        self.name = name
        self.namespace = namespace
        self.server_url = server_url
        self._port_forward_proc: subprocess.Popen | None = None

    @asynccontextmanager
    async def connect_mcp(self):
        """Connect an MCP client session to this server."""
        async with streamable_http_client(f"{self.server_url}/mcp") as (
            read,
            write,
            _,
        ):
            async with ClientSession(read, write) as session:
                await session.initialize()
                yield session


@pytest_asyncio.fixture
async def deploy_server(kubeconfig, chart_path, server_image):
    """Factory fixture for deploying MCP server instances.

    Usage::

        async def test_something(deploy_server):
            server = await deploy_server("my-test", '''
                read_only = true
                toolsets = ["core", "config"]
            ''')
            async with server.connect_mcp() as session:
                result = await session.list_tools()
    """
    await k8s_config.load_kube_config(config_file=kubeconfig)
    api = ApiClient()
    core_v1 = CoreV1Api(api)

    deployments: list[ServerDeployment] = []

    async def _deploy(name: str, config_toml: str = "") -> ServerDeployment:
        namespace = await _create_namespace(core_v1, name)
        await _helm_install(
            core_v1, namespace, name, chart_path, server_image, config_toml,
        )
        server_url, proc = _start_port_forward(namespace, name)
        await _wait_for_healthz(server_url)

        dep = ServerDeployment(name, namespace, server_url)
        dep._port_forward_proc = proc
        deployments.append(dep)
        return dep

    yield _deploy

    for dep in reversed(deployments):
        subprocess.run(
            ["helm", "uninstall", dep.name, "--namespace", dep.namespace],
            capture_output=True,
        )
        if dep._port_forward_proc:
            dep._port_forward_proc.terminate()
            try:
                dep._port_forward_proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                dep._port_forward_proc.kill()
        try:
            await core_v1.delete_namespace(dep.namespace)
        except Exception:
            pass

    await api.close()


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


async def _create_namespace(core_v1: CoreV1Api, prefix: str) -> str:
    ns = await core_v1.create_namespace(
        body=V1Namespace(
            metadata=V1ObjectMeta(
                generate_name=f"e2e-{prefix}-",
                labels={"app.kubernetes.io/managed-by": "e2e-test"},
            )
        )
    )
    return ns.metadata.name


def _parse_image(image: str) -> tuple[str, str, str]:
    """Split 'registry/repo:tag' into (registry, repository, version)."""
    version = "latest"
    if ":" in image:
        image, version = image.rsplit(":", 1)
    if "/" in image:
        registry, repo = image.split("/", 1)
    else:
        registry, repo = "", image
    return registry, repo, version


def _find_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


async def _helm_install(
    core_v1: CoreV1Api,
    namespace: str,
    name: str,
    chart_path: str,
    image: str,
    config_toml: str,
) -> None:
    config = {}
    if config_toml.strip():
        config = tomllib.loads(config_toml)
    # Remove http section — Helm's toToml converts large integers to scientific
    # notation which the TOML parser rejects.
    # https://github.com/helm/helm/issues/32040
    config.pop("http", None)

    registry, repo, version = _parse_image(image)
    values = {
        "fullnameOverride": name,
        "config": config,
        "image": {
            "registry": registry,
            "repository": repo,
            "version": version,
            "pullPolicy": "IfNotPresent",
        },
        "ingress": {"enabled": False},
    }

    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".yaml", delete=False
    ) as f:
        yaml.dump(values, f)
        values_file = f.name

    try:
        result = subprocess.run(
            [
                "helm", "install", name, chart_path,
                "--namespace", namespace,
                "--values", values_file,
                "--wait",
                "--timeout", "1m",
            ],
            capture_output=True,
            text=True,
        )
        if result.returncode != 0:
            diag = await _dump_pod_diagnostics(core_v1, namespace, name)
            raise RuntimeError(
                f"helm install failed:\n{result.stdout}\n{result.stderr}\n{diag}"
            )
    finally:
        os.unlink(values_file)


def _start_port_forward(
    namespace: str, name: str
) -> tuple[str, subprocess.Popen]:
    local_port = _find_free_port()
    proc = subprocess.Popen(
        [
            "kubectl", "port-forward",
            "-n", namespace,
            f"svc/{name}",
            f"{local_port}:{SERVER_PORT}",
        ],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return f"http://127.0.0.1:{local_port}", proc


async def _wait_for_healthz(url: str, timeout: float = 30.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(f"{url}/healthz", timeout=2):
                return
        except (urllib.error.URLError, OSError):
            await asyncio.sleep(0.5)
    raise TimeoutError(f"Server at {url}/healthz not reachable within {timeout}s")


async def _dump_pod_diagnostics(
    core_v1: CoreV1Api, namespace: str, release_name: str
) -> str:
    label = f"app.kubernetes.io/instance={release_name}"
    sections: list[str] = []

    # Pod status
    pods_items = []
    try:
        pods = await core_v1.list_namespaced_pod(
            namespace=namespace, label_selector=label,
        )
        pods_items = pods.items
        lines = []
        for pod in pods_items:
            phase = pod.status.phase if pod.status else "Unknown"
            node = pod.spec.node_name or "<unscheduled>"
            statuses = ""
            if pod.status and pod.status.container_statuses:
                parts = []
                for cs in pod.status.container_statuses:
                    ready = "ready" if cs.ready else "not-ready"
                    restarts = cs.restart_count
                    parts.append(f"{cs.name}:{ready}(restarts={restarts})")
                statuses = "  " + ", ".join(parts)
            lines.append(f"  {pod.metadata.name}  {phase}  {node}{statuses}")
        sections.append("--- Pods ---\n" + "\n".join(lines))
    except Exception as exc:
        sections.append(f"--- Pods --- (error: {exc})")

    # Pod logs
    for pod in pods_items:
        try:
            logs = await core_v1.read_namespaced_pod_log(
                name=pod.metadata.name,
                namespace=namespace,
                tail_lines=50,
            )
            sections.append(f"--- Logs ({pod.metadata.name}) ---\n{logs}")
        except Exception as exc:
            sections.append(
                f"--- Logs ({pod.metadata.name}) --- (error: {exc})"
            )

    # Events sorted by timestamp
    try:
        event_list = await core_v1.list_namespaced_event(namespace=namespace)
        events = sorted(
            event_list.items,
            key=lambda e: e.last_timestamp or e.event_time or "",
        )
        lines = []
        for event in events:
            ts = event.last_timestamp or event.event_time or ""
            lines.append(f"  {ts}  {event.reason}: {event.message}")
        sections.append("--- Events ---\n" + "\n".join(lines))
    except Exception as exc:
        sections.append(f"--- Events --- (error: {exc})")

    return "\n\n".join(sections)
