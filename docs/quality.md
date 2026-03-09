# Continuous Quality Verification

This document covers quality verification practices and the development workflow for odh-cli.

For setup and build commands, see [setup.md](setup.md). For code review guidelines, see [code-review.md](code-review.md).

**Critical Requirement:** MUST run `make check` after EVERY implementation. This is NOT optional.

Quality verification is a mandatory part of the development workflow, not a pre-commit step. All code changes must pass quality gates before being considered complete.

## Development Workflow

1. **Make code changes**
2. **Run `make lint-fix`** - Auto-fix formatting and simple issues
3. **Run `make lint`** - Check for remaining linting issues
4. **Manual fixes** - Address issues that can't be auto-fixed
5. **Run `make check`** - Complete quality verification (lint + tests)

**make check includes:**
- `make lint` - golangci-lint with all enabled linters
- `make test` - All unit and integration tests

## Lint-Fix-First

**CRITICAL: ALWAYS use `make lint/fix` as first effort to fix linting issues.**

**Never use tools directly:**
- ❌ `golangci-lint run --fix` - Use `make lint/fix` instead
- ❌ `gci write` - Use `make fmt` instead
- ❌ `gofmt -w` - Use `make fmt` instead

**Always run auto-fix before manual fixes:**

```bash
# ✓ CORRECT workflow
make lint/fix    # Auto-fix first (NEVER use golangci-lint directly)
make lint        # Check what remains (NEVER use golangci-lint directly)
# manually fix remaining issues
make check       # Final verification

# ❌ WRONG workflow - DO NOT DO THIS
golangci-lint run           # Wrong: use make lint
gci write ./...             # Wrong: use make fmt
# manually fix all issues without trying auto-fix
make check
```

**Rationale:**
- `make lint/fix` automatically resolves 80%+ of common issues (formatting, imports, simple patterns)
- Manual fixes should only address issues that require human judgment
- Using make ensures consistent tool configuration across all developers
- Direct tool invocation may break critical files (e.g., blank imports in cmd/lint/lint.go)

## Quality Gates

All of these MUST pass before code is considered complete:

**Linting:**
```bash
make lint
```

**Tests:**
```bash
make test
```

**Complete Check (all of the above):**
```bash
make check
```

## When to Run

- After **every** implementation (function, method, test)
- Before **every** commit
- After resolving merge conflicts
- When resuming work on a branch

**NOT optional.** Quality verification is part of implementation, not a separate step.
