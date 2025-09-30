# Kubernetes MCP Server

A Helm chart for the Kubernetes Model Context Protocol (MCP) server.

## Prerequisites

- Kubernetes 1.16+
- Helm 3+

## Installing the Chart

To install the chart with the release name `my-release`:

```bash
helm install my-release .
```

The command deploys the Kubernetes MCP server on the Kubernetes cluster in the default configuration. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```bash
helm delete my-release
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Parameters

The following table lists the configurable parameters of the Kubernetes MCP server chart and their default values.

| Parameter                  | Description                                                                                                                               | Default                                 |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| `replicaCount`             | Number of replicas to deploy.                                                                                                             | `1`                                     |
| `image.repository`         | Image repository.                                                                                                                         | `aihorde/kubernetes-mcp-server`       |
| `image.pullPolicy`         | Image pull policy.                                                                                                                        | `IfNotPresent`                          |
| `image.tag`                | Image tag. Overrides the chart's `appVersion`.                                                                                            | `""`                                    |
| `imagePullSecrets`         | Image pull secrets.                                                                                                                       | `[]`                                    |
| `nameOverride`             | String to override the name of the chart.                                                                                                 | `""`                                    |
| `fullnameOverride`         | String to override the fully qualified app name.                                                                                          | `""`                                    |
| `serviceAccount.create`    | If `true`, a new service account is created. If `false`, you must provide the name of an existing service account in `serviceAccount.name`. | `false`                                 |
| `serviceAccount.name`      | The name of the service account to use. Required if `serviceAccount.create` is `false`.                                                   | `""`                                    |
| `podAnnotations`           | Annotations to add to the pod.                                                                                                            | `{}`                                    |
| `podSecurityContext`       | Pod security context.                                                                                                                     | `{}`                                    |
| `securityContext`          | Container security context.                                                                                                               | `{}`                                    |
| `service.type`             | Service type.                                                                                                                             | `ClusterIP`                             |
| `service.port`             | Service port.                                                                                                                             | `8080`                                  |
| `ingress.enabled`          | Enable ingress.                                                                                                                           | `false`                                 |
| `ingress.className`        | Ingress class name.                                                                                                                       | `""`                                    |
| `ingress.annotations`      | Ingress annotations.                                                                                                                      | `{}`                                    |
| `ingress.hosts`            | Ingress hosts.                                                                                                                            | `[]`                                    |
| `ingress.tls`              | Ingress TLS configuration.                                                                                                                | `[]`                                    |
| `resources`                | Resource requests and limits.                                                                                                             | `{}`                                    |
| `autoscaling.enabled`      | Enable autoscaling.                                                                                                                       | `false`                                 |
| `nodeSelector`             | Node selector.                                                                                                                            | `{}`                                    |
| `tolerations`              | Tolerations.                                                                                                                              | `[]`                                    |
| `affinity`                 | Affinity.                                                                                                                                 | `{}`                                    |
| `rbac.create`              | If `true`, a `Role` and `RoleBinding` will be created for the service account. If `false`, the chart will rely on existing permissions.   | `true`                                  |
| `config`                   | Application-specific configuration. See the [Application Configuration](#application-configuration) section for more details.             | `{}`                                    |

## Application Configuration

The `config` parameter allows you to configure the Kubernetes MCP server. The following options are available:

| Parameter                      | Description                                                                    | Default     |
| ------------------------------ | ------------------------------------------------------------------------------ | ----------- |
| `logLevel`                     | Log level (from 0 to 9).                                                       | `0`         |
| `readOnly`                     | If true, only tools annotated with `readOnlyHint=true` are exposed.            | `true`      |
| `disableDestructive`           | If true, tools annotated with `destructiveHint=true` are disabled.             | `true`      |
| `toolsets`                     | List of MCP toolsets to use.                                                   | `[]`        |
| `denied_resources`             | List of resources to deny access to.                                           | `[]`        |
| `confluence.url`               | URL of the Confluence instance.                                                | `""`        |
| `confluence.username`          | Confluence username.                                                           | `""`        |
| `confluence.token`             | Confluence API token. **It is strongly recommended to set this via `--set-string` in a CI/CD pipeline rather than in the values file.** | `""`        |
| `oauth.require`                | If true, requires OAuth authorization.                                         | `false`     |
| `oauth.audience`               | OAuth audience for token claims validation.                                    | `""`        |
| `oauth.validateToken`          | If true, validates the token against the Kubernetes API Server.                | `false`     |
| `oauth.authorizationUrl`       | OAuth authorization server URL.                                                | `""`        |
| `oauth.serverUrl`              | Server URL of this application.                                                | `""`        |
| `oauth.certificateAuthority`   | Path to the certificate authority file to verify certificates.                 | `""`        |