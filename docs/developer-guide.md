# 🔧 Developer Guide

This guide provides comprehensive information for developers working on kubeflare-light, including architecture details, development setup, and contribution guidelines.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Development Setup](#development-setup)
- [Code Structure](#code-structure)
- [Adding New Features](#adding-new-features)
- [Testing](#testing)
- [Debugging](#debugging)
- [Contributing](#contributing)

---

## Architecture Overview

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                       │
│                                                             │
│  ┌─────────────────┐    ┌──────────────────────────────┐   │
│  │   kubectl       │    │        kubeflare-light       │   │
│  │   apply         │───▶│         Controller           │   │
│  │   CRDs          │    │                              │   │
│  └─────────────────┘    │  ┌────────────────────────┐  │   │
│                         │  │    RateLimit           │  │   │
│  ┌─────────────────┐    │  │    Controller          │  │   │
│  │   Custom        │◀───┤  │                        │  │   │
│  │   Resources     │    │  └────────────────────────┘  │   │
│  │   (CRDs)        │    │                              │   │
│  └─────────────────┘    │  ┌────────────────────────┐  │   │
│                         │  │    WAFRule             │  │   │
│  ┌─────────────────┐    │  │    Controller          │  │   │
│  │   ConfigMap     │───▶│  │                        │  │   │
│  │   (Zone Config) │    │  └────────────────────────┘  │   │
│  └─────────────────┘    │                              │   │
│                         │  ┌────────────────────────┐  │   │
│                         │  │    Zone Resolver       │  │   │
│                         │  │    Utility             │  │   │
│                         │  └────────────────────────┘  │   │
│                         └──────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                                     │
                                     │ Cloudflare Go SDK v4.5.1
                                     │ (Rulesets API)
                                     ▼
┌─────────────────────────────────────────────────────────────┐
│                     Cloudflare API                          │
│  ┌──────────────────┐      ┌──────────────────────────┐    │
│  │   Rate Limiting  │      │      WAF Rules           │    │
│  │   (Rulesets API) │      │   (Custom Rulesets)      │    │
│  └──────────────────┘      └──────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Key Components

1. **Controllers**: Kubernetes controllers that watch for CRD changes
2. **Cloudflare Client**: Wrapper around the official Cloudflare Go SDK
3. **Zone Resolver**: Utility for flexible zone ID resolution
4. **CRDs**: Custom Resource Definitions for RateLimit and WAFRule
5. **Helm Chart**: Deployment and configuration management

### Controller Pattern

kubeflare-light follows the standard Kubernetes controller pattern:

```go
// Reconciliation Loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the resource
    // 2. Handle deletion (finalizers)
    // 3. Validate and process
    // 4. Sync with Cloudflare
    // 5. Update status
    // 6. Requeue if needed
}
```

---

## Development Setup

### Prerequisites

- **Go 1.24+**
- **Docker**
- **kubectl**
- **Kind** (for local testing)
- **Helm 3.0+**
- **Cloudflare API token**

### Local Environment Setup

1. **Clone and setup:**
```bash
git clone https://github.com/wzygadlo/kubeflare-light.git
cd kubeflare-light

# Install dependencies
go mod download

# Install development tools
make dev-tools
```

2. **Environment configuration:**
```bash
# Copy example configuration
cp .env.example .env

# Edit with your settings
export CLOUDFLARE_API_TOKEN="your-token-here"
export DEFAULT_ZONE_ID="your-zone-id"
```

3. **Create Kind cluster:**
```bash
# Create cluster with registry
make kind-cluster

# Verify cluster
kubectl cluster-info --context kind-kubeflare-dev
```

### Build and Test Locally

```bash
# Build binary
make build

# Run tests
make test

# Generate manifests
make manifests

# Build Docker image
make docker-build

# Load image to Kind
make kind-load

# Deploy to Kind cluster
make deploy-kind
```

---

## Code Structure

### Directory Layout

```
kubeflare-light/
├── cmd/kubeflare/           # Main application entry point
├── pkg/
│   ├── apis/cloudflare/v1alpha1/  # CRD type definitions
│   ├── cloudflare/client/   # Cloudflare API client
│   ├── controller/          # Kubernetes controllers
│   └── utils/               # Shared utilities
├── config/                  # Kubernetes manifests
├── chart/                   # Helm chart
├── deploy/                  # Direct deployment manifests
├── examples/                # Example resources
├── docs/                    # Documentation
└── hack/                    # Development scripts
```

### Key Files

#### Main Application
- `cmd/kubeflare/main.go`: Application entry point
- `pkg/controller/`: Controller implementations

#### API Types
- `pkg/apis/cloudflare/v1alpha1/ratelimit_types.go`: RateLimit CRD
- `pkg/apis/cloudflare/v1alpha1/wafrule_types.go`: WAFRule CRD

#### Cloudflare Integration
- `pkg/cloudflare/client/client.go`: Cloudflare API wrapper

#### Utilities
- `pkg/utils/zone.go`: Zone ID resolution logic

### Code Conventions

#### Go Style

Follow standard Go conventions:
- Use `gofmt` for formatting
- Follow effective Go guidelines
- Use meaningful variable names
- Add godoc comments for public functions

#### Error Handling

```go
// Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to create rate limit: %w", err)
}

// Use structured logging
log.Error(err, "Failed to reconcile rate limit", 
    "ratelimit", rateLimit.Name,
    "namespace", rateLimit.Namespace)
```

#### Controller Patterns

```go
// Use finalizers for cleanup
const rateLimitFinalizer = "cloudflare.io/ratelimit-finalizer"

// Update status with conditions
r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Created", "Rate limit created successfully")

// Proper requeue handling
return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
```

---

## Adding New Features

### Adding a New CRD Field

1. **Update type definitions:**
```go
// pkg/apis/cloudflare/v1alpha1/ratelimit_types.go
type RateLimitSpec struct {
    // ... existing fields
    NewField string `json:"newField,omitempty"`
}
```

2. **Regenerate code:**
```bash
make generate
make manifests
```

3. **Update controller logic:**
```go
// pkg/controller/ratelimit_controller.go
func (r *RateLimitReconciler) convertToCloudflareRateLimit(rateLimit *cloudflareV1alpha1.RateLimit) cfclient.RateLimit {
    cfRateLimit := cfclient.RateLimit{
        // ... existing fields
        NewField: rateLimit.Spec.NewField,
    }
    return cfRateLimit
}
```

4. **Update validation:**
```go
// Add validation in webhook or controller
if rateLimit.Spec.NewField == "" {
    return fmt.Errorf("newField is required")
}
```

### Adding a New Controller

1. **Create controller file:**
```go
// pkg/controller/newresource_controller.go
package controller

import (
    // imports
)

type NewResourceReconciler struct {
    client.Client
    Log      logr.Logger
    Scheme   *runtime.Scheme
    CF       *cfclient.CloudflareClient
}

func (r *NewResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Implementation
}

func SetupNewResourceController(mgr ctrl.Manager) error {
    // Setup logic
}
```

2. **Register controller:**
```go
// cmd/kubeflare/main.go
if err = controller.SetupNewResourceController(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "NewResource")
    os.Exit(1)
}
```

3. **Add RBAC permissions:**
```go
// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=newresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=newresources/status,verbs=get;update;patch
```

### Adding Cloudflare API Methods

1. **Add method to client:**
```go
// pkg/cloudflare/client/client.go
func (c *CloudflareClient) CreateNewResource(ctx context.Context, zoneID string, resource NewResource) (*NewResourceResponse, error) {
    // Input validation
    if zoneID == "" {
        return nil, fmt.Errorf("zone ID is required")
    }
    
    // API call implementation
    resp, err := c.api.SomeCloudflareMethod(ctx, cloudflare.ZoneIdentifier(zoneID), params)
    if err != nil {
        return nil, fmt.Errorf("failed to create resource: %w", err)
    }
    
    return &NewResourceResponse{
        ID: resp.ID,
        // ... other fields
    }, nil
}
```

2. **Add corresponding types:**
```go
type NewResource struct {
    ID          string `json:"id,omitempty"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    // ... other fields
}

type NewResourceResponse struct {
    ID string `json:"id"`
    // ... other fields
}
```

---

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./pkg/controller/...

# Run with coverage
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```

### Integration Tests

1. **Setup test environment:**
```bash
# Create test cluster
make kind-cluster-test

# Deploy test resources
make deploy-test
```

2. **Run integration tests:**
```bash
# Run end-to-end tests
make test-e2e

# Test specific scenarios
go test -tags=integration ./test/...
```

### Example Test Structure

```go
// pkg/controller/ratelimit_controller_test.go
package controller

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    // ... other imports
)

func TestRateLimitController_Reconcile(t *testing.T) {
    tests := []struct {
        name     string
        rateLimit *cloudflareV1alpha1.RateLimit
        want     ctrl.Result
        wantErr  bool
    }{
        {
            name: "successful creation",
            rateLimit: &cloudflareV1alpha1.RateLimit{
                // test data
            },
            want:    ctrl.Result{RequeueAfter: time.Minute * 10},
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

---

## Debugging

### Local Debugging

1. **Enable debug logging:**
```bash
export LOG_LEVEL=debug
make run-local
```

2. **Use debugger:**
```go
// Add breakpoints in your IDE
// Or use delve
dlv debug ./cmd/kubeflare/
```

3. **Mock Cloudflare API:**
```go
// Use interfaces for testing
type CloudflareClientInterface interface {
    CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimitResponse, error)
}

// Mock implementation
type MockCloudflareClient struct{}
func (m *MockCloudflareClient) CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimitResponse, error) {
    return &RateLimitResponse{ID: "mock-id"}, nil
}
```

### Cluster Debugging

1. **Check controller logs:**
```bash
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller -f
```

2. **Describe resources:**
```bash
kubectl describe ratelimit my-rate-limit
kubectl describe wafrule my-waf-rule
```

3. **Check events:**
```bash
kubectl get events --sort-by=.metadata.creationTimestamp
```

4. **Port forward for debugging:**
```bash
kubectl port-forward -n kubeflare-light-system deployment/kubeflare-light-controller 8080:8080
# Access metrics at http://localhost:8080/metrics
```

### Common Issues

| Issue | Solution |
|-------|----------|
| CRD not found | Run `make install` or check CRD installation |
| Permission denied | Check RBAC configuration |
| Cloudflare API errors | Verify API token and permissions |
| Image pull errors | Check image registry and pull policy |

---

## Security Considerations

### Input Validation

Always validate user input:

```go
// Validate URL patterns
func validateURLPattern(pattern string) error {
    if len(pattern) > 1024 {
        return fmt.Errorf("URL pattern too long")
    }
    
    // Check for injection attempts
    if strings.Contains(pattern, `"`) || strings.Contains(pattern, `\`) {
        return fmt.Errorf("URL pattern contains invalid characters")
    }
    
    return nil
}
```

### Expression Sanitization

Sanitize Cloudflare expressions:

```go
func sanitizeExpression(expr string) string {
    // Escape quotes and backslashes
    expr = strings.ReplaceAll(expr, `\`, `\\`)
    expr = strings.ReplaceAll(expr, `"`, `\"`)
    return expr
}
```

### Secrets Management

- Store API tokens in Kubernetes Secrets
- Use RBAC to restrict access to secrets
- Implement token rotation mechanisms
- Never log sensitive information

---

## Performance Optimization

### Controller Performance

1. **Increase concurrency:**
```go
return ctrl.NewControllerManagedBy(mgr).
    For(&cloudflareV1alpha1.RateLimit{}).
    WithOptions(controller.Options{
        MaxConcurrentReconciles: 5, // Increase from 1
    }).
    Complete(r)
```

2. **Implement caching:**
```go
type RulesetCache struct {
    cache map[string]*CachedRuleset
    ttl   time.Duration
    mutex sync.RWMutex
}
```

3. **Add change detection:**
```go
if rateLimit.Status.ObservedGeneration == rateLimit.Generation {
    // No changes, skip API call
    return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}
```

### API Optimization

1. **Batch operations:**
```go
// Batch multiple rule updates
func (c *CloudflareClient) BatchUpdateRules(ctx context.Context, zoneID string, rules []Rule) error {
    // Implementation
}
```

2. **Use appropriate timeouts:**
```go
ctx, cancel := context.WithTimeout(ctx, time.Second*30)
defer cancel()
```

---

## Contributing

### Contribution Workflow

1. **Fork the repository**
2. **Create feature branch:**
```bash
git checkout -b feature/new-feature
```

3. **Make changes and test:**
```bash
make test
make lint
```

4. **Commit with conventional commits:**
```bash
git commit -m "feat: add new rate limiting feature"
```

5. **Push and create PR:**
```bash
git push origin feature/new-feature
# Create PR on GitHub
```

### Code Review Guidelines

- **Security**: Check for input validation and injection vulnerabilities
- **Performance**: Consider performance implications
- **Testing**: Ensure adequate test coverage
- **Documentation**: Update documentation for new features
- **Compatibility**: Maintain backward compatibility

### Release Process

1. **Update version:**
```bash
# Update Chart.yaml and other version files
make bump-version VERSION=v1.2.0
```

2. **Generate changelog:**
```bash
make changelog
```

3. **Create release:**
```bash
git tag v1.2.0
git push origin v1.2.0
```

4. **Build and push images:**
```bash
make release VERSION=v1.2.0
```

---

## Troubleshooting Development Issues

### Build Issues

```bash
# Clean and rebuild
make clean
make build

# Update dependencies
go mod tidy
go mod vendor
```

### Test Failures

```bash
# Run specific test
go test -v -run TestSpecificFunction ./pkg/controller/

# Run with race detection
go test -race ./...
```

### Kind Cluster Issues

```bash
# Delete and recreate cluster
kind delete cluster --name kubeflare-dev
make kind-cluster

# Check cluster status
kubectl cluster-info --context kind-kubeflare-dev
```

---

This developer guide provides the foundation for contributing to kubeflare-light. For more specific questions, check the existing code, tests, and documentation, or reach out to the maintainers.