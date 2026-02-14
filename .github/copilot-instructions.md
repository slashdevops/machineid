# Development Guidelines

This document contains the critical information about working with the project codebase.
Follows these guidelines precisely to ensure consistency and maintainability of the code.

## Stack

- Language: Go (Go 1.22+)
- Framework: Go standard library
- Testing: Go's built-in testing package
- Dependency Management: Go modules
- Version Control: Git
- Documentation: go doc
- Code Review: Pull requests on GitHub
- CI/CD: GitHub Actions
- Logging: `slog` package from the standard library

## Project Structure

Since this is a library build in native go, the files are mostly organized following the standard Go project layout with some additional folders for specific functionalities.

- Library files are located in the root directory.
- examples/ contains example code demonstrating how to use the library.
- .github/ contains GitHub-specific files such as workflows for CI/CD.
- .gitignore specifies files and directories to be ignored by Git.
- .vscode/ contains Visual Studio Code configuration files.
- LICENSE is the license file for the project.
- README.md provides an overview of the project, installation instructions, usage examples, and other relevant information.
- go.mod and go.sum manage the project's dependencies.
- \*.go files contain the main source code of the library.
- \*\_test.go files contain the test cases for the library.

## Code Style

- Follow Go's idiomatic style defined in
  - #fetch https://google.github.io/styleguide/go/guide
  - #fetch https://google.github.io/styleguide/go/decisions
  - #fetch https://google.github.io/styleguide/go/best-practices
  - #fetch https://golang.org/doc/effective_go.html
- Use meaningful names for variables, functions, and packages.
- Keep functions small and focused on a single task.
- Use comments to explain complex logic or decisions.
- Use dependency injection for services and repositories to facilitate testing and maintainability.
- don't use `interface{}` instead use `any` for better readability.

## Documentation

- Use Go's built-in documentation system with `go doc` and comments.
- Document all exported functions, types, and variables with clear and concise comments.
- Use examples in the documentation to illustrate how to use the library effectively.
- Keep documentation up to date with code changes. The package documentation located at `doc.go` should provide an overview of the package and its main functionalities. and the Public documentation at `README.md` should provide an overview of the project, installation instructions, usage examples, and other relevant information.
