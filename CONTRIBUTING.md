# Contributing to Kubernetes MCP Server

We'd love to have you join the community! Whether you're reporting issues, improving documentation, fixing bugs, or developing new features, your contributions are essential to our success.

## Code of Conduct

This project follows the [Containers Community Code of Conduct](https://github.com/containers/common/blob/main/CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Contribution Expectations

Maintainer time and attention are limited. Contributions should improve the project enough to justify the effort needed to review and maintain them. Maintainers may decline technically correct reports or changes when their benefit does not justify that effort.

Before opening a series of related issues or pull requests, propose the scope in a single issue or discussion and wait for maintainer agreement. Focus on a small amount of work and follow it through discussion and resolution before expanding your activity. The same expectation applies to reviewing multiple PRs: prioritize actionable feedback and collaboration over volume.

### AI Assistance and Communication

AI tools may help you investigate problems, understand the codebase, write code, and run tests. You are responsible for checking their output and understanding the contributions you submit. Disclose AI assistance in your issue, PR, or review, including how it was used.

Write issue reports, PR descriptions, review comments, and responses to feedback in your own words. Explain the problem, its practical impact, and your reasoning concisely. Code snippets, logs, and other diagnostic output can be included as supporting evidence. Do not have an agent open issues, post reviews, or conduct discussions on your behalf. We want to collaborate with the person making the contribution, rather than evaluate a stream of generated reports or responses.

Maintainers are exempt from these AI-specific requirements because they have established trust within the project and demonstrated judgment in using these tools. They remain accountable for the quality of their contributions and their impact on other contributors and maintainers.

## Reporting Issues

Before reporting an issue, check our backlog of [open issues](https://github.com/containers/kubernetes-mcp-server/issues) to see if someone else has already reported it. If so, feel free to add your scenario or additional information to the discussion. Please use a thumbs-up emoji on the original report rather than adding "+1" comments.

If you find a new bug or have a feature request, we'd love to hear about it! For bug reports, the most important aspect is that they include enough information for us to reproduce the problem. Please include as much detail as possible and try to remove extra details that don't relate to the issue itself. The easier it is for us to reproduce it, the faster it'll be fixed! For feature requests, please describe the use case and why the feature would be valuable.

Please don't include any private or sensitive information in your issue. Security issues should **NOT** be reported via GitHub issues. Please report security vulnerabilities responsibly by following our [security policy](SECURITY.md).

## Contributing Code

For release cadence, versioning, and published artifacts, see [Releases](docs/releases.md).

### Prerequisites

- [Go](https://go.dev/dl/) (version specified in `go.mod`)

### Getting Started

1. Fork the repository on GitHub.
2. Clone your fork locally:
   ```bash
   git clone https://github.com/<your-username>/kubernetes-mcp-server.git
   cd kubernetes-mcp-server
   ```
3. Create a new branch for your changes:
   ```bash
   git checkout -b my-feature
   ```
4. Make your changes and verify them:
   ```bash
   make build
   make test
   ```
5. Commit your changes (see [Commit Messages](#commit-messages)).
6. Push to your fork and [open a Pull Request](#pull-request-guidelines).

### Building

```bash
# Clean, tidy, format, lint, and build the binary
make build
```

The `build` target runs `clean`, `tidy`, `format`, and `lint` before compiling. The resulting executable is `kubernetes-mcp-server`. Run `make help` to see all available Makefile targets.

### Testing

```bash
make test
```

The test suite uses `setup-envtest` from `sigs.k8s.io/controller-runtime`, which provides a lightweight Kubernetes API server and etcd binary -- no real cluster is required. The first run downloads the `envtest` environment, so network access is needed.

When writing tests:

- Use `testify/suite` for organizing tests into suites.
- Test the public API only (black-box testing).
- Use real implementations instead of mocks.
- Use nested subtests with `s.Run()` and descriptive names.
- Aim for one assertion per test case.
- Cover edge cases (nil inputs, empty values, invalid formats).

See [AGENTS.md](AGENTS.md#testing-patterns-and-guidelines) for detailed testing patterns and examples.

### Adding New MCP Tools

The project uses a toolset-based architecture:

1. Define the tool handler function implementing the tool's logic.
2. Create a `ServerTool` struct with the tool definition and handler in `pkg/api/`.
3. Add the tool to an appropriate toolset in `pkg/toolsets/` (or create a new toolset if needed).
4. Register the toolset in `pkg/toolsets/` if it's a new toolset.
5. Run `make update-readme-tools` to update the auto-generated toolset tables.

Toolset ownership, PR labels, and reviewer notifications are defined in
`.github/ownership.json`. When changing ownership or toolset paths, update that
file and run `make update-codeowners` to regenerate `.github/CODEOWNERS`.
Run `make check-codeowners` to validate the JSON and check for drift; CI runs
this target as well.
Use identical path patterns for shared areas. Nested or overlapping patterns
are rejected because CODEOWNERS uses the last matching rule, which could
otherwise hide an area's approvers.

### Dependencies

When introducing new modules, run `make tidy` so that `go.mod` and `go.sum` remain clean.

## Pull Request Guidelines

Small, focused pull requests (PRs) are welcome. Please consider their practical benefit and review cost, and follow the [contribution expectations](#contribution-expectations).

### Concurrent Pull Request Limit

Contributors without write access may have at most **two open PRs** in this repository at a time, including draft PRs. Follow existing PRs through review and resolution before opening more. Maintainers may approve exceptions for trusted contributors or coordinated work; ask before exceeding the limit.

### Preparing a Pull Request

All PRs should be submitted against the `main` branch. Maintainers will take care of backporting if needed.

While bug fixes can first be identified via an issue, that is not required. It's ok to just open up a PR with the fix, but make sure you include the same information you would have included in an issue, like how to reproduce it.

For larger new features, please open an issue or discussion first so the approach can be agreed upon before you invest significant time in the implementation. PRs for new features should include some background on what use cases the new code is trying to address. When possible and when it makes sense, try to break up larger PRs into smaller ones -- it's easier to review smaller code changes. But only if those smaller ones make sense as stand-alone PRs.

All PRs should include:

- **Well-documented code changes.** A commit message should answer *why* a change was made.
- **Tests.** Ideally, they should fail without your code change applied.
- **Documentation updates** if the changes affect user-facing behavior.

### Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/). Each commit message must follow the format:

```
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

Common types include:

- `feat` -- a new feature
- `fix` -- a bug fix
- `docs` -- documentation changes (see `docs/` and `docs/specs/`)
- `test` -- adding or updating tests
- `refactor` -- code changes that neither fix a bug nor add a feature
- `build` -- changes to the build system or dependencies
- `ci` -- changes to CI configuration
- `chore` -- other changes that don't modify source or test files

Additional guidelines:

- Keep the subject line concise (under 72 characters).
- Separate subject from body with a blank line.
- In the body, explain *what* and *why*, not just *how*.
- Solve one problem per commit.
- Reference related issues with `Fixes: #00000` or `Closes: #00000`.

### Sign Your Commits

All commits must include a `Signed-off-by` trailer. This certifies that you wrote the patch or otherwise have the right to pass it on as an open-source patch under the [Developer Certificate of Origin](https://developercertificate.org/):

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

Add the sign-off line to every commit message:

```
Signed-off-by: Your Name <your.email@example.com>
```

If you set your `user.name` and `user.email` git configs, you can sign your commit automatically with `git commit -s`.

### Code Review

Once a PR is submitted, a maintainer will review it. If nobody responds within two weeks, please ping a maintainer. Sometimes PRs are overlooked.

Community reviews are welcome, especially concrete findings that help the author and maintainers assess correctness and practical impact. Avoid unsolicited approval-only summaries or repeated validation reports that add no actionable information. Community feedback does not replace approval from the designated maintainers. See the [contribution ladder](#contribution-ladder) for how reviewers develop an ongoing role in the project.

Keep an eye on the CI results. If something fails, check the logs to see if it's related to your change. If you're unsure, ask in the PR comments.

If changes are requested, amend them into the relevant commit rather than adding extra "fix" commits. Use `git commit --amend` and force-push to your branch. This keeps the git history clean.

## Contribution Ladder

We welcome all community members to become contributors and over time grow into larger roles within the project! The table describes the general
responsibilities expected of individuals in each role. A detailed description of the responsibilities and requirements for each role is provided
below the table.

| **Role**           | **Responsibilities**                                                    |
| ------------------ | ----------------------------------------------------------------------- |
| Contributor        | Follow the Code of Conduct and contribute to the project                |
| Toolset Reviewer   | Review PRs for a specific Toolset                                       |
| Toolset Maintainer | Review and approve PRs and general direction for a specific Toolset     |
| Core Reviewer      | Review PRs for the project as a whole                                   |
| Core Maintainer    | Review and approve PRs and general direction for the project as a whole |

### Building Trust and Taking Responsibility

Roles recognize an established working relationship and demonstrated responsibility; they are not rewards for contribution counts or time served. Contributors provide useful work, reviewers become dependable technical collaborators, and maintainers are entrusted with acceptance decisions and ongoing responsibility. Maintainers hold the approver role within their area of responsibility.

Build trust by working consistently in an area, participating in technical discussions, following contributions through review and resolution, and taking responsibility when problems are found. Coordinate with maintainers before expanding your activity.

Useful contributions reduce the work needed to move the project forward. This includes identifying important problems, improving proposed solutions, helping contributors reach resolution, and recognizing when a change is unnecessary or outside the project's priorities.

Review contributions are a valid path toward greater responsibility; authoring code is not a prerequisite. However, review volume, approval counts, and reports of testing do not establish trust by themselves.

#### Endorsement and Progression

An endorsing or nominating maintainer should have worked directly with the contributor over multiple contributions and discussions, and be able to describe examples of their judgment, reliability, and collaboration.

The experience guidelines below, including the approximate three-month periods, are not promotion deadlines or activity quotas. Meeting a guideline does not automatically qualify someone for a role. Promotion requires maintainer agreement that the contributor is ready to take responsibility for the relevant area.

### Contributor

This role describes people who have just started contributing, or who contribute but don't participate in project governance or
have defined responsibilities.

A Contributor contributes directly to the project and adds value to it. Contributions need not be code.

#### Requirements

Contributors should be familiar with the project and its processes, and should contribute the project in one or more of the following ways:
- Report and sometimes resolve issues
- Submit PRs
- Contribute to documentation
- Submit feedback on issues and PRs
- Test releases and PRs and submit reviews

### Toolset Reviewer

This role describes people who review and give feedback and direction on a specific toolset. These are often contributors to existing toolsets
who have not yet met the criteria to be a Toolset Maintainer, or contributors to a newly added toolset who have not engaged with the community
for long enough to be trusted as a Toolset Maintainer yet.

It is worth noting that anyone can review PRs to a toolset (or to any other part of the project) without being a Toolset Reviewer. The Toolset
Reviewer role is meant to denote people who are a technical authority in the area and who are willing to be pinged to provide reviews within
that area.

#### Requirements

Toolset Reviewers should exhibit strong judgement within the area of the toolset that they are a reviewer for. To become a reviewer, one should:
- Provide consistently useful feedback or contributions within the toolset, demonstrating understanding of its behavior, surrounding code, and project conventions
- Distinguish correctness defects and other blocking concerns from optional improvements, considering practical impact and project priorities
- Participate constructively in technical discussions and follow work through revision and resolution
- Receive endorsement from a Maintainer of the same toolset or from a core Maintainer

#### Responsibilities and Privileges

Toolset Reviewers are expected to:
- Demonstrate sound judgement
- Ensure toolset quality via reviews
- State uncertainties and the limits of their review
- Revise or withdraw findings when further discussion shows they are mistaken
- Help contributors resolve feedback and coordinate with maintainers on acceptance decisions

### Toolset Maintainer

This role describes people who are trusted to approve and merge code within a specific toolset. Beyond just code review, they are expected
to also take a holistic view on the overall quality and direction of the toolset.

#### Requirements

Toolset Maintainers must exhibit sustained contributions and good judgement within the community. Generally speaking, contributors should aim
to be a Toolset Reviewer first. A Toolset Maintainer is expected to:
- Build sustained experience reviewing other people's contributions to the toolset, typically over about 3 months
- Demonstrate judgment on non-trivial contributions, including compatibility, scope, maintenance cost, and alignment with project priorities
- Take responsibility for following up on regressions and helping other contributors
- Be nominated by an existing maintainer of the same toolset, or a Core Maintainer, with no objections from other maintainers

#### Responsibilities and Privileges

Toolset Maintainers are given the privilege of being able to merge PRs within their toolset. As such they are expected to:
- Demonstrate sound judgement
- Ensure toolset quality via reviews
- Mentor new contributors and reviewers within their toolset
- Work to ensure that the toolset is moving in a good direction

### Core Reviewer

This role describes people who review and give feedback and direction across the project as a whole.

It is worth noting that anyone can review PRs without being a Core Reviewer. The Core Reviewer role is meant to denote people who are a
technical authority in the area and who are willing to be pinged to provide reviews within that area.

#### Requirements

Core Reviewers should exhibit technical prowess across the whole project. To become a reviewer, one should:
- Provide consistently useful feedback or contributions across the project, demonstrating understanding of shared infrastructure and interactions between toolsets
- Distinguish correctness defects and other blocking concerns from optional improvements, considering practical impact and project priorities
- Work constructively with toolset owners and follow contributions through revision and resolution
- Receive endorsement from a core Maintainer

#### Responsibilities and Privileges

Core Reviewers are expected to:
- Demonstrate sound judgement
- Ensure project quality via reviews
- State uncertainties and the limits of their review
- Revise or withdraw findings when further discussion shows they are mistaken
- Coordinate with domain owners and seek their input when a change needs expertise outside their own

### Core Maintainer

This role describes people who are trusted to approve and merge code across the whole project. Beyond just code review, they are expected
to also take a holistic view on the overall quality and direction of the project. Where possible, Core Maintainers should defer to Toolset
Maintainers on changes affecting individual toolsets, however Core Maintainers ultimately have the ability to merge code across the whole
project.

#### Requirements

Core Maintainers must exhibit sustained contributions and good judgement within the community. Generally speaking, contributors should aim
to be a Core Reviewer first. A Core Maintainer is expected to:
- Build sustained experience reviewing other people's contributions across the project, typically over about 3 months and beyond a single toolset
- Demonstrate judgment on non-trivial contributions and project-wide consequences, including compatibility, scope, maintenance cost, and project priorities
- Coordinate acceptance decisions with toolset maintainers and take responsibility for following up on regressions and cross-cutting problems
- Be nominated by a Core Maintainer, with no objections from other maintainers

#### Responsibilities and privileges

Core Maintainers are given the privilege of being able to merge PRs across the full project. As such they are expected to:
- Demonstrate sound judgement
- Ensure toolset quality via reviews
- Mentor new contributors and reviewers
- Work to ensure that the toolset is moving in a good direction

## Communication

- [Slack](https://cloud-native.slack.com/archives/C0AHQJVR725) -- `#kubernetes-mcp-server` channel on the CNCF Slack workspace ([request an invitation](https://slack.cncf.io)).
- [GitHub Issues](https://github.com/containers/kubernetes-mcp-server/issues) -- for bugs and feature requests.
- [GitHub Discussions](https://github.com/containers/kubernetes-mcp-server/discussions) -- for questions and general discussion.
- [GitHub Pull Requests](https://github.com/containers/kubernetes-mcp-server/pulls) -- for code contributions.

## Additional Resources

- [AGENTS.md](AGENTS.md) -- detailed project structure, coding style, and testing guidelines.
- [docs/](docs/) -- user-facing documentation.
- [docs/specs/](docs/specs/) -- feature specifications (living documentation for contributors and coding agents).
- [README.md](README.md) -- project overview and setup instructions.
