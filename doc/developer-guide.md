# Developer Guide

This guide explains the codebase structure and packages for kubeflare-light developers.

## Package Structure

```
pkg/
├── apis/cloudflare/v1alpha1/     # CRD type definitions
├── cloudflare/client/            # Cloudflare API client wrapper
├── controller/                   # Kubernetes controllers
└── utils/                        # Shared utilities
```

## Package Details

### pkg/apis/cloudflare/v1alpha1/

**Purpose**: Kubernetes Custom Resource Definitions (CRDs) for Cloudflare resources.

**Key Files**:
- `groupversion_info.go` - API group registration (`cloudflare.kubeflare.io/v1alpha1`)
- `ratelimit_types.go` - RateLimit CRD structure
- `wafrule_types.go` - WAFRule CRD structure
- `zz_generated.deepcopy.go` - Auto-generated DeepCopy methods

**Core Types**:

**RateLimit CRD**:
```go
type RateLimitSpec struct {
    ZoneID      string              // Zone ID or domain name
    Description string              // Human-readable description
    Threshold   int                 // Request count trigger
    Period      int                 // Time window in seconds
    Action      *RateLimitAction    // Action when triggered
    Match       *RateLimitMatch     // Request matching criteria
    Disabled    bool                // Enable/disable state
}
```

**WAFRule CRD**:
```go
type WAFRuleSpec struct {
    ZoneID           string                // Zone ID or domain name
    Description      string                // Human-readable description
    Expression       string                // Cloudflare filter expression
    Action           string                // Action to take
    Priority         int                   // Rule priority
    Paused           bool                  // Enable/disable state
    ActionParameters *WAFActionParameters  // Additional action config
}
```

**Rate Limit Matching**:
- **URL Operators**: `urlWildcard`, `urlStartsWith`, `urlEndsWith`, `urlContains`, `urlEquals`
- **Multiple Patterns**: `urlStartsWithAny`, `urlContainsAny` (OR conditions)
- **HTTP**: `methods`, `schemes`, `headers`, `cookies`, `queryParams`
- **Geography**: `countries`, `countriesExclude`, `ipSources`, `asns`
- **Bot Management**: `verifiedBot`, `botScore`, `ja3Fingerprint`, `ja4Fingerprint`
- **Request Body**: `bodySize`, `bodyContains`, `bodyFormFields`, `bodyJSONFields`

### pkg/cloudflare/client/

**Purpose**: Wrapper around the official Cloudflare Go SDK v4 for simplified operations.

**Key Components**:

**CloudflareClient**:
```go
type CloudflareClient struct {
    client *cloudflare.Client  // Official v4 SDK client
}
```

**Rate Limit Operations**:
- `CreateRateLimit()` - Create new rate limit rule
- `GetRateLimit()` - Retrieve existing rule
- `UpdateRateLimit()` - Update rule configuration
- `DeleteRateLimit()` - Remove rule
- `ListRateLimits()` - List all rules for zone

**WAF Rule Operations**:
- `CreateWAFRule()` - Create new WAF rule
- `GetWAFRule()` - Retrieve existing rule
- `UpdateWAFRule()` - Update rule configuration
- `DeleteWAFRule()` - Remove rule
- `ListWAFRules()` - List all rules for zone

**Authentication**:
- Uses API token authentication
- `VerifyToken()` - Validates token on startup

### pkg/controller/

**Purpose**: Kubernetes controllers that implement the reconciliation logic.

**Key Files**:
- `ratelimit_controller.go` - RateLimit resource controller
- `wafrule_controller.go` - WAFRule resource controller
- `config.go` - Shared configuration utilities

**Controller Pattern**:

Both controllers follow the same pattern:
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch Kubernetes resource
    // 2. Handle deletion (finalizers)
    // 3. Resolve zone ID via ConfigMap
    // 4. Convert to Cloudflare format
    // 5. Sync with Cloudflare API
    // 6. Update Kubernetes status
    // 7. Schedule next reconciliation
}
```

**Key Features**:

**Finalizers**:
- `cloudflare.io/ratelimit-finalizer`
- `cloudflare.io/wafrule-finalizer`
- Ensures Cloudflare cleanup on resource deletion

**Drift Detection**:
- Periodic checks of Cloudflare state vs desired state
- Automatically recreates deleted rules
- Configurable via `RECONCILE_INTERVAL` (default: 10m)

**Error Handling**:
- Retry failed operations with backoff
- Configurable via `ERROR_RETRY_INTERVAL` (default: 5m)
- Updates resource status with error conditions

**Expression Building** (RateLimit only):
- Converts CRD match criteria to Cloudflare expressions
- Input sanitization to prevent injection
- Supports 50+ matching fields

### pkg/utils/

**Purpose**: Shared utilities used across controllers.

**zone.go - ZoneResolver**:

Handles flexible zone ID resolution:

```go
type ZoneResolver struct {
    client    client.Client    // Kubernetes client
    configMap string          // ConfigMap name
    namespace string          // ConfigMap namespace
}
```

**Resolution Logic**:
1. Check if input is already a zone ID (32-char hex)
2. Look up domain in ConfigMap (`kubeflare-zones`)
3. Fall back to "default" key in ConfigMap
4. Fall back to `DEFAULT_ZONE_ID` environment variable
5. Error if no mapping found

**Configuration**:
- ConfigMap: `ZONES_CONFIGMAP_NAME` (default: `kubeflare-zones`)
- Namespace: `ZONES_CONFIGMAP_NAMESPACE` (default: `kubeflare-light-system`)

**Example ConfigMap**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubeflare-zones
  namespace: kubeflare-light-system
data:
  default: "zone-id-1"
  example.com: "zone-id-2"
  api.example.com: "zone-id-3"
```

