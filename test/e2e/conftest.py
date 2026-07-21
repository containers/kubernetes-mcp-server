"""Shared fixtures for kubernetes-mcp-server e2e tests."""

from __future__ import annotations

import asyncio
import base64
import os
import re
import subprocess
import tempfile
import time
import tomllib
import urllib.error
import urllib.request
import warnings
from contextlib import asynccontextmanager
from pathlib import Path

import httpx
import pytest
import pytest_asyncio
import yaml
from kubernetes_asyncio import config as k8s_config
from kubernetes_asyncio.client import (
    ApiClient,
    ApiException,
    CoreV1Api,
    CustomObjectsApi,
    V1Namespace,
    V1ObjectMeta,
    V1Secret,
)
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client

from fixtures.keycloak import keycloak  # noqa: F401 — re-export fixture
from fixtures.entra_mock import entra_mock  # noqa: F401 — re-export fixture

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


@pytest.fixture(scope="session")
def helm_bin():
    """Path to the helm binary."""
    return os.environ.get("HELM_BIN", "helm")


@pytest.fixture(scope="session")
def kubectl_bin():
    """Path to the kubectl binary."""
    return os.environ.get("KUBECTL_BIN", "kubectl")


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
        async with self.connect_mcp_with_auth(token=None) as session:
            yield session

    @asynccontextmanager
    async def connect_mcp_with_auth(self, token: str | None):
        """Connect an MCP client session, optionally with an OAuth Bearer token."""
        headers = {"Authorization": f"Bearer {token}"} if token is not None else {}
        async with httpx.AsyncClient(
            headers=headers,
            timeout=httpx.Timeout(30.0, read=300.0),
            follow_redirects=True,
        ) as http_client:
            async with streamable_http_client(
                f"{self.server_url}/mcp", http_client=http_client,
            ) as (read, write, _):
                async with ClientSession(read, write) as session:
                    session.mcp_init_result = await session.initialize()
                    yield session

    async def raw_mcp_request(self, token: str | None = None) -> int:
        """Make a raw HTTP request to the MCP endpoint and return the status code.

        Useful for testing auth rejection without establishing a full session.
        """
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
        }
        if token is not None:
            headers["Authorization"] = f"Bearer {token}"
        body = (
            b'{"jsonrpc":"2.0","method":"initialize","id":1,'
            b'"params":{"protocolVersion":"2025-03-26",'
            b'"capabilities":{},'
            b'"clientInfo":{"name":"e2e","version":"0.1"}}}'
        )
        async with httpx.AsyncClient(timeout=30.0) as client:
            resp = await client.post(
                f"{self.server_url}/mcp",
                headers=headers,
                content=body,
            )
            return resp.status_code


