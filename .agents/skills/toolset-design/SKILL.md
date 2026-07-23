---
name: toolset-design
description: >
  Design and validate new MCP toolsets, tools, and tool changes. Use when planning a new
  toolset, adding tools to an existing toolset, reviewing a toolset PR, or deciding whether
  a new tool is warranted. Covers the full lifecycle: eval-first validation, tool design
  methodology, consolidation patterns, and review criteria.
when_to_use: >
  Trigger when: user wants to add a new toolset, add tools to a toolset, review a toolset
  PR, decide if a domain needs dedicated tools, plan tool parameters, or evaluate whether
  existing tools already cover a use case. Also useful for "should this be one tool or
  three?" type design questions.
---

# Toolset Design Guide

This skill codifies the methodology for designing MCP tools and toolsets in this repository.
The core principle is **eval-first, tools-second**: prove the need before writing the code.

---

## Phase 1: Validate the need with evals

Before writing any toolset code, write eval tasks that represent what a user would actually
ask an LLM to do in the target domain. This serves two purposes:

1. **Baseline**: Run the evals with only the existing toolsets enabled (core, config, etc.).
   If the LLM can already accomplish the tasks using `pods_exec`, `resources_list`, or other
   generic tools, you may not need dedicated tools at all.

2. **Gap identification**: The tasks where the baseline fails (or produces poor results)
   reveal the actual gaps that new tools should fill.

### How to write eval tasks

Create tasks under `evals/tasks/<domain>/` using the `mcpchecker/v1alpha2` format.
See existing tasks in `evals/tasks/` for examples at different difficulty levels.

Design tasks that represent real user workflows, not tool-shaped requests:

```yaml
# BAD: This is testing a tool, not a workflow
prompt:
  inline: Run helm list --all-namespaces

# GOOD: This is testing what a user would actually ask
prompt:
  inline: What Helm releases are deployed across the cluster?

# BETTER: This tests whether the LLM can solve a real problem
prompt:
  inline: >
    The application in namespace "payments" was working yesterday but
    is failing after a recent Helm upgrade. Investigate what changed
    in the latest release and identify the issue.
```

**Task difficulty should span the range:**
- **Easy**: Simple queries (list resources, show config)
- **Medium**: Filtered/analyzed output (find specific entries matching a pattern, correlate data across sources)
- **Hard**: Multi-step diagnosis (investigate root cause, identify misconfiguration, suggest fix)

### Running the baseline

```bash
# Run evals with only core+config toolsets to establish baseline
mcpchecker check evals/core-eval-testing/<agent>/eval-core.yaml --label-selector suite=<your-domain>
```

**What to look for in baseline results:**
- If the LLM uses `pods_exec` successfully -> dedicated tool adds only convenience
- If the LLM fails because it doesn't know the right commands -> tool descriptions add value
- If the LLM succeeds but output is noisy/unparsed -> filtering/pagination adds value
- If the LLM can't accomplish the task at all -> new capability is needed

### Decision framework

| Baseline result | Action |
|----------------|--------|
| LLM succeeds reliably with existing tools | Don't add new tools. Consider adding a **prompt** instead to guide the LLM's approach. |
| LLM succeeds but results are messy | Consider a tool only if it adds validation, filtering, or structured output that `pods_exec` can't provide. |
| LLM fails because it lacks domain knowledge | Add a **prompt** that provides the domain context. Re-test before adding tools. |
| LLM fails because the operation is genuinely new | Add tools. Proceed to Phase 2. |

---

## Phase 2: Design the tools

### The key question: tools vs. prompts

Before designing tools, ask whether a **prompt** would solve the problem instead.

- **Tools** execute operations and return results. They're for actions the LLM needs to perform.
- **Prompts** gather diagnostic data and frame it for LLM analysis. They're for workflows
  where the LLM needs context to reason about a problem.

See the kubevirt toolset for a good example: `vm-troubleshoot` is a prompt (not a tool) that
gathers VM status, pod logs, and events, then presents them with analysis instructions.
The core toolset's `cluster-health-check` prompt follows the same pattern.

**Rule of thumb**: If the value is in *what data to gather and how to think about it*,
use a prompt. If the value is in *executing a specific operation safely*, use a tool.

