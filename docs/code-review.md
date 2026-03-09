# Code Review Guidelines

This document covers code review practices, linter rules, commit conventions, and pull request requirements.

For quality verification workflow, see [quality.md](quality.md). For coding conventions, see [coding/conventions.md](coding/conventions.md).

## Linter Rules

**CRITICAL: MUST use `make lint` to run linter. NEVER use `golangci-lint` directly.**

The project uses golangci-lint v2 with a comprehensive configuration (`.golangci.yml`) that enables all linters by default with specific exclusions.

**Correct usage:**
```bash
# ✓ Check for linting issues
make lint

# ✓ Auto-fix issues where possible (ALWAYS try this FIRST)
make lint/fix
```

**Prohibited:**
```bash
# ❌ NEVER do this - use make lint instead
golangci-lint run

# ❌ NEVER do this - use make lint/fix instead
golangci-lint run --fix
```

**Configuration Highlights:**

* **Enabled**: All linters except those explicitly disabled
* **Disabled linters**: wsl, varnamelen, exhaustruct, ireturn, depguard, err113, paralleltest, funcorder, noinlineerr
* **Test file exclusions**: Many strict linters are disabled for `*_test.go` files to allow for more flexible test code
* **Import ordering**: Uses `gci` formatter to organize imports in sections (standard, default, k8s.io, project, dot)
* **Revive rules**: Enable most revive rules with sensible exclusions for package comments, line length, function length, etc.

**Key Rules:**

* **goconst**: Extract repeated string literals to constants
* **gosec**: No hardcoded secrets (use `//nolint:gosec` only for test data with comment explaining why)
* **staticcheck**: Follow all suggestions
* **Comment formatting**: All comments should be complete sentences ending with periods
* **Error wrapping**: Use `fmt.Errorf` with `%w` for error chains
* **Complexity limits**: cyclop (max 15), gocognit (min 50)

**Running the Linter:**

**CRITICAL: ALWAYS use make commands. NEVER invoke tools directly.**

```bash
# Check for issues (NEVER use golangci-lint directly)
make lint

# Auto-fix issues where possible (ALWAYS try this FIRST)
make lint/fix

# Run all checks
make check
```

**Why you MUST use make instead of golangci-lint directly:**
- Ensures correct configuration and flags are applied
- Prevents accidental damage to critical files (blank imports)
- Maintains consistency across all developers
- Makefile may include additional safety checks or pre-processing

## Git Commit Conventions

**Commit Message Format:**
```
<type>: <subject>

<body>

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

**Types:**
* `feat`: New feature
* `fix`: Bug fix
* `refactor`: Code refactoring (no functional changes)
* `test`: Adding or updating tests
* `docs`: Documentation changes
* `build`: Build system or dependency changes
* `chore`: Maintenance tasks

**Subject:**
* Use imperative mood ("add feature" not "added feature")
* Don't capitalize first letter
* No period at the end
* Max 72 characters

**Body:**
* Explain what and why (not how)
* Separate from subject with blank line
* Wrap at 72 characters
* Use bullet points for multiple items

**Example:**
```
feat: add pod health check to doctor command

This commit adds a new check that verifies pod readiness status:

- Check all pods in the ODH namespace
- Report WARNING if any pods are not ready
- Report ERROR if pods cannot be listed

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

**Task-Based Commits:**

When implementing tasks from `specs/*/tasks.md`, commit granularity follows task boundaries:

- **One commit per task**: Each task gets exactly one commit
- **Task ID in subject**: Use format `T###: <description>` where ### is the task number
- **Grouped tasks**: Multiple related tasks can be `T###, T###: <description>`

**Task Commit Examples:**
```
T001: implement Check interface for serverless removal validation

Adds the Check interface implementation that validates serverless
components are removed when upgrading to 3.x.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

```
T005, T006: add tests for serverless check and version detection

Groups two related testing tasks into a single commit.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
```

## Pull Request Checklist

Before submitting a PR:
* [ ] All tests pass (`make test`)
* [ ] All checks pass (`make check` - includes lint)
* [ ] Code formatted (`make fmt`)
* [ ] Dependencies tidied (`make tidy`)
* [ ] New tests added for new features
* [ ] Documentation updated (design.md, development.md, or AGENTS.md as needed)
* [ ] All test data extracted to package-level constants
* [ ] Error handling follows conventions
* [ ] Functional options pattern used for configuration
* [ ] No linter warnings or errors

## Code Style

* **Function signatures**: Each parameter must have its own type declaration (never group parameters with same type)
* **Comments**: Explain *why*, not *what*. Focus on non-obvious behavior, edge cases, and relationships
* **Error wrapping**: Always use `fmt.Errorf` with `%w` for error chains
* **Context propagation**: Pass context through all layers for cancellation support
* **Zero values**: Leverage zero value semantics instead of pointers where appropriate
* **Early returns**: Use early returns to reduce nesting and improve readability