class GatewayConnection:
    """An MCP gateway reachable via port-forward.

    Wraps the gateway URL and the Host header required by the gateway
    listener so that callers get the same ``connect_mcp()`` interface as
    :class:`ServerDeployment`.
    """

    def __init__(self, url: str, host: str):
        self.url = url
        self.host = host

    @asynccontextmanager
    async def connect_mcp(self):
        """Connect an MCP client session through the gateway."""
        async with httpx.AsyncClient(
            headers={"Host": self.host},
            follow_redirects=True,
            timeout=httpx.Timeout(30.0, read=300.0),
        ) as http_client:
            async with streamable_http_client(
                f"{self.url}/mcp", http_client=http_client,
            ) as (read, write, _):
                async with ClientSession(read, write) as session:
                    session.mcp_init_result = await session.initialize()
                    yield session


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def deploy_server(kubeconfig, chart_path, server_image, helm_bin, kubectl_bin):
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
    async with ApiClient() as api:
        core_v1 = CoreV1Api(api)

        deployments: list[ServerDeployment] = []

        async def _deploy(
            name: str,
            config_toml: str = "",
            extra_values: dict | None = None,
            namespace_setup=None,
        ) -> ServerDeployment:
            namespace = await _create_namespace(core_v1, name)
            success = False
            try:
                if namespace_setup is not None:
                    await namespace_setup(core_v1, namespace)
                await _helm_install(
                    core_v1, namespace, name, chart_path, server_image, config_toml,
                    helm_bin, extra_values,
                )
                server_url, proc = _start_port_forward(namespace, name, kubectl_bin)
                try:
                    await _wait_for_healthz(server_url)
                except BaseException as exc:
                    pf_stderr = ""
                    _kill_proc(proc)
                    try:
                        stderr_file = proc._stderr_file
                        stderr_file.seek(0)
                        pf_stderr = stderr_file.read().decode(errors="replace")
                        stderr_file.close()
                        proc._stdout_file.close()
                    except Exception:
                        pass
                    if isinstance(exc, TimeoutError):
                        diag = await _dump_pod_diagnostics(core_v1, namespace, name)
                        raise RuntimeError(
                            f"Server at {server_url} failed health check.\n"
                            f"--- port-forward stderr ---\n{pf_stderr}\n{diag}"
                        ) from exc
                    raise

                dep = ServerDeployment(name, namespace, server_url)
                dep._port_forward_proc = proc
                deployments.append(dep)
                success = True
                return dep
            finally:
                if not success:
                    try:
                        subprocess.run(
                            [helm_bin, "uninstall", name, "--namespace", namespace],
                            capture_output=True, timeout=120,
                        )
                    except Exception:
                        pass
                    try:
                        await core_v1.delete_namespace(namespace)
                    except Exception:
                        pass

        yield _deploy

        for dep in reversed(deployments):
            try:
                subprocess.run(
                    [helm_bin, "uninstall", dep.name, "--namespace", dep.namespace],
                    capture_output=True,
                    timeout=120,
                )
            except Exception:
                pass
            if dep._port_forward_proc:
                try:
                    _kill_proc(dep._port_forward_proc)
                except Exception:
                    pass
                for attr in ("_stderr_file", "_stdout_file"):
                    fh = getattr(dep._port_forward_proc, attr, None)
                    if fh:
                        try:
                            fh.close()
                        except Exception:
                            pass
            try:
                await core_v1.delete_namespace(dep.namespace)
            except Exception:
                pass


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


async def _create_namespace(core_v1: CoreV1Api, prefix: str) -> str:
    # Retry once on SSL errors caused by stale pooled connections (common
    # when the API server is reached through a TCP proxy).
    for attempt in range(2):
        try:
            ns = await core_v1.create_namespace(
                body=V1Namespace(
                    metadata=V1ObjectMeta(
                        generate_name=f"e2e-{prefix}-",
                        labels={"app.kubernetes.io/managed-by": "e2e-test"},
                    )
                )
            )
            return ns.metadata.name
        except OSError:
            if attempt == 0:
                await asyncio.sleep(0.5)
                continue
            raise


def _parse_image(image: str) -> tuple[str, str, str]:
    """Split a container image reference into (registry, repository, version).

    Handles ``registry/repo:tag``, ``registry:port/repo:tag``, and
    ``repo@sha256:digest`` forms.
    """
    version = "latest"
    # Digest references (name@algo:hash) take precedence over tag
    if "@" in image:
        image, version = image.rsplit("@", 1)
    else:
        # A colon is only a tag separator when it appears in the last path
        # component.  Colons before the last '/' are registry port numbers
        # (e.g. localhost:5000/repo).
        last_slash = image.rfind("/")
        tag_sep = image.find(":", last_slash + 1)
        if tag_sep != -1:
            version = image[tag_sep + 1 :]
            image = image[:tag_sep]

    # The first path component is a registry when it contains '.' or ':'
    # (port) or equals 'localhost'.
    if "/" in image:
        first, rest = image.split("/", 1)
        if "." in first or ":" in first or first == "localhost":
            return first, rest, version
        return "", image, version
    return "", image, version


async def _helm_install(
    core_v1: CoreV1Api,
    namespace: str,
    name: str,
    chart_path: str,
    image: str,
    config_toml: str,
    helm_bin: str,
    extra_values: dict | None = None,
) -> None:
    config = {}
    if config_toml.strip():
        config = tomllib.loads(config_toml)
    # Remove http section — Helm's toToml converts large integers to scientific
    # notation which the TOML parser rejects.
    # https://github.com/helm/helm/issues/32040
    if "http" in config:
        warnings.warn(
            "The [http] config section is dropped before Helm install due to "
            "helm/helm#32040 (toToml mangles large integers). "
            "HTTP settings will use server defaults.",
            stacklevel=2,
        )
        del config["http"]

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
    if extra_values:
        values = merge_extra_values(values, extra_values)

    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".yaml", delete=False
    ) as f:
        yaml.dump(values, f)
        values_file = f.name

    try:
        result = subprocess.run(
            [
                helm_bin, "install", name, chart_path,
                "--namespace", namespace,
                "--values", values_file,
                "--wait",
                "--timeout", "1m",
            ],
            capture_output=True,
            text=True,
            timeout=120,
        )
        if result.returncode != 0:
            diag = await _dump_pod_diagnostics(core_v1, namespace, name)
            raise RuntimeError(
                f"helm install failed:\n{result.stdout}\n{result.stderr}\n{diag}"
            )
    finally:
        os.unlink(values_file)


