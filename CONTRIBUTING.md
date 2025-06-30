# 🤝 Contributing to Kubeflare Light

Thank you for your interest in contributing to kubeflare-light! This document provides guidelines and information for contributors.

## 📋 Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Contribution Types](#contribution-types)
- [Pull Request Process](#pull-request-process)
- [Testing Guidelines](#testing-guidelines)
- [Documentation](#documentation)
- [Community](#community)

---

## 📜 Code of Conduct

### Our Pledge

We pledge to make participation in our project a harassment-free experience for everyone, regardless of age, body size, disability, ethnicity, gender identity and expression, level of experience, nationality, personal appearance, race, religion, or sexual identity and orientation.

### Our Standards

**Positive behavior includes:**
- Using welcoming and inclusive language
- Being respectful of differing viewpoints and experiences
- Gracefully accepting constructive criticism
- Focusing on what is best for the community
- Showing empathy towards other community members

**Unacceptable behavior includes:**
- Harassment of any kind
- Discriminatory language or actions
- Personal attacks or trolling
- Publishing others' private information without permission
- Any conduct that could reasonably be considered inappropriate

### Enforcement

Project maintainers are responsible for clarifying standards and may take corrective action in response to inappropriate behavior. Contact the project team at [security@kubeflare-light.io] for reports.

---

## 🚀 Getting Started

### Prerequisites

- **Go 1.24+**
- **Docker**
- **kubectl**
- **Kind** (for local testing)
- **Git**
- **Basic Kubernetes knowledge**
- **Cloudflare account** (for testing)

### Development Setup

1. **Fork and clone:**
```bash
# Fork the repository on GitHub first
git clone https://github.com/YOUR-USERNAME/kubeflare-light.git
cd kubeflare-light

# Add upstream remote
git remote add upstream https://github.com/wzygadlo/kubeflare-light.git
```

2. **Install dependencies:**
```bash
# Install Go dependencies
go mod download

# Install development tools
make dev-tools

# Verify setup
make verify
```

3. **Create development environment:**
```bash
# Create Kind cluster
make kind-cluster

# Set up test environment
export CLOUDFLARE_API_TOKEN="your-test-token"
export DEFAULT_ZONE_ID="your-test-zone"
```

4. **Verify everything works:**
```bash
# Run tests
make test

# Build project
make build

# Deploy to Kind cluster
make deploy-kind
```

---

## 🔄 Development Workflow

### Branching Strategy

We use a simplified Git flow:

- **`main`**: Production-ready code
- **`develop`**: Integration branch for features
- **`feature/*`**: Feature development branches
- **`fix/*`**: Bug fix branches
- **`docs/*`**: Documentation updates

### Branch Naming

```bash
# Features
feature/add-metrics-support
feature/improve-error-handling

# Bug fixes
fix/expression-injection-vulnerability
fix/memory-leak-in-controller

# Documentation
docs/update-api-reference
docs/add-troubleshooting-guide

# Refactoring
refactor/extract-validation-logic
refactor/improve-test-coverage
```

### Workflow Steps

1. **Sync with upstream:**
```bash
git checkout main
git pull upstream main
git push origin main
```

2. **Create feature branch:**
```bash
git checkout -b feature/your-feature-name
```

3. **Develop and test:**
```bash
# Make changes
git add .
git commit -m "feat: add new feature"

# Run tests
make test
make lint
```

4. **Keep branch updated:**
```bash
git fetch upstream
git rebase upstream/main
```

5. **Push and create PR:**
```bash
git push origin feature/your-feature-name
# Create PR on GitHub
```

---

## 🎯 Contribution Types

### 🐛 Bug Reports

**Before submitting:**
- Search existing issues
- Verify on latest version
- Gather reproduction steps

**Issue template:**
```markdown
## Bug Description
Brief description of the issue

## Reproduction Steps
1. Step one
2. Step two
3. Expected vs actual behavior

## Environment
- kubeflare-light version:
- Kubernetes version:
- Go version:
- OS/Platform:

## Logs
```
Relevant logs here
```

## Additional Context
Any other relevant information
```

### ✨ Feature Requests

**Before submitting:**
- Check roadmap and existing issues
- Consider if it fits project scope
- Think about implementation approach

**Feature template:**
```markdown
## Feature Description
What feature would you like to see?

## Use Case
Why do you need this feature?

## Proposed Solution
How would you like it to work?

## Alternatives Considered
Other approaches you've considered

## Additional Context
Any other relevant information
```

### 🔧 Code Contributions

**Types of contributions:**
- **Bug fixes**: Address reported issues
- **Features**: Add new functionality
- **Performance**: Optimization improvements
- **Refactoring**: Code quality improvements
- **Tests**: Improve test coverage
- **Documentation**: Update or add docs

### 📚 Documentation

**Documentation needs:**
- API documentation
- User guides and tutorials
- Developer documentation
- Code comments and examples
- README improvements

---

## 🔀 Pull Request Process

### Before Creating PR

1. **Code quality:**
```bash
# Format code
make fmt

# Run linters
make lint

# Run tests
make test

# Check security
make security-scan
```

2. **Documentation:**
- Update relevant documentation
- Add code comments
- Update CHANGELOG.md if needed

3. **Testing:**
- Add unit tests for new features
- Add integration tests if needed
- Verify all tests pass

### PR Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix (non-breaking change)
- [ ] New feature (non-breaking change)
- [ ] Breaking change (would cause existing functionality to change)
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing performed

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests added and passing
- [ ] No breaking changes (or properly documented)

## Related Issues
Fixes #(issue_number)
```

### Review Process

1. **Automated checks:** All CI checks must pass
2. **Code review:** At least one maintainer approval required
3. **Testing:** Manual testing for significant changes
4. **Documentation:** Docs reviewed for accuracy

### Merge Requirements

- ✅ All CI checks passing
- ✅ At least one maintainer approval
- ✅ No merge conflicts
- ✅ Up-to-date with main branch
- ✅ Conventional commit messages

---

## 🧪 Testing Guidelines

### Test Structure

```
test/
├── unit/           # Unit tests
├── integration/    # Integration tests
├── e2e/           # End-to-end tests
├── fixtures/      # Test data
└── helpers/       # Test utilities
```

### Writing Tests

#### Unit Tests
```go
func TestRateLimitValidation(t *testing.T) {
    tests := []struct {
        name        string
        rateLimit   *RateLimit
        expectError bool
        errorMsg    string
    }{
        {
            name: "valid rate limit",
            rateLimit: &RateLimit{
                Threshold: 100,
                Period:    60,
            },
            expectError: false,
        },
        {
            name: "invalid threshold",
            rateLimit: &RateLimit{
                Threshold: -1,
                Period:    60,
            },
            expectError: true,
            errorMsg:    "threshold must be positive",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateRateLimit(tt.rateLimit)
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorMsg)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### Integration Tests
```go
func TestRateLimitController_Integration(t *testing.T) {
    // Setup test environment
    ctx := context.Background()
    client := fake.NewClientBuilder().Build()
    
    // Create test rate limit
    rateLimit := &cloudflareV1alpha1.RateLimit{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-rate-limit",
            Namespace: "default",
        },
        Spec: cloudflareV1alpha1.RateLimitSpec{
            ZoneID:    "test-zone",
            Threshold: 100,
            Period:    60,
        },
    }
    
    // Test controller reconciliation
    reconciler := &RateLimitReconciler{
        Client: client,
        CF:     &MockCloudflareClient{},
    }
    
    result, err := reconciler.Reconcile(ctx, ctrl.Request{
        NamespacedName: types.NamespacedName{
            Name:      rateLimit.Name,
            Namespace: rateLimit.Namespace,
        },
    })
    
    assert.NoError(t, err)
    assert.Equal(t, ctrl.Result{RequeueAfter: time.Minute * 10}, result)
}
```

### Test Commands

```bash
# Run all tests
make test

# Run specific test package
go test ./pkg/controller/...

# Run with coverage
make test-coverage

# Run integration tests
make test-integration

# Run e2e tests
make test-e2e

# Run tests with race detection
go test -race ./...
```

### Mock Guidelines

Use interfaces for external dependencies:

```go
// Define interface
type CloudflareClientInterface interface {
    CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimitResponse, error)
    UpdateRateLimit(ctx context.Context, zoneID, ruleID string, rateLimit RateLimit) (*RateLimitResponse, error)
    DeleteRateLimit(ctx context.Context, zoneID, ruleID string) error
}

// Mock implementation
type MockCloudflareClient struct {
    CreateRateLimitFunc func(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimitResponse, error)
    // ... other methods
}

func (m *MockCloudflareClient) CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimitResponse, error) {
    if m.CreateRateLimitFunc != nil {
        return m.CreateRateLimitFunc(ctx, zoneID, rateLimit)
    }
    return &RateLimitResponse{ID: "mock-id"}, nil
}
```

---

## 📝 Documentation

### Documentation Types

1. **Code Documentation:**
   - Godoc comments for public APIs
   - Inline comments for complex logic
   - README files in packages

2. **User Documentation:**
   - API reference
   - User guides and tutorials
   - Configuration examples

3. **Developer Documentation:**
   - Architecture documentation
   - Development setup guides
   - Testing guidelines

### Documentation Standards

#### Go Code Documentation
```go
// Package controller implements Kubernetes controllers for managing
// Cloudflare resources through Custom Resource Definitions.
package controller

// RateLimitReconciler reconciles RateLimit objects by synchronizing
// them with Cloudflare's rate limiting API.
//
// The reconciler handles:
//   - Creating rate limit rules in Cloudflare
//   - Updating existing rules when CRD changes
//   - Cleaning up rules when CRD is deleted
//   - Maintaining status conditions
type RateLimitReconciler struct {
    client.Client
    Log    logr.Logger
    Scheme *runtime.Scheme
    CF     CloudflareClientInterface
}

// Reconcile handles the reconciliation loop for RateLimit resources.
// It ensures the desired state defined in the CRD matches the actual
// state in Cloudflare.
//
// Returns:
//   - ctrl.Result: Requeue configuration
//   - error: Any error that occurred during reconciliation
func (r *RateLimitReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Implementation
}
```

#### Markdown Documentation
- Use clear headers and structure
- Include code examples
- Add tables for reference information
- Use badges for status indicators
- Include diagrams where helpful

### Documentation Commands

```bash
# Generate API docs
make docs-api

# Generate godoc
godoc -http=:6060

# Lint documentation
make docs-lint

# Serve docs locally
make docs-serve
```

---

## 🌟 Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Pull Requests**: Code review and collaboration
- **Documentation**: Always up-to-date guides

### Getting Help

1. **Documentation**: Check docs first
2. **Search Issues**: Look for existing solutions
3. **Create Issue**: Detailed problem description
4. **Discussion**: For general questions

### Recognition

Contributors are recognized through:
- **Contributors list** in README
- **Release notes** for significant contributions
- **Maintainer status** for long-term contributors

---

## 🏆 Maintainer Guidelines

### For Project Maintainers

#### Code Review Standards
- **Security**: Check for vulnerabilities
- **Performance**: Consider performance impact
- **Testing**: Ensure adequate test coverage
- **Documentation**: Verify docs are updated
- **API**: Check for breaking changes

#### Merge Criteria
- All automated checks pass
- At least one maintainer approval
- No unresolved review comments
- Documentation updated
- Tests added/updated

#### Release Process
1. Update version numbers
2. Generate changelog
3. Create release notes
4. Tag release
5. Build and publish artifacts

### Becoming a Maintainer

Maintainer status is earned through:
- **Consistent contributions** over time
- **High-quality code** and reviews
- **Community involvement** and support
- **Security awareness** and practices
- **Documentation contributions**

---

## ❓ FAQ

### Common Questions

**Q: How do I set up a development environment?**
A: Follow the [Development Setup](#development-setup) section above.

**Q: What should I work on?**
A: Check the [roadmap](./docs/README.md#roadmap) and [good first issues](https://github.com/wzygadlo/kubeflare-light/labels/good%20first%20issue).

**Q: How do I run tests locally?**
A: Run `make test` for unit tests, `make test-integration` for integration tests.

**Q: What's the code review process?**
A: See [Pull Request Process](#pull-request-process) for detailed information.

**Q: How do I report security vulnerabilities?**
A: See our [Security Guide](./docs/security.md) for responsible disclosure.

### Getting Unstuck

If you're stuck:
1. Check the documentation
2. Search existing issues
3. Ask in GitHub Discussions
4. Create a detailed issue

---

Thank you for contributing to kubeflare-light! Your contributions make this project better for everyone. 🙏