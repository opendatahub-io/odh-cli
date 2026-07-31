# Development Guide: odh-cli

This is the main entry point for odh-cli development documentation. Use this guide to navigate to specific topics.

For architectural information and design decisions, see [design.md](design.md).

## Quick Start

New to the project? Start here:

1. **[Setup and Build](setup.md)** - Build commands, test commands, and development environment setup
2. **[Coding Conventions](coding/conventions.md)** - Core coding standards and practices
3. **[Testing Guidelines](testing.md)** - How to write and organize tests
4. **[Quality Verification](quality.md)** - Running linters, tests, and quality checks

## Documentation Structure

### Getting Started
- **[setup.md](setup.md)** - Build commands, test commands, prohibited commands

### Coding Standards
The `coding/` directory contains focused coding guidelines:

- **[coding/conventions.md](coding/conventions.md)** - Core coding conventions
  - Error handling
  - Function signatures
  - Package organization
  - Naming conventions
  - Code comments (WHY not WHAT)
  - Message constants
  - Command interface pattern

- **[coding/patterns.md](coding/patterns.md)** - Architectural patterns and design practices
  - Functional options pattern
  - IOStreams wrapper
  - JQ-based field access
  - Centralized GVK/GVR definitions
  - High-level resource operations
  - Cluster-wide operations

- **[coding/formatting.md](coding/formatting.md)** - Code formatting rules
  - Using `make fmt`
  - Blank imports for auto-registration
  - Import ordering

### Testing
- **[testing.md](testing.md)** - Testing practices and conventions
  - Test framework (Gomega)
  - Test data organization
  - Mock organization
  - Struct assertions
  - Kubernetes sets for deduplication
  - Generic type conversion

### Extensibility
- **[extensibility.md](extensibility.md)** - Adding features and extending the CLI
  - Adding new commands
  - Command-specific logic
  - Adding output formats
  - Using the table renderer

### Quality and Review
- **[quality.md](quality.md)** - Continuous quality verification
  - Development workflow
  - Lint-fix-first approach
  - Quality gates
  - When to run checks

- **[code-review.md](code-review.md)** - Code review guidelines
  - Linter rules and configuration
  - Git commit conventions
  - Task-based commits
  - Pull request checklist
  - Code style summary

### Command-Specific Documentation
- **[lint/architecture.md](lint/architecture.md)** - Lint command architecture and design
- **[lint/writing-checks.md](lint/writing-checks.md)** - Writing diagnostic checks for the lint command
- **[migrate/rhbok-migration.md](migrate/rhbok-migration.md)** - As-built RHBOK Kueue migration (`kueue.rhbok.migrate`)
- **[migrate/upgrade-helpers-comparison.md](migrate/upgrade-helpers-comparison.md)** - Operator guide: upgrade-helpers scripts → migrate CLI

## Common Tasks

### Setting Up Development Environment
See [setup.md](setup.md#build-commands)

### Writing a New Command
See [extensibility.md](extensibility.md#adding-a-new-command)

### Writing Tests
See [testing.md](testing.md#test-framework)

### Running Quality Checks
See [quality.md](quality.md#quality-gates)

### Preparing a Pull Request
See [code-review.md](code-review.md#pull-request-checklist)

## Architecture Documentation

For broader architectural context:
- **[design.md](design.md)** - Overall CLI design and architecture
- **[lint/architecture.md](lint/architecture.md)** - Lint command-specific architecture
- **[migrate/rhbok-migration.md](migrate/rhbok-migration.md)** - RHBOK Kueue migration as-built

## Contributing

Before contributing:
1. Read [coding/conventions.md](coding/conventions.md) and [coding/patterns.md](coding/patterns.md)
2. Understand [testing.md](testing.md) practices
3. Follow the [quality.md](quality.md) workflow
4. Review [code-review.md](code-review.md) guidelines
