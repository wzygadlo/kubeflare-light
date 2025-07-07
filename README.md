# kubeflare-light

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Cloudflare API](https://img.shields.io/badge/Cloudflare%20API-v4.5.1-orange.svg)](https://developers.cloudflare.com/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A production-ready Kubernetes controller that manages Cloudflare security resources (rate limits and WAF rules) through Custom Resource Definitions (CRDs). Built with modern Cloudflare Rulesets API, featuring comprehensive drift detection, advanced expression syntax, and enterprise-grade security.

## Features

- **Drift Detection**: Automatically recreates rules deleted from Cloudflare dashboard
- **Advanced Rate Limiting**: Full 2024 Cloudflare features including Bot Management, JA3/JA4 fingerprinting
- **Expression Syntax**: Complete URL operator support (wildcard, starts_with, ends_with, contains, eq, matches)
- **Zone Resolution**: Flexible domain-to-zone-ID mapping via ConfigMaps
- **Finalizer Cleanup**: Ensures proper Cloudflare resource cleanup on deletion
- **Security**: Expression validation prevents injection vulnerabilities
- **Enterprise Ready**: Production-tested with proper error handling and resilient reconciliation

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Kubernetes    │    │  kubeflare-light │    │   Cloudflare    │
│      CRDs       │───▶│    Controller    │───▶│  Rulesets API   │
│  (RateLimit,    │    │                  │    │                 │
│   WAFRule)      │    │  ┌─────────────┐ │    │ ┌─────────────┐ │
└─────────────────┘    │  │ Reconciler  │ │    │ │ Rate Limits │ │
                       │  │   Logic     │ │    │ │  WAF Rules  │ │
┌─────────────────┐    │  └─────────────┘ │    │ └─────────────┘ │
│   ConfigMap     │    │  ┌─────────────┐ │    └─────────────────┘
│ (Zone Mapping)  │───▶│  │    Zone     │ │    
└─────────────────┘    │  │  Resolver   │ │    ┌─────────────────┐
                       │  └─────────────┘ │    │  Drift Detection │
                       │  ┌─────────────┐ │    │  & Auto-Recovery │
                       │  │  Finalizer  │ │    │                 │
                       │  │   Cleanup   │ │    └─────────────────┘
                       │  └─────────────┘ │
                       └──────────────────┘
```

## Installation

### Prerequisites

- Kubernetes 1.19+
- Cloudflare API Token with Zone:Edit permissions
- Go 1.24+ (for building from source)

### Quick Start with Helm

```bash
# Install with Helm (recommended)
helm install kubeflare-light ./chart/kubeflare-light \
  --set cloudflare.apiToken="your-cloudflare-api-token" \
  --set zones.defaultZoneId="your-zone-id"
```

### Manual Installation

```bash
# Clone repository
git clone https://github.com/wzygadlo/kubeflare-light
cd kubeflare-light

# Set environment variables
export CLOUDFLARE_API_TOKEN="your-token"

# Deploy controller and CRDs
make deploy

# Create zone mapping
kubectl create configmap kubeflare-zones -n kubeflare-light-system \
  --from-literal=default=your-default-zone-id \
  --from-literal=example.com=zone-id-1
```

## Usage Examples

### Basic Rate Limiting

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
  namespace: production
spec:
  zoneId: "example.com"  # Resolved via ConfigMap
  description: "API endpoint protection"
  match:
    request:
      urlStartsWith: "/api/v1"
      methods: ["POST", "PUT"]
  threshold: 100
  period: 60
  action:
    mode: "block"
```

### Advanced Multi-Pattern Rate Limiting

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: auth-endpoints-limit
spec:
  zoneId: "auth.example.com"
  description: "Authentication endpoints protection"
  match:
    request:
      urlStartsWithAny:
        - "/auth/login"
        - "/auth/register"
        - "/auth/reset-password"
  threshold: 5
  period: 300
  action:
    mode: "challenge"
```

### Enterprise Bot Protection

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: bot-protection
spec:
  zoneId: "api.example.com"
  description: "Advanced bot protection"
  match:
    request:
      urlPattern: "/api/*"
  threshold: 1000
  period: 3600
  countingCharacteristics:
    ipWithNAT: true
    ja3Fingerprint: true
  expressionFields:
    botManagement:
      verifiedBot: false
      botScore: 30
  action:
    mode: "throttle"
    throttleConfig:
      throttleRate: 10
      blockAction: "challenge"
```

## Development

### Build Commands

```bash
make build              # Build manager binary
make test               # Run tests with coverage
make fmt                # Format Go code
make vet                # Run go vet
make tidy               # Clean up Go modules
```

### Local Development

```bash
make run                # Run controller locally
make docker-build       # Build container image
make deploy             # Deploy to cluster
make undeploy           # Remove from cluster
```

### Testing with Kind

```bash
kind create cluster --name kubeflare-test
make docker-build
kind load docker-image kubeflare-light:latest --name kubeflare-test
make deploy
```

## Monitoring

### Check Controller Status

```bash
# View controller logs
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller-manager

# Check rate limit resources
kubectl get ratelimits -A
kubectl describe ratelimit my-rate-limit -n production

# Verify zone resolution
kubectl get configmap kubeflare-zones -n kubeflare-light-system -o yaml
```

### Resource Status

```bash
# Check rate limit status
kubectl get ratelimit test-limit -o jsonpath='{.status.conditions[0]}'

# View Cloudflare ID
kubectl get ratelimit test-limit -o jsonpath='{.status.cloudflareId}'
```

## Advanced Features

### Drift Detection

The controller automatically detects when rules are manually deleted from the Cloudflare dashboard and recreates them to maintain the desired state:

```yaml
# Example logs showing drift detection in action
2024-07-04T23:27:06Z INFO Rate limit rule not found in Cloudflare, recreating
2024-07-04T23:27:07Z INFO Recreated rate limit in Cloudflare after drift detection
```

**How it works:**
1. Controller checks if Cloudflare rule exists before each update
2. If rule is missing, automatically recreates it with same configuration
3. Updates Kubernetes status with new Cloudflare ID
4. Logs all drift detection events for monitoring

### Zone Resolution

Flexible domain-to-zone-ID mapping via ConfigMaps:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubeflare-zones
  namespace: kubeflare-light-system
data:
  default: "428bf4838ad2fc164e7b3bdf35c70092"
  example.com: "zone-id-for-example-com"
  staging.example.com: "zone-id-for-staging"
  api.example.com: "zone-id-for-api-subdomain"
```

### URL Operators

Complete Cloudflare expression syntax support with proper parentheses formatting:

| Operator | Example | Cloudflare Expression |
|----------|---------|----------------------|
| `urlWildcard` | `/api/*` | `(http.request.uri.path wildcard "/api/*")` |
| `urlStartsWith` | `/auth` | `(starts_with(http.request.uri.path, "/auth"))` |
| `urlEndsWith` | `.json` | `(ends_with(http.request.uri.path, ".json"))` |
| `urlContains` | `admin` | `(contains(http.request.uri.path, "admin"))` |
| `urlEquals` | `/health` | `(http.request.uri.path eq "/health")` |
| `urlMatches` | `^/api/v[0-9]+/` | `(http.request.uri.path matches "^/api/v[0-9]+/")` |

### Multiple Pattern Support (OR Conditions)

```yaml
spec:
  match:
    request:
      urlStartsWithAny: ["/api", "/v1", "/v2"]     # Multiple start patterns
      urlContainsAny: ["admin", "dashboard"]       # Multiple contains patterns
      urlEndsWithAny: [".json", ".xml", ".csv"]    # Multiple end patterns
```

### Finalizer Cleanup

Ensures proper resource cleanup when deleting Kubernetes resources:

```bash
# Normal deletion - finalizer handles Cloudflare cleanup
kubectl delete ratelimit my-rate-limit

# Check logs for cleanup confirmation
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller-manager | grep "Deleted rate limit from Cloudflare"
```

**Stuck deletions** (if controller is down):
```bash
# Force removal by clearing finalizer
kubectl patch ratelimit stuck-resource --type=merge -p '{"metadata":{"finalizers":[]}}'
```

## Troubleshooting

### Common Issues

**Zone Resolution Failures**:
```bash
# Add missing domain mapping
kubectl patch configmap kubeflare-zones -n kubeflare-light-system \
  --type=merge -p '{"data":{"new-domain.com":"zone-id"}}'
```

**Expression Syntax Errors** (Free plan limitations):
- Use `wildcard` instead of `matches` on free plans
- Ensure proper parentheses: `(starts_with(...))`
- Check expression format in Cloudflare dashboard

**Drift Detection Issues**:
```bash
# Enable debug logging
kubectl set env deployment/kubeflare-light-controller-manager DEBUG=true -n kubeflare-light-system

# Watch drift detection in action
kubectl logs -f -n kubeflare-light-system deployment/kubeflare-light-controller-manager | grep -E "(recreating|drift)"
```

### Debug Mode

```bash
# Enable debug logging
kubectl set env deployment/kubeflare-light-controller-manager DEBUG=true -n kubeflare-light-system

# View comprehensive logs
kubectl logs -f -n kubeflare-light-system deployment/kubeflare-light-controller-manager
```

## Configuration Reference

### Environment Variables

- `CLOUDFLARE_API_TOKEN` - Required for Cloudflare API access
- `MAX_CONCURRENT_RECONCILES` - Controller concurrency (default: 1)
- `SYNC_PERIOD` - Reconciliation interval (default: 10m)

### CRD Fields

#### Basic Configuration
- `zoneId` - Domain name or Cloudflare zone ID (resolved via ConfigMap)
- `description` - Human-readable description
- `threshold` - Requests per period
- `period` - Time period in seconds (10-65535)

#### Advanced Features
- `countingCharacteristics` - IP, headers, geography, fingerprinting
- `expressionFields` - Bot Management, request/response matching
- `throttleConfig` - Advanced throttling behavior

#### Status Fields
- `cloudflareId` - Cloudflare rule ID (updated after drift detection)
- `observedGeneration` - Last processed generation
- `conditions` - Ready status with reasons like "Created", "Updated", "Recreated"

## Security Features

### Expression Injection Prevention
- All user input is validated and sanitized
- Proper escaping of special characters in expressions
- Custom error types distinguish between "not found" and API failures

### Production Hardening
- Interface-based architecture for comprehensive testing
- Proper RBAC configuration
- Pod security contexts and network policies
- Input validation at multiple levels

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Setup

```bash
git clone https://github.com/wzygadlo/kubeflare-light
cd kubeflare-light
make deps
make test
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Links

- [Cloudflare API Documentation](https://developers.cloudflare.com/api/go/)
- [Kubernetes Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
- [Controller Runtime](https://book.kubebuilder.io/)


**Status**: Production Ready | Drift Detection | Finalizer Cleanup | Advanced Rate Limiting | Enterprise Features