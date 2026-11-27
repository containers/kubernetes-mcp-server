# Core MCP Apps evals

This smoke suite exercises two representative MCP Apps tools using
mcpchecker v0.0.23 or newer. Run it with `make run-evals SUITE=core-apps` against
a server started with `make run-server SUITE=core-apps TOOLSETS=core,config`.
See [local setup](../../README.md#mcp-apps-suite-core-apps) for requirements.

| Task | Required app tool | Answer verification |
| --- | --- | --- |
| namespaces | `namespaces_list` | JSON object with fixture namespace and Active phase |
| pods | `pods_get` | JSON object with Pod name, namespace, Running phase, image |

Each tool must be called and supply a readable HTML UI resource with the MCP
Apps MIME type. Missing tools fail instead of skipping coverage. Answer
verification complements resource validation: a tool error can still have a
valid UI resource. Playwright covers UI rendering and interactions.

Answers must be a single JSON object with exactly the requested fields. The
verifiers use `jq` to compare the complete object, so values mentioned for
different resources cannot satisfy independent substring checks. `make run-evals`
installs `jq` and puts it on the task scripts' PATH; direct mcpchecker runs need
`jq` available on PATH too.

Keep `requirePresent: true` alongside each task's `apps` assertions: mcpchecker
v0.0.23 resolves task and eval assertion sets independently, so the eval's
presence setting would not apply to the task's Apps assertions.

Each task owns a labeled namespace (`mcp-apps-namespaces` or `mcp-apps-pods`).
Setup creates the namespace and fails if it already exists; cleanup removes it
only if it carries this suite's ownership label. Deletion uses a UID precondition
to protect a replacement namespace created after the ownership check. Setup
rolls back its namespace on failure, since mcpchecker does not run task cleanup
when setup fails. Avoid concurrent runs of the same task against the same
cluster. The Pod fixture uses
`registry.k8s.io/pause:3.10`; the cluster must be able to pull that image.

If cleanup times out, a fixture namespace may remain and block the next setup.
After confirming no `core-apps` eval is running against this cluster, remove
leftover namespaces owned by this suite and retry:

```bash
kubectl delete namespace -l evals.kubernetes-mcp-server/suite=core-apps --timeout=120s
```

Setup deliberately does not delete pre-existing namespaces, which could belong
to another run. If deletion still times out, inspect the terminating namespace
and its remaining resources before retrying.

Metrics, events, generic resources, and additional Pod listing scenarios can be
added later. This suite requires no Metrics API or OpenShift-specific APIs.