**Anti-pattern: instructions instead of data.** A prompt or tool that returns a to-do
list for the model ("call `resources_get`, then `pods_list`…") wastes tokens, adds
round-trips, and hands a deterministic step to a stochastic actor. Instead, use
**dynamic context injection** — the handler pre-fetches the data and injects it into
the prompt text so the model receives facts to analyze, not steps to follow.

```yaml
# BAD: output is a to-do list for the model
"To troubleshoot, call resources_get on the VM, then pods_list,
 then events_list, and look for errors."

# GOOD: the server gathers what it knows is needed
<VM + VMI + virt-launcher pod + logs + events, already fetched>
"Analyze the data above: check printableStatus, pod restarts, Warning events."
```

If the server can know in advance what to fetch, it should fetch it.

### Tool consolidation: fewer tools is better

Every tool added to the MCP server increases cognitive load for the LLM. The LLM must
read every tool's name and description to decide which one to use. Fewer, well-designed
tools outperform many narrow ones.

**The enum/subcommand pattern**

When multiple operations share the same parameters and differ only in the action performed,
consolidate them into a single tool with an enum parameter.

The kubevirt toolset demonstrates this well:
- `vm_lifecycle` with `action: start | stop | restart` (3 operations, 1 tool)
- `vm_guest_info` with `info_type: all | os | filesystem | users | network` (5 operations, 1 tool)

See `pkg/toolsets/kubevirt/vm/lifecycle/tool.go` and
`pkg/toolsets/kubevirt/vm/guestagent/tool.go` for implementation.

**When to consolidate:**
- Operations share the same required parameters (namespace, name, etc.)
- Operations are in the same conceptual category (same CLI binary, same resource type)
- The LLM doesn't need to distinguish between them at the tool-selection level

**When NOT to consolidate:**
- Operations have fundamentally different parameter shapes
- You need different access control per operation (read-only vs destructive)
- The operations serve different user intents (diagnosis vs modification)

**Grouping heuristic**: Group by the CLI binary or API the tool wraps.
If you're wrapping subcommands of the same CLI tool, that's one MCP tool with
a subcommand enum. Don't create separate MCP tools for `list-x` and `list-y`
when they're both subcommands of the same binary with identical base parameters.

### Do core toolset tools already cover this?

Before designing any new tool, check whether the core toolset already handles the
use case. The two most common sources of unnecessary tools:

**1. CRUD on Kubernetes resources**

The core toolset provides `resources_list`, `resources_get`, `resources_create_or_update`,
`resources_delete`, and `resources_scale`. These work with **any** Kubernetes resource type
via GVK (group/version/kind) parameters. If the proposed tool is essentially "list CRDs of
type X" or "get a specific custom resource", it's already covered.

A dedicated tool for a specific resource type is only justified when:
- The tool does significant **post-processing** beyond what `resources_list`/`resources_get` return
  (e.g., correlating data across multiple resources, computing derived fields)
