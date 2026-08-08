"""Startup configuration validation tests.

These tests run the compiled server binary with invalid TOML configs and
verify that it exits with an appropriate error message.  They do NOT
require a running Kubernetes cluster.
"""

from __future__ import annotations

import os
import subprocess
import tempfile

import pytest

from conftest import _toml_escape


pytestmark = pytest.mark.validation


def _run_server(binary: str, config_toml: str, extra_args: list[str] | None = None) -> subprocess.CompletedProcess:
    """Run the server binary with a TOML config and return the result.

    Uses ``--version --port 8080`` so the binary runs through
    Complete → Validate → Run (prints version and exits).  Invalid
    configs fail at the Validate step with a non-zero exit code.
    """
    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".toml", delete=False,
    ) as f:
        f.write(config_toml)
        config_path = f.name

    args = [binary, "--config", config_path, "--port", "8080", "--version"]
    if extra_args:
        args.extend(extra_args)
    try:
        return subprocess.run(args, capture_output=True, text=True, timeout=15)
    finally:
        os.unlink(config_path)


def _assert_fails_with(binary: str, config_toml: str, expected_error: str, extra_args: list[str] | None = None):
    result = _run_server(binary, config_toml, extra_args)
    assert result.returncode != 0, (
        f"Expected non-zero exit code.\nstdout: {result.stdout}\nstderr: {result.stderr}"
    )
    combined = result.stdout + result.stderr
    assert expected_error in combined, (
        f"Expected error containing {expected_error!r}.\n"
        f"stdout: {result.stdout}\nstderr: {result.stderr}"
    )


# ---- OAuth fields without require_oauth ----

def test_oauth_fields_without_oauth(server_binary):
    _assert_fails_with(
        server_binary,
        'oauth_audience = "mcp-server"',
        "only valid if require-oauth",
    )


def test_server_url_without_oauth(server_binary):
    _assert_fails_with(
        server_binary,
        'server_url = "https://example.com"',
        "only valid if require-oauth",
    )


# ---- Token exchange without OAuth ----

def test_sts_without_oauth(server_binary):
    _assert_fails_with(
        server_binary,
        'sts_audience = "openshift"',
        "token exchange requires require_oauth",
    )


# ---- Invalid cluster_auth_mode ----

def test_invalid_cluster_auth_mode(server_binary):
    _assert_fails_with(
        server_binary,
        'cluster_auth_mode = "bogus"',
        "invalid cluster_auth_mode",
    )


def test_kubeconfig_mode_with_oauth(server_binary):
    _assert_fails_with(
        server_binary,
        """\
cluster_auth_mode = "kubeconfig"
require_oauth = true
authorization_url = "https://example.com"
""",
        "not compatible with require_oauth",
    )


def test_kubeconfig_mode_with_sts(server_binary):
    _assert_fails_with(
        server_binary,
        """\
cluster_auth_mode = "kubeconfig"
sts_audience = "openshift"
""",
        "incompatible with cluster_auth_mode",
    )


# ---- Invalid STS auth style ----

def test_invalid_sts_auth_style(server_binary):
    _assert_fails_with(
        server_binary,
        'sts_auth_style = "bogus"',
        "invalid sts_auth_style",
    )


# ---- Invalid token exchange strategy ----

def test_invalid_token_exchange_strategy(server_binary):
    _assert_fails_with(
        server_binary,
        """\
require_oauth = true
skip_jwt_verification = true
token_exchange_strategy = "bogus"
""",
        "invalid token_exchange_strategy",
    )


def test_assertion_style_missing_cert(server_binary):
    _assert_fails_with(
        server_binary,
        'sts_auth_style = "assertion"',
        "sts_client_cert_file is required",
    )


def test_assertion_style_missing_key(server_binary):
    with tempfile.NamedTemporaryFile(suffix=".pem", delete=False) as f:
        f.write(b"placeholder")
        cert_path = f.name
    try:
        _assert_fails_with(
            server_binary,
            f"""\
sts_auth_style = "assertion"
sts_client_cert_file = "{_toml_escape(cert_path)}"
""",
            "sts_client_key_file is required",
        )
    finally:
        os.unlink(cert_path)


# ---- TLS validation ----

def test_tls_cert_without_key(server_binary):
    with tempfile.NamedTemporaryFile(suffix=".pem", delete=False) as f:
        f.write(b"placeholder")
        cert_path = f.name
    try:
        _assert_fails_with(
            server_binary,
            f'tls_cert = "{_toml_escape(cert_path)}"',
            "both --tls-cert and --tls-key",
        )
    finally:
        os.unlink(cert_path)


def test_http_authorization_url_accepted(server_binary):
    """Config validation accepts authorization_url with plain HTTP scheme."""
    result = _run_server(server_binary, """\
require_oauth = true
authorization_url = "http://example.com"
skip_jwt_verification = true
""")
    assert result.returncode == 0, (
        f"Expected server to accept HTTP authorization_url.\n"
        f"stdout: {result.stdout}\nstderr: {result.stderr}"
    )


def test_require_tls_with_http_url(server_binary):
    _assert_fails_with(
        server_binary,
        """\
require_tls = true
require_oauth = true
authorization_url = "http://example.com"
skip_jwt_verification = true
""",
        "secure scheme required",
    )


# ---- File path validation ----

def test_nonexistent_ca_path(server_binary):
    _assert_fails_with(
        server_binary,
        """\
require_oauth = true
authorization_url = "https://example.com"
certificate_authority = "/nonexistent/ca.crt"
""",
        "valid file path",
    )


# ---- Confirmation rules ----

def test_invalid_confirmation_fallback(server_binary):
    _assert_fails_with(
        server_binary,
        'confirmation_fallback = "bogus"',
        "invalid confirmation_fallback",
    )


# ---- HTTP rate limits ----

def test_negative_rate_limit_rps(server_binary):
    _assert_fails_with(
        server_binary,
        """\
[http]
rate_limit_rps = -1
""",
        "rate_limit_rps",
    )


def test_negative_rate_limit_burst(server_binary):
    _assert_fails_with(
        server_binary,
        """\
[http]
rate_limit_burst = -1
""",
        "rate_limit_burst",
    )
