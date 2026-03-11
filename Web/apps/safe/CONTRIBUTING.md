# Contributing to Primus-SaFE Web

Thank you for your interest in contributing to Primus-SaFE Web! We welcome
contributions from the community.

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

This project adheres to a code of conduct. By participating, you are expected
to uphold this code. Please report unacceptable behavior to the project
maintainers.

### Our Standards

- Be respectful and inclusive
- Welcome newcomers and help them get started
- Accept constructive criticism gracefully
- Focus on what is best for the community
- Show empathy towards other community members

## Getting Started

### Prerequisites

Before you begin, ensure you have:

- Node.js 18+ (LTS recommended)
- npm 9+ (or pnpm/yarn, if you prefer)
- Git

### Setting Up Development Environment

1. **Fork and clone the repository**
   ```bash
   git clone https://github.com/YOUR_USERNAME/primus-safe-web.git
   cd primus-safe-web
   ```

2. **Set up upstream remote**
   ```bash
   git remote add upstream https://github.com/AMD-AGI/primus-safe-web.git
   ```

3. **Create a development branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

4. **Install dependencies**
   ```bash
   npm install
   ```

5. **Start the dev server**
   ```bash
   npm run dev
   ```

## Development Workflow

### Making Changes

1. **Keep your fork synced**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Make your changes**
   - Follow existing UI/UX patterns and component structure
   - Add or update tests when applicable
   - Update documentation if behavior changes

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
- [ ] All tests and checks pass
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

### Vue and TypeScript

- Use Vue 3 Composition API and `<script setup>` where possible
- Keep components focused and reusable
- Avoid deeply nested components when a shared component makes sense
- Prefer explicit typing for public APIs and complex objects
- Use `const` by default; avoid `any` when possible

### Styling

- Follow existing CSS/SCSS conventions in `src/assets`
- Prefer reusable utility classes where available
- Keep UI changes consistent with existing design system

### File Organization

- Place page-level views in `src/pages`
- Reusable components go in `src/components`
- API services go in `src/services`

## Testing

### Running Lint and Type Checks

```bash
npm run lint
npm run type-check
```

### Building for Production

```bash
npm run build
```

## Documentation

### When to Update Documentation

- New features or modules
- API changes
- Configuration changes
- Installation procedure changes

### Documentation Guidelines

- Use clear, concise language
- Include screenshots for UI changes
- Keep `README.md` up to date

## Community

### Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and discussions
- **Pull Requests**: For code contributions

## License

By contributing to Primus-SaFE Web, you agree that your contributions will be
licensed under the Apache License 2.0.

---

Thank you for contributing to Primus-SaFE Web!