def _start_port_forward(
    namespace: str, name: str, kubectl_bin: str,
) -> tuple[str, subprocess.Popen]:
    # Use ":SERVER_PORT" so kubectl picks a free port *and* binds it
    # atomically, avoiding the TOCTOU race of finding a port then hoping it
    # stays free until kubectl binds it.
    #
    # Both streams go to temp files rather than PIPEs: kubectl is long-lived
    # and keeps writing to stdout ("Handling connection for <port>" on every
    # forwarded connection), so an undrained PIPE would deadlock once the
    # ~64 KB OS buffer fills.  A file never blocks the writer, and we can poll
    # it for the "Forwarding from" line to learn the port kubectl chose.
    stdout_file = tempfile.TemporaryFile()
    stderr_file = tempfile.TemporaryFile()
    try:
        proc = subprocess.Popen(
            [
                kubectl_bin, "port-forward",
                "-n", namespace,
                f"svc/{name}",
                f":{SERVER_PORT}",
            ],
            stdout=stdout_file,
            stderr=stderr_file,
        )
    except BaseException:
        stdout_file.close()
        stderr_file.close()
        raise
    proc._stdout_file = stdout_file
    proc._stderr_file = stderr_file

    # kubectl prints "Forwarding from 127.0.0.1:<port> -> <remote>" once the
    # local socket is bound (on dual-stack hosts a "[::1]:<port>" line follows;
    # scanning the whole output makes us order-independent).  Poll with a hard
    # deadline so a kubectl that wedges during setup can't hang the suite.
    deadline = time.monotonic() + 30.0
    while time.monotonic() < deadline:
        stdout_file.seek(0)
        output = stdout_file.read().decode(errors="replace")
        m = re.search(r"Forwarding from 127\.0\.0\.1:(\d+)", output)
        if m:
            return f"http://127.0.0.1:{int(m.group(1))}", proc
        if proc.poll() is not None:
            break
        time.sleep(0.1)

    # Failed to learn the port: kubectl exited early or never bound in time.
    # Re-read once more in case a final flush landed after the last poll.
    stdout_file.seek(0)
    out = stdout_file.read().decode(errors="replace")
    m = re.search(r"Forwarding from 127\.0\.0\.1:(\d+)", out)
    if m:
        return f"http://127.0.0.1:{int(m.group(1))}", proc
    _kill_proc(proc)
    stderr_file.seek(0)
    err = stderr_file.read().decode(errors="replace")
    stdout_file.close()
    stderr_file.close()
    raise RuntimeError(
        "kubectl port-forward did not report a local port within 30s "
        f"(exit code {proc.returncode}).\n"
        f"--- stdout ---\n{out}\n--- stderr ---\n{err}"
    )


def _kill_proc(proc: subprocess.Popen) -> None:
    """Terminate a subprocess, escalating to SIGKILL if needed."""
    try:
        proc.terminate()
        proc.wait(timeout=10)
    except OSError:
        pass
    except subprocess.TimeoutExpired:
        proc.kill()
        try:
            proc.wait(timeout=10)
        except (subprocess.TimeoutExpired, OSError):
            pass


