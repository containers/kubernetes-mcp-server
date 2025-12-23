# kubernetes-mcp-server

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![AppVersion: latest](https://img.shields.io/badge/AppVersion-latest-informational?style=flat-square)

Helm Chart for the Kubernetes MCP Server

**Homepage:** <https://github.com/containers/kubernetes-mcp-server>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Andrew Block | <ablock@redhat.com> |  |
| Marc Nuri | <marc.nuri@redhat.com> |  |

## Installing the Chart

The Chart can be installed quickly and easily to a Kubernetes cluster. Since an _Ingress_ is added as part of the default install of the Chart, the `ingress.host` Value must be specified.

Install the Chart using the following command from the root of this directory:

```shell
helm upgrade -i -n kubernetes-mcp-server --create-namespace kubernetes-mcp-server oci://ghcr.io/containers/charts/kubernetes-mcp-server --set ingress.host=<hostname>
```

### Optimized OpenShift Deployment

Functionality has been added to the Chart to simplify the deployment to OpenShift Cluster.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| config.port | string | `"{{ .Values.service.port }}"` |  |
| configFilePath | string | `"/etc/kubernetes-mcp-server/config.toml"` |  |
| defaultPodSecurityContext | object | `{"seccompProfile":{"type":"RuntimeDefault"}}` | Default Security Context for the Pod when one is not provided |
| defaultSecurityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"runAsNonRoot":true}` | Default Security Context for the Container when one is not provided |
| extraVolumeMounts | list | `[]` | Additional volumeMounts on the output Deployment definition. |
| extraVolumes | list | `[]` | Additional volumes on the output Deployment definition. |
| fullnameOverride | string | `""` |  |
| image | object | `{"pullPolicy":"IfNotPresent","registry":"quay.io","repository":"containers/kubernetes_mcp_server","version":"latest"}` | This sets the container image more information can be found here: https://kubernetes.io/docs/concepts/containers/images/ |
| image.pullPolicy | string | `"IfNotPresent"` | This sets the pull policy for images. |
| image.version | string | `"latest"` | This sets the tag or sha digest for the image. |
| imagePullSecrets | list | `[]` | This is for the secrets for pulling an image from a private repository more information can be found here: https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/ |
| ingress | object | `{"annotations":{},"className":"","enabled":true,"host":"","hosts":null,"path":"/","pathType":"ImplementationSpecific","termination":"edge","tls":null}` | This block is for setting up the ingress for more information can be found here: https://kubernetes.io/docs/concepts/services-networking/ingress/ |
| livenessProbe | object | `{"httpGet":{"path":"/healthz","port":"http"}}` | Liveness and readiness probes for the container. |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| openshift | bool | `false` | Enable OpenShift specific features |
| podAnnotations | object | `{}` | For more information checkout: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/ |
| podLabels | object | `{}` | For more information checkout: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/ |
| podSecurityContext | object | `{}` | Define the Security Context for the Pod |
| readinessProbe.httpGet.path | string | `"/healthz"` |  |
| readinessProbe.httpGet.port | string | `"http"` |  |
| replicaCount | int | `1` | This will set the replicaset count more information can be found here: https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/ |
| resources | object | `{"limits":{"cpu":"100m","memory":"128Mi"},"requests":{"cpu":"100m","memory":"128Mi"}}` | Resource requests and limits for the container. |
| securityContext | object | `{}` | Define the Security Context for the Container |
| service | object | `{"port":8080,"type":"ClusterIP"}` | This is for setting up a service more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/ |
| service.port | int | `8080` | This sets the ports more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/#field-spec-ports |
| service.type | string | `"ClusterIP"` | This sets the service type more information can be found here: https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types |
| serviceAccount | object | `{"annotations":{},"create":true,"name":""}` | This section builds out the service account more information can be found here: https://kubernetes.io/docs/concepts/security/service-accounts/ |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.name | string | `""` | If not set and create is true, a name is generated using the fullname template |
| tolerations | list | `[]` |  |

## Updating the README

The contents of the README.md file is generated using [helm-docs](https://github.com/norwoodj/helm-docs). Whenever changes are introduced to the Chart and its _Values_, the documentation should be regenerated.

Execute the following command to regenerate the documentation from within the Helm Chart directory.

```shell
helm-docs -t README.md.gotmpl
```