- The tool provides a **domain-specific workflow** that spans multiple API calls
  (e.g., kubevirt's `vm-troubleshoot` prompt gathers VM + VMI + pod + events in one shot)
- The resource requires **specialized input** that the generic tools can't express
  (e.g., Helm releases aren't plain Kubernetes resources)

If none of these apply, the tool is just a thinner wrapper around `resources_get` and
should not be added.

**2. Running commands in pods**

The core toolset provides `pods_exec` which can run arbitrary commands in pods.
A dedicated tool wrapping a pod exec is only justified if it provides value beyond
what `pods_exec` offers:

| Value-add | Example | Justifies a new tool? |
|-----------|---------|----------------------|
| **Input validation** | Sanitizing user-supplied args against injection | Yes, if the command has injection risk |
| **Output parsing/filtering** | Pattern matching, head/tail pagination on large output | Yes, if output is routinely large |
| **Structured output** | Returning JSON with typed fields instead of raw text | Moderate -- helps programmatic consumers |
| **Discoverability** | LLM doesn't need to know domain-specific CLI syntax | Moderate -- but a prompt can also teach this |
| **Convenience only** | Saves the LLM from typing a longer command | No -- not sufficient justification |

### Tool parameter design

**Required vs optional parameters:**
- Make parameters required only when the tool cannot function without them
- Use sensible defaults for optional parameters (e.g., head defaults to 100 lines)
- Use `DependentRequired` for conditional requirements (see `pkg/toolsets/kiali/tools/list_or_get_resources.go`)

**Parameter naming conventions:**
- Use `snake_case` for parameter names
- Use the same name as the upstream CLI flag where possible
- Common shared parameters: `namespace`, `name`, `container`, `pattern`, `head`, `tail`

**Conditional parameters with enums:**
When a tool uses an enum subcommand and different subcommands need different parameters,
document which parameters apply to which subcommand in the tool description.
Make conditionally-required parameters technically optional and validate in the handler.

### Tool annotations

Every tool must set appropriate annotations:

- `ReadOnlyHint: true` for diagnostic/query tools
- `DestructiveHint: true` for tools that delete or modify resources
- `IdempotentHint: true` if repeated calls with same args have no additional effect
- `OpenWorldHint: true` if the tool interacts with a live cluster (most tools)

See `pkg/api/toolsets.go` for annotation definitions.

### Tool descriptions

Write descriptions for the LLM, not for humans reading docs:

- **First sentence**: What the tool does (verb phrase)
- **Second sentence**: When/why to use it (context)
- **Keep it concise**: The description consumes tokens on every LLM call

### Returning results

Use `api.NewToolCallResultStructured(result, err)` for tools that return typed data.
This populates both text content and structured content for MCP clients.

Avoid manual `json.Marshal` + `api.NewToolCallResult` -- that's the older pattern.

---

## Phase 3: Validate the design with evals

After implementing the tools, update the eval tasks to assert the new tools are used:

```yaml
assertions:
  toolsUsed:
    - server: kubernetes
      toolPattern: "my_domain_.*"  # Assert the new tools are called
  minToolCalls: 1
  maxToolCalls: 20
```

Then run the evals again. Compare against the baseline:
- Do the evals pass at a higher rate?
- Does the LLM use the new tools correctly, or does it still fall back to `pods_exec`?
- Are there tools you added that the LLM never uses? Those might not be needed.

Each toolset gets its own suite (e.g. `suite: my-domain`) and a corresponding
`eval-<suite>.yaml` per agent. Create `evals/core-eval-testing/<agent>/eval-my-domain.yaml`
for each agent that has eval configs. The eval file should include the `core` and
`config` task sets alongside the new suite's tasks (see existing files for the pattern).

---

## Phase 4: Implementation checklist

Once the design is validated, the implementation follows a mechanical checklist.
See `CLAUDE.md` for full details on each step. The key files to touch:

1. **Toolset package**: `pkg/toolsets/<name>/toolset.go` -- implements `api.Toolset` interface
2. **Tool definitions**: `pkg/toolsets/<name>/<group>/tools.go` -- tool schemas and handlers
3. **Module import**: `pkg/mcp/modules.go` -- add blank import for auto-registration
4. **Snapshot test**: `pkg/mcp/toolsets_test.go` -- add to `TestGranularToolsetsTools`
5. **Generate snapshots**: `UPDATE_TOOLSETS_JSON=1 go test -count=1 -v ./pkg/mcp`
6. **Update docs**: `make update-readme-tools`
7. **Eval configs**: `evals/core-eval-testing/<agent>/eval-<suite>.yaml` (one per toolset per agent)
8. **Eval tasks**: `evals/tasks/<domain>/*/task.yaml`

---

## Design review checklist

When reviewing a toolset PR, verify:

- [ ] **Eval-first**: Are there eval tasks? Do they test real user workflows, not tool-shaped requests?
- [ ] **Baseline tested**: Was the domain tested with existing tools first? Is there evidence the new tools improve outcomes?
- [ ] **Tool count justified**: Could any tools be consolidated using enum parameters? Does each tool provide value beyond `pods_exec`?
- [ ] **Prompts considered**: Would a diagnostic prompt serve better than (or alongside) some tools?
- [ ] **Annotations correct**: Are read-only tools marked `ReadOnlyHint: true`? Are destructive tools marked appropriately?
- [ ] **Descriptions LLM-friendly**: Do descriptions explain what the tool does and when to use it?
- [ ] **Results use NewToolCallResultStructured**: Not manual `json.Marshal` + `NewToolCallResult`
- [ ] **Container targeting**: If exec-ing into multi-container pods, is the correct container selected?
- [ ] **Snapshot tests added**: Is the toolset in `TestGranularToolsetsTools` with a snapshot file?
- [ ] **All eval configs updated**: `evals/core-eval-testing/<agent>/eval-<suite>.yaml` (one per toolset per agent)
