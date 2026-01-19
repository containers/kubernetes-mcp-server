# Toolset Versioning

This document describes how toolsets are versioned, the rules for toolsets changing versions, and how to configure which 
tools/toolsets should be used through their versions.

## How Toolsets and Tools are Versioned

All tools/prompts and toolsets are versioned as one of "alpha", "beta", or "ga"/"stable". Each toolset has a default version
for the toolset, however individual tools/prompts may have their own versions. For example, a toolset as a whole may be in beta,
however a newly added tool in that toolset may only be in alpha.

The general idea for these versions is:
- "alpha": the toolset is not guaranteed to work well
- "beta": the toolset is not guaranteed to work well, but we are evaluating how well it works
- "stable": the toolset works well, and we are evaluating how well it works to avoid regressions

## Rules for Tool/Prompt/Toolset Versioning

Below are the criteria for the versioning of every tool/prompt/toolset.

### Alpha

All tools/prompts/toolsets begin in "alpha". If you are contributing a new tool/prompt/toolset, this is the version to set.
There are no minimum requirements for something to be considered alpha, apart from the code getting merged.

### Beta

For a tool/prompt/toolset to enter into "beta", we require that there are eval scenarios. For a toolset to enter "beta", there must be scenarios
excercising all of the tools and prompts in the toolset. For individual tools and prompts to enter "beta", we only require an eval scenario
for the specific tool or prompt.

**Note**: for beta we do not require that all the eval scenarios are passing - we just require that they exist.

### GA/Stable

For a tool/prompt/toolset to enter into "stable", we require that 95% or more of the eval scenarios are passing. There is the same requirements as "beta" in terms of the number of evaluation scenarios.

## Configuring tools/toolsets on the server by their version

When configuring the MCP server, you can set a default toolset version to use for all tools with the `default_toolset_version` key. 
Within all the toolsets you enable, only the tools which meet this minimum version will be enabled. For example, if a toolset has 
both "alpha" and "beta" tools and you enable only "beta" tools on the toolset, you will not see any of the "alpha" tools.

You can also enable specific minimum versions for specific toolsets using the "toolset:version" syntax when enabling the toolset.
For example, if you want to allow all the "alpha" tools in the "core" toolset, you could set `toolsets = [ "core:alpha" ]`, and this would
enable all alpha+ tools in the core toolset.

See a full config example below:
```toml
default_toolset_version = "beta"

toolsets = [ "core", "config", "helm:alpha" ]
```
