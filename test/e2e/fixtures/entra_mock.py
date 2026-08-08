"""In-cluster mock Entra ID (Azure AD) STS fixture for e2e tests.

Deploys a lightweight mock STS as a Kubernetes Deployment+Service inside
the Kind cluster, eliminating host-networking dependencies.  Works
identically on Docker and rootless podman.
"""

from __future__ import annotations

import json
import os
import subprocess
import time
import urllib.request
import uuid
from pathlib import Path

import jwt
import pytest
import pytest_asyncio

from fixtures.kubectl_helpers import kill_proc, start_port_forward

# ---------------------------------------------------------------------------
# Static RSA private key (test-only, matches the key in entra-mock/server.py)
# ---------------------------------------------------------------------------

_PRIVATE_KEY_PEM = b"""-----BEGIN PRIVATE KEY-----
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

# ---------------------------------------------------------------------------
# Constants (must match entra-mock/deployment.yaml and entra-mock/server.py)
# ---------------------------------------------------------------------------

_NAMESPACE = "entra-mock"
_SERVICE_URL = "http://entra-mock.entra-mock.svc.cluster.local:8080"
_MANIFEST = str(
    Path(__file__).resolve().parent.parent / "entra-mock" / "deployment.yaml"
)


class EntraMockClient:
    """Client for the in-cluster Entra mock STS.

    Provides the same API surface as the old host-based EntraMockSTS so
    that test code requires zero changes.
    """

    CLIENT_ID = "mock-entra-client"
    CLIENT_SECRET = "mock-entra-secret"
    VALID_SCOPE = "api://default/.default"

    def __init__(self, admin_url: str):
        self._admin_url = admin_url
        self._oauth_authz_server_enabled = True

    @property
    def issuer_url(self) -> str:
        return _SERVICE_URL

    @property
    def client_id(self) -> str:
        return self.CLIENT_ID

    @property
    def client_secret(self) -> str:
        return self.CLIENT_SECRET

    def get_user_token(self) -> str:
        """Generate a user token locally using the static keypair."""
        return jwt.encode(
            {
                "sub": "test-user",
                "aud": self.CLIENT_ID,
                "iss": _SERVICE_URL,
                "exp": int(time.time()) + 3600,
                "iat": int(time.time()),
                "jti": str(uuid.uuid4()),
            },
            _PRIVATE_KEY_PEM,
            algorithm="RS256",
            headers={"kid": "test-kid"},
        )

    @property
    def oauth_authz_server_enabled(self) -> bool:
        return self._oauth_authz_server_enabled

    @oauth_authz_server_enabled.setter
    def oauth_authz_server_enabled(self, value: bool):
        self._oauth_authz_server_enabled = value
        body = json.dumps({"oauth_authz_server_enabled": value}).encode()
        req = urllib.request.Request(
            f"{self._admin_url}/admin/config",
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        urllib.request.urlopen(req, timeout=5)


def _wait_for_mock(url: str, timeout: float = 30.0) -> None:
    """Poll the mock's OIDC discovery endpoint until it responds."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            urllib.request.urlopen(
                f"{url}/.well-known/openid-configuration", timeout=2,
            )
            return
        except (urllib.error.URLError, OSError):
            time.sleep(0.5)
    raise TimeoutError(f"Entra mock at {url} not reachable within {timeout}s")


@pytest_asyncio.fixture(loop_scope="session", scope="session")
async def entra_mock(kubectl_bin):
    """Session-scoped in-cluster Entra mock STS.

    Deploys the mock as a Deployment+Service, starts a port-forward for
    the admin API, and yields an EntraMockClient.

    Skips all dependent tests if the mock image is not loaded into Kind
    or the pod fails to start.
    """
    result = subprocess.run(
        [kubectl_bin, "apply", "-f", _MANIFEST],
        capture_output=True, text=True, timeout=30,
    )
    if result.returncode != 0:
        pytest.skip(
            f"Failed to deploy entra-mock (is the image loaded? "
            f"run 'make e2e-image'): {result.stderr.strip()}"
        )

    try:
        result = subprocess.run(
            [kubectl_bin, "wait", "--for=condition=ready", "pod",
             "-l", "app=entra-mock", "-n", _NAMESPACE,
             "--timeout=60s"],
            capture_output=True, text=True, timeout=90,
        )
        if result.returncode != 0:
            _dump_and_skip(kubectl_bin, result.stderr)

        admin_url, proc = start_port_forward(
            _NAMESPACE, "entra-mock", kubectl_bin,
        )
        try:
            _wait_for_mock(admin_url)
            yield EntraMockClient(admin_url)
        finally:
            kill_proc(proc)
            for attr in ("_stderr_file", "_stdout_file"):
                fh = getattr(proc, attr, None)
                if fh:
                    try:
                        fh.close()
                    except Exception:
                        pass
    finally:
        subprocess.run(
            [kubectl_bin, "delete", "namespace", _NAMESPACE,
             "--ignore-not-found", "--wait=false"],
            capture_output=True, timeout=30,
        )


def _dump_and_skip(kubectl_bin: str, wait_stderr: str):
    """Collect pod diagnostics and skip with a useful message."""
    diag_parts = [f"kubectl wait failed: {wait_stderr.strip()}"]
    for cmd_label, cmd in [
        ("pods", [kubectl_bin, "get", "pods", "-n", _NAMESPACE, "-o", "wide"]),
        ("events", [kubectl_bin, "get", "events", "-n", _NAMESPACE,
                    "--sort-by=.lastTimestamp"]),
        ("logs", [kubectl_bin, "logs", "-n", _NAMESPACE, "-l", "app=entra-mock",
                  "--tail=20"]),
    ]:
        try:
            r = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
            diag_parts.append(f"--- {cmd_label} ---\n{r.stdout.strip()}")
        except Exception:
            pass

    subprocess.run(
        [kubectl_bin, "delete", "namespace", _NAMESPACE,
         "--ignore-not-found", "--wait=false"],
        capture_output=True, timeout=30,
    )
    pytest.skip(
        "Entra mock pod not ready (is the image loaded? "
        f"run 'make e2e-image'):\n" + "\n".join(diag_parts)
    )
