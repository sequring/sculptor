# Contributing to Chameleon 🦎

First off, thank you for considering contributing to Chameleon! We welcome contributions from the community to make Chameleon even better. Whether it's bug reports, feature requests, documentation improvements, or code contributions, your help is appreciated.

## Table of Contents

*   [Code of Conduct](#code-of-conduct)
*   [How Can I Contribute?](#how-can-i-contribute)
    *   [Reporting Bugs](#reporting-bugs)
    *   [Suggesting Enhancements](#suggesting-enhancements)
    *   [Your First Code Contribution](#your-first-code-contribution)
    *   [Pull Requests](#pull-requests)
*   [Development Setup](#development-setup)
*   [Styleguides](#styleguides)
    *   [Go Code](#go-code)
    *   [Git Commit Messages](#git-commit-messages)
*   [Testing](#testing)
*   [Community](#community)

## Code of Conduct

This project and everyone participating in it is governed by the [Chameleon Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to chameleon@sequre42.com .

## How Can I Contribute?

### Reporting Bugs

If you find a bug, please ensure the bug was not already reported by searching on GitHub under [Issues](https://github.com/sequring/chameleon/issues).

If you're unable to find an open issue addressing the problem, [open a new one](https://github.com/sequring/chameleon/issues/new). Be sure to include:

*   A clear and descriptive title.
*   The version of Chameleon you are using (commit hash or release tag).
*   A detailed description of the issue.
*   Steps to reproduce the bug.
*   What you expected to happen.
*   What actually happened (include error messages, logs, or screenshots if applicable).
*   Your operating system and Go version.

### Suggesting Enhancements

If you have an idea for a new feature or an improvement to an existing one:

1.  Check the [Issues](https://github.com/sequring/chameleon/issues) to see if there's a similar suggestion already.
2.  If not, [open a new issue](https://github.com/sequring/chameleon/issues/new) to discuss your idea. Provide as much context and detail as possible.
    *   Explain the problem you're trying to solve or the use case for the enhancement.
    *   Describe your proposed solution.

### Your First Code Contribution

Unsure where to begin contributing to Chameleon? You can start by looking through `good first issue` and `help wanted` issues:

*   [Good first issues](https://github.com/sequring/chameleon/labels/good%20first%20issue) - issues which should only require a few lines of code, and a test or two.
*   [Help wanted issues](https://github.com/sequring/chameleon/labels/help%20wanted) - issues which should be a bit more involved than `good first issue` issues.

### Pull Requests

When you're ready to contribute code:

1.  **Fork the repository** on GitHub.
2.  **Clone your fork** locally: `git clone git@github.com:YOUR_USERNAME/chameleon.git`
3.  **Create a new branch** for your changes: `git checkout -b name-of-your-feature-or-fix`
4.  **Make your changes.** Adhere to the [Styleguides](#styleguides).
5.  **Add tests** for your changes. See [Testing](#testing).
6.  **Ensure all tests pass.**
7.  **Commit your changes** with a descriptive commit message (see [Git Commit Messages](#git-commit-messages)).
8.  **Push your branch** to your fork: `git push origin name-of-your-feature-or-fix`
9.  **Open a Pull Request** against the `main` (or `develop`) branch of `sequring/chameleon`.
    *   Provide a clear title and description for your PR.
    *   Link to any relevant issues (e.g., "Closes #123").
    *   Be prepared to discuss your changes and make adjustments if requested by maintainers.

## Development Setup

1.  Go version [e.g., 1.21] or higher.
2.  Clone the repository.
3.  Dependencies are managed by Go modules. Run `go mod tidy` if needed.
4.  To run the application locally: `go run ./main.go -config config.yml` (assuming you have a `config.yml`).

## Styleguides

### Go Code

*   Follow standard Go formatting (`gofmt` or `goimports`). Most editors do this automatically.
*   Write clear, understandable code with comments where necessary.
*   Follow Go best practices (e.g., effective error handling).
*   Try to adhere to the existing code style within the project.

### Git Commit Messages

*   Use the present tense ("Add feature" not "Added feature").
*   Use the imperative mood ("Move cursor to..." not "Moves cursor to...").
*   Limit the first line to 72 characters or less.
*   Reference issues and pull requests liberally after the first line.
*   Consider using [Conventional Commits](https://www.conventionalcommits.org/) for more structured commit messages, e.g.:
    *   `feat: Add new proxy routing strategy`
    *   `fix: Correct handling of empty user tags`
    *   `docs: Update README with new API endpoint`
    *   `refactor: Simplify health check logic`

## Testing

*   Please write unit tests for new functionality or bug fixes.
*   Place tests in `_test.go` files in the same package as the code they are testing.
*   Run tests with: `go test ./...`
*   Ensure your changes don't break existing tests.


We look forward to your contributions!