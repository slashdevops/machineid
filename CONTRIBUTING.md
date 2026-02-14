# Contributing to machineid

Thank you for your interest in contributing to machineid! This document provides guidelines and best practices for contributing to the project.

## Development Setup

### Prerequisites

- Go 1.22 or higher
- Git
- Make

### Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/machineid.git
   cd machineid
   ```

3. Add the upstream repository:
   ```bash
   git remote add upstream https://github.com/slashdevops/machineid.git
   ```

4. Create a new branch for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Workflow

### Building

Build the project using the provided Makefile:

```bash
make build
```

### Testing

Run the test suite:

```bash
make test
```

Run tests with coverage:

```bash
make test-coverage
```

### Linting

Run the linter to ensure code quality:

```bash
make lint
```

## Versioning and Releases

This project follows [Semantic Versioning 2.0.0](https://semver.org/). Version numbers follow the format `MAJOR.MINOR.PATCH`:

- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible functionality additions
- **PATCH**: Backwards-compatible bug fixes

### Creating Tags

**Important**: Tags must follow semantic versioning order. You cannot create a tag with an older version number than existing tags.

#### Example Error

If you try to create `v0.0.1` when `v0.0.2` already exists, you'll get:

```
! [remote rejected] v0.0.1 -> v0.0.1 (push declined due to repository rule violations)
error: failed to push some refs to 'github.com:slashdevops/machineid.git'
```

This is a GitHub repository rule that prevents version rollback and ensures proper version ordering.

#### Correct Versioning Workflow

1. Check existing tags:
   ```bash
   git tag -l
   ```

2. Create the next appropriate version tag:
   ```bash
   # If the latest tag is v0.0.2, create v0.0.3 or higher
   git tag -a "v0.0.3" -m "Release v0.0.3"
   ```

3. Push the tag:
   ```bash
   git push origin v0.0.3
   ```

#### Version Increment Guidelines

Choose the appropriate version increment based on your changes:

- **Patch** (e.g., `v0.0.2` → `v0.0.3`): Bug fixes, documentation updates
- **Minor** (e.g., `v0.0.2` → `v0.1.0`): New features, backwards-compatible changes
- **Major** (e.g., `v0.0.2` → `v1.0.0`): Breaking changes, incompatible API modifications

### Release Process

1. Ensure all tests pass:
   ```bash
   make test
   ```

2. Update documentation if needed

3. Create and push a tag with the next appropriate version:
   ```bash
   git tag -a "vX.Y.Z" -m "Release vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. The GitHub Actions workflow will automatically create a release with binaries

## Code Style

Follow Go's idiomatic style as defined in:
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)

### Key Guidelines

- Use meaningful names for variables, functions, and packages
- Keep functions small and focused on a single task
- Use comments to explain complex logic or decisions
- Prefer `any` over `interface{}` for better readability
- Use dependency injection for services to facilitate testing

## Pull Request Process

1. Update documentation for any new features or changes
2. Ensure all tests pass
3. Ensure code passes linting
4. Update the README.md if needed
5. Submit a pull request with a clear description of changes

### Pull Request Title

Use conventional commit format:

- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation changes
- `test:` for test-related changes
- `chore:` for maintenance tasks

Example: `feat: add support for custom MAC address filters`

## Questions?

If you have questions or need help, please:
- Open an issue on GitHub
- Check existing issues for similar questions
- Review the README.md for usage examples

Thank you for contributing to machineid!