## Development Workflow

### Adding New Rate Limit Fields

1. **Update CRD**:
```go
// pkg/apis/cloudflare/v1alpha1/ratelimit_types.go
type RateLimitMatchRequest struct {
    // ... existing fields
    NewField string `json:"newField,omitempty"`
}
```

2. **Regenerate**:
```bash
make generate
make manifests
```

3. **Update Controller**:
```go
// pkg/controller/ratelimit_controller.go
func (r *RateLimitReconciler) buildCloudflareExpression(req *RateLimitMatchRequest) string {
    // ... existing logic
    if req.NewField != "" {
        expressions = append(expressions,
            fmt.Sprintf("(http.request.new_field eq \"%s\")",
                r.sanitizeString(req.NewField)))
    }
}
```

### Adding New Cloudflare API Methods

1. **Add Interface Method**:
```go
// pkg/cloudflare/client/client.go
type CloudflareClientInterface interface {
    // ... existing methods
    NewMethod(ctx context.Context, zoneID string, params NewParams) (*NewResponse, error)
}
```

2. **Implement Method**:
```go
func (c *CloudflareClient) NewMethod(ctx context.Context, zoneID string, params NewParams) (*NewResponse, error) {
    // Input validation
    if zoneID == "" {
        return nil, fmt.Errorf("zone ID is required")
    }

    // Call Cloudflare v4 SDK
    resp, err := c.client.SomeAPI.Method(ctx, cloudflare.ZoneIdentifier(zoneID), params)
    if err != nil {
        return nil, fmt.Errorf("API call failed: %w", err)
    }

    return &NewResponse{ID: resp.ID}, nil
}
```

## Testing

### Unit Tests
```bash
make test
```

### Local Development
```bash
export CLOUDFLARE_API_TOKEN="your-token"
make run
```

### Build and Deploy
```bash
make build
make docker-build
make deploy
```

## Configuration

### Environment Variables

**Required**:
- `CLOUDFLARE_API_TOKEN` - Cloudflare API token with Zone:Edit permissions

**Optional**:
- `RECONCILE_INTERVAL` - Drift detection frequency (default: "10m")
- `ERROR_RETRY_INTERVAL` - Error retry frequency (default: "5m")
- `DEFAULT_ZONE_ID` - Fallback zone ID when ConfigMap lookup fails
- `ZONES_CONFIGMAP_NAME` - ConfigMap name (default: "kubeflare-zones")
- `ZONES_CONFIGMAP_NAMESPACE` - ConfigMap namespace (default: "kubeflare-light-system")

### Resource Status

Both CRDs use similar status patterns:

```go
type Status struct {
    CloudflareID         string             // ID in Cloudflare
    ObservedGeneration   int64              // Last processed generation
    Conditions           []metav1.Condition // Status conditions
}
```

**Condition Types**:
- `Ready` - Resource is synced with Cloudflare
- Reasons: `Created`, `Updated`, `Synced`, `CreateFailed`, `UpdateFailed`, `ZoneResolutionFailed`

## Architecture Notes

### Controller Design
- **Single Responsibility**: Each controller manages one CRD type
- **Declarative**: Desired state in Kubernetes, actual state in Cloudflare
- **Resilient**: Handles API failures, rate limits, and network issues
- **Observable**: Structured logging and status updates

### Security
- **Input Sanitization**: All user inputs are validated and sanitized
- **Least Privilege**: Controllers only access required resources
- **Secret Management**: API tokens stored in Kubernetes Secrets

### Performance
- **Concurrent Reconciliation**: Configurable via `MaxConcurrentReconciles`
- **Change Detection**: Only update when Kubernetes generation changes
- **Efficient Polling**: Configurable reconciliation intervals

This guide reflects the actual current codebase structure and implementation.