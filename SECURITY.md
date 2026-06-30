# Security Policy

The Kubernetes MCP Server maintainers and community take the security of the project seriously.
We appreciate your efforts to responsibly disclose your findings and will work with you to resolve them.

## Supported Versions

Security fixes are applied to the **latest released version** of `kubernetes-mcp-server`.
We do not maintain long-term support or backport branches, so we strongly encourage everyone to stay on the most recent release.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, pull requests, or discussions.**

Instead, report them privately through GitHub's built-in private vulnerability reporting:

1. Go to the repository's [**Report a vulnerability**](https://github.com/containers/kubernetes-mcp-server/security/advisories/new) page, also reachable from the repository's [**Security** tab](https://github.com/containers/kubernetes-mcp-server/security).
2. Fill in the advisory form and submit it.

This creates a private advisory visible only to the project maintainers.

To help us triage and prioritize the report, please include as much of the following as you can:

- The type of issue (e.g. credential exposure, privilege escalation, command injection, SSRF).
- The affected version(s) of `kubernetes-mcp-server`, and the relevant configuration (e.g. enabled toolsets, transport, read-only mode).
- Step-by-step instructions to reproduce the issue.
- Proof-of-concept or exploit code, if available.
- The impact of the issue, including how an attacker might exploit it.

Reports written in English are preferred.

## What to Expect

- We will acknowledge your report as soon as possible and keep you informed as we triage it and work toward a fix.
- The repository maintainers triage incoming reports.
  Issues that also affect downstream distributions are additionally handled through Red Hat's internal product security process.
- We will work with you privately to understand and resolve the issue before any public disclosure, and we are happy to credit you in the resulting advisory unless you prefer to remain anonymous.

## Disclosure and Announcements

Once a fix is available, we publish the details as a [GitHub Security Advisory](https://github.com/containers/kubernetes-mcp-server/security/advisories) and include them in the corresponding [release notes](https://github.com/containers/kubernetes-mcp-server/releases).
We recommend watching the repository's releases to stay informed about security updates.
