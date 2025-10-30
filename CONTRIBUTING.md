# Contributing to Primus-SaFE

Thank you for your interest in contributing to Primus-SaFE! We welcome contributions from the community.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Documentation](#documentation)
- [Community](#community)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

### Our Standards

- Be respectful and inclusive
- Welcome newcomers and help them get started
- Accept constructive criticism gracefully
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Prerequisites

Before you begin, ensure you have:

- Go 1.21+ (for SaFE platform components)
- Python 3.8+ (for Bench and Lens components)
- Docker or Podman
- Kubernetes cluster (for testing)
- Git

### Setting Up Development Environment

1. **Fork and clone the repository**
   ```bash
   git clone https://github.com/YOUR_USERNAME/Primus-SaFE.git
   cd Primus-SaFE
   ```

2. **Set up upstream remote**
   ```bash
   git remote add upstream https://github.com/AMD-AGI/Primus-SaFE.git
   ```

3. **Create a development branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Workflow

### Making Changes

1. **Keep your fork synced**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Make your changes**
   - Write clear, commented code
   - Follow existing code style
   - Add tests for new features
   - Update documentation as needed

3. **Commit your changes**
   ```bash
   git add .
   git commit -m "feat: add new feature"
   ```

   **Commit message format:**
   - `feat:` New feature
   - `fix:` Bug fix
   - `docs:` Documentation changes
   - `style:` Code style changes (formatting, etc)
   - `refactor:` Code refactoring
   - `test:` Adding or updating tests
   - `chore:` Maintenance tasks

## Pull Request Process

### Before Submitting

- [ ] Code follows project style guidelines
- [ ] All tests pass
- [ ] Documentation is updated
- [ ] Commit messages are clear and descriptive
- [ ] Branch is up to date with main

### Submitting a Pull Request

1. **Push your changes**
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Create Pull Request**
   - Go to GitHub and create a new Pull Request
   - Fill in the PR template with:
     - Clear description of changes
     - Related issue numbers
     - Testing performed
     - Screenshots (if UI changes)

3. **Review Process**
   - Maintainers will review your PR
   - Address feedback and update as needed
   - Once approved, your PR will be merged

### PR Review Checklist

Reviewers will check:
- [ ] Code quality and style
- [ ] Test coverage
- [ ] Documentation completeness
- [ ] No breaking changes (or properly documented)
- [ ] Performance implications

## Coding Standards

### Go Code (SaFE Platform)

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Run `golint` and address warnings
- Write meaningful variable and function names
- Add comments for exported functions

**Example:**
```go
// ProcessWorkload handles the lifecycle of a training workload
// and returns an error if processing fails.
func ProcessWorkload(ctx context.Context, workload *Workload) error {
    // Implementation
}
```

### Python Code (Bench, Lens exporters)

- Follow [PEP 8](https://www.python.org/dev/peps/pep-0008/)
- Use `black` for formatting
- Use type hints where applicable
- Write docstrings for functions and classes

**Example:**
```python
def validate_node_health(node: str, checks: List[str]) -> Dict[str, bool]:
    """
    Validates the health of a given node.
    
    Args:
        node: The node hostname or IP address
        checks: List of health check names to perform
        
    Returns:
        Dictionary mapping check names to pass/fail status
    """
    # Implementation
```

### Shell Scripts (Bootstrap, installation scripts)

- Use `#!/bin/bash` shebang
- Add error handling (`set -e`)
- Use meaningful variable names (UPPERCASE for globals)
- Add comments for complex logic

## Testing

### Running Tests

**Go components:**
```bash
cd SaFE/job-manager
go test ./...
```

**Python components:**
```bash
cd Bench
pytest tests/
```

### Writing Tests

- Write unit tests for new functions
- Add integration tests for new features
- Ensure tests are deterministic
- Mock external dependencies

## Documentation

### When to Update Documentation

- New features or modules
- API changes
- Configuration changes
- Installation procedure changes

### Documentation Guidelines

- Use clear, concise language
- Include code examples
- Add diagrams for complex concepts
- Keep README.md up to date

### Component Documentation

Each major component should have:
- README.md with overview and quick start
- API documentation (if applicable)
- Configuration reference
- Troubleshooting guide

## Reporting Issues

### Bug Reports

Include:
- Clear description of the bug
- Steps to reproduce
- Expected vs actual behavior
- Environment details (OS, versions, etc)
- Logs or error messages

### Feature Requests

Include:
- Clear description of the feature
- Use cases and benefits
- Potential implementation approach
- Any relevant examples

## Community

### Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and discussions
- **Pull Requests**: For code contributions

### Recognition

Contributors will be recognized in:
- Release notes
- Contributors list
- Project acknowledgments

## License

By contributing to Primus-SaFE, you agree that your contributions will be licensed under the Apache License 2.0.

---

**Thank you for contributing to Primus-SaFE!** 🎉

We appreciate your time and effort in making this project better for everyone.