async def _wait_for_healthz(url: str, timeout: float = 30.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(f"{url}/healthz", timeout=2):
                return
        except (urllib.error.URLError, OSError):
            await asyncio.sleep(0.5)
    raise TimeoutError(f"Server at {url}/healthz not reachable within {timeout}s")


async def create_file_secret(
    core_v1: CoreV1Api, namespace: str, file_path: str,
    secret_name: str, data_key: str,
) -> None:
    """Create a K8s Secret containing a single file."""
    raw = Path(file_path).read_bytes()
    await core_v1.create_namespaced_secret(
        namespace=namespace,
        body=V1Secret(
            metadata=V1ObjectMeta(name=secret_name),
            data={data_key: base64.b64encode(raw).decode()},
        ),
    )


async def create_ca_cert_secret(
    core_v1: CoreV1Api, namespace: str, ca_cert_path: str,
    secret_name: str = "keycloak-ca-cert",
) -> None:
    """Create a K8s Secret containing a CA certificate."""
    await create_file_secret(core_v1, namespace, ca_cert_path, secret_name, "ca.crt")


KEYCLOAK_CA_MOUNT_PATH = "/etc/ssl/keycloak/ca.crt"


def _toml_escape(s: str) -> str:
    """Escape a string for embedding in a TOML double-quoted value."""
    return (
        s.replace("\\", "\\\\")
        .replace('"', '\\"')
        .replace("\b", "\\b")
        .replace("\f", "\\f")
        .replace("\n", "\\n")
        .replace("\r", "\\r")
        .replace("\t", "\\t")
    )


def toml_extra_lines(**kwargs) -> list[str]:
    """Convert keyword arguments to TOML config lines (bool, int, float, str, list[str])."""
    lines = []
    for k, v in kwargs.items():
        if isinstance(v, bool):
            lines.append(f'{k} = {"true" if v else "false"}')
        elif isinstance(v, (int, float)):
            lines.append(f'{k} = {v}')
        elif isinstance(v, str):
            lines.append(f'{k} = "{_toml_escape(v)}"')
        elif isinstance(v, list):
            items = ", ".join(f'"{_toml_escape(x)}"' for x in v)
            lines.append(f'{k} = [{items}]')
        else:
            raise TypeError(f"unsupported TOML value type for {k!r}: {type(v)}")
    return lines


def oauth_toml(base_lines: list[str], **extra) -> str:
    """Build OAuth TOML config from base lines and extra kwargs.

    Raises ValueError if any kwarg conflicts with a key already set in base_lines.
    """
    known_keys = set()
    for line in base_lines:
        m = re.match(r"(\w+)\s*=", line)
        if m:
            known_keys.add(m.group(1))
    conflicts = known_keys & extra.keys()
    if conflicts:
        raise ValueError(f"kwargs conflict with keys already set: {conflicts}")
    lines = list(base_lines)
    lines.extend(toml_extra_lines(**extra))
    return "\n".join(lines)


def keycloak_oauth_toml(keycloak_issuer: str, audience: str = "mcp-server", **extra) -> str:
    """Build common Keycloak OAuth TOML config."""
    base_lines = [
        'require_oauth = true',
        f'authorization_url = "{_toml_escape(keycloak_issuer)}"',
        f'oauth_audience = "{_toml_escape(audience)}"',
        'oauth_scopes = ["openid", "mcp-server"]',
        f'certificate_authority = "{KEYCLOAK_CA_MOUNT_PATH}"',
    ]
    return oauth_toml(base_lines, **extra)


def merge_extra_values(*dicts):
    """Merge multiple Helm extra_values dicts, concatenating lists and replacing everything else."""
    result = {}
    for d in dicts:
        for key, val in d.items():
            if key in result and isinstance(result[key], list) and isinstance(val, list):
                result[key] = result[key] + val
            else:
                result[key] = val
    return result


KEYCLOAK_CA_EXTRA_VALUES = {
    "extraVolumes": [
        {"name": "keycloak-ca", "secret": {"secretName": "keycloak-ca-cert"}},
    ],
    "extraVolumeMounts": [
        {"name": "keycloak-ca", "mountPath": "/etc/ssl/keycloak", "readOnly": True},
    ],
}


@pytest.fixture(scope="session")
def kind_node_ip(kubectl_bin):
    """InternalIP of the first cluster node (for hostAliases in Kind)."""
    result = subprocess.run(
        [kubectl_bin, "get", "nodes", "-o",
         'jsonpath={.items[0].status.addresses[?(@.type=="InternalIP")].address}'],
        capture_output=True, text=True, timeout=10,
    )
    if result.returncode != 0 or not result.stdout.strip():
        return None
    return result.stdout.strip()


@pytest.fixture(scope="session")
def keycloak_extra_values(kind_node_ip):
    """Helm extra values for Keycloak tests: CA cert volumes + hostAliases."""
    values = dict(KEYCLOAK_CA_EXTRA_VALUES)
    if kind_node_ip:
        values = merge_extra_values(values, {
            "hostAliases": [
                {"ip": kind_node_ip, "hostnames": ["keycloak.127-0-0-1.sslip.io"]},
            ],
        })
    return values


def ca_namespace_setup(ca_cert_path):
    """Return an async callable that creates the Keycloak CA cert Secret."""

    async def setup(core_v1, namespace):
        await create_ca_cert_secret(core_v1, namespace, ca_cert_path)

    return setup


# ---------------------------------------------------------------------------
# Kuadrant MCP Gateway fixtures
# ---------------------------------------------------------------------------

GATEWAY_NAMESPACE = os.environ.get("GATEWAY_NAMESPACE", "gateway-system")
GATEWAY_SERVICE = os.environ.get("GATEWAY_SERVICE", "mcp-gateway-istio")
GATEWAY_HOST = os.environ.get("GATEWAY_HOST", "mcp.127-0-0-1.sslip.io")


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def k8s_api_client(kubeconfig) -> ApiClient:
    """Shared Kubernetes API client.

    Loads the kubeconfig once and provides an :class:`ApiClient` that is
    closed automatically after the test.  All typed client fixtures
    (``k8s_core_v1``, ``k8s_custom_objects``) depend on this rather than
    creating their own connections.
    """
    await k8s_config.load_kube_config(config_file=kubeconfig)
    api = ApiClient()
    yield api
    await api.close()


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def k8s_core_v1(k8s_api_client) -> CoreV1Api:
    """Kubernetes CoreV1Api backed by the shared API client."""
    return CoreV1Api(k8s_api_client)


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def k8s_custom_objects(k8s_api_client) -> CustomObjectsApi:
    """Kubernetes CustomObjectsApi backed by the shared API client."""
    return CustomObjectsApi(k8s_api_client)


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def kuadrant_gateway(k8s_core_v1, kubectl_bin) -> GatewayConnection:
    """Port-forward to the Kuadrant MCP Gateway and yield a GatewayConnection.

    Auto-skips the test if the gateway service is not present in the cluster.
    Use the returned object's ``connect_mcp()`` context manager to open an MCP
    session through the gateway.
    """
    try:
        await k8s_core_v1.read_namespaced_service(
            name=GATEWAY_SERVICE, namespace=GATEWAY_NAMESPACE,
        )
    except ApiException as exc:
        if exc.status != 404:
            raise
        pytest.skip(
            f"Kuadrant MCP Gateway service {GATEWAY_NAMESPACE}/{GATEWAY_SERVICE} "
            f"not found — install with 'make kuadrant-setup'"
        )

    gateway_url, proc = _start_port_forward(
        GATEWAY_NAMESPACE, GATEWAY_SERVICE, kubectl_bin,
    )
    try:
        yield GatewayConnection(gateway_url, GATEWAY_HOST)
    finally:
        _kill_proc(proc)
        for attr in ("_stderr_file", "_stdout_file"):
            fh = getattr(proc, attr, None)
            if fh:
                fh.close()


# ---------------------------------------------------------------------------
# Additional session-scoped fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="session")
def keycloak_ca_cert_path():
    """Path to the cert-manager CA certificate on the test host."""
    path = os.environ.get(
        "KEYCLOAK_CA_CERT",
        str(
            Path(__file__).resolve().parent.parent.parent
            / "_output"
            / "cert-manager-ca"
            / "ca.crt"
        ),
    )
    if not os.path.isfile(path):
        pytest.skip(f"CA cert not found: {path}")
    return path


@pytest.fixture(scope="session")
def server_binary():
    """Path to the pre-built kubernetes-mcp-server binary."""
    path = os.environ.get(
        "SERVER_BINARY",
        str(
            Path(__file__).resolve().parent.parent.parent
            / "kubernetes-mcp-server"
        ),
    )
    if not os.path.isfile(path):
        pytest.skip(f"Server binary not found: {path}")
    return path


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
        def _event_sort_key(e):
            ts = e.last_timestamp or e.event_time
            if ts is None:
                return ""
            return str(ts)

        events = sorted(event_list.items, key=_event_sort_key)
        lines = []
        for event in events:
            ts = event.last_timestamp or event.event_time or ""
            lines.append(f"  {ts}  {event.reason}: {event.message}")
        sections.append("--- Events ---\n" + "\n".join(lines))
    except Exception as exc:
        sections.append(f"--- Events --- (error: {exc})")

    return "\n\n".join(sections)
