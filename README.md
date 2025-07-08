# kubeflare-light

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.19+-blue.svg)](https://kubernetes.io/)
[![Cloudflare API](https://img.shields.io/badge/Cloudflare%20API-v4.5.1-orange.svg)](https://developers.cloudflare.com/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A production-ready Kubernetes controller that manages Cloudflare security resources (rate limits and WAF rules) through Custom Resource Definitions (CRDs). Built with modern Cloudflare Rulesets API, featuring comprehensive drift detection, advanced expression syntax, and enterprise-grade security.

## Features

- **Comprehensive Drift Detection**: Automatically recreates rules deleted from Cloudflare dashboard with configurable check intervals
- **50+ Rate Limiting Fields**: Full 2025 Cloudflare features including Bot Management, JA3/JA4 fingerprinting, geographic filtering
- **Advanced Expression Building**: Complete URL operator support with proper parentheses wrapping and input sanitization
- **Name-Based Alignment**: Kubernetes metadata.name automatically syncs as Cloudflare description for consistency
- **Configurable Reconciliation**: Environment variables control drift detection speed and error retry intervals
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
  name: api-rate-limit  # Name appears as description in Cloudflare
  namespace: production
spec:
  zoneId: "example.com"  # Resolved via ConfigMap
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
  name: auth-endpoints-limit  # Name used for Cloudflare alignment
spec:
  zoneId: "auth.example.com"
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
  name: bot-protection  # Name synced to Cloudflare
spec:
  zoneId: "api.example.com"
  match:
    request:
      urlWildcard: "/api/*"
      # Advanced bot management (Enterprise feature)
      verifiedBot: false
      botScore: "lt:30"  # Less than 30 bot score
      ja3Fingerprint: ["abc123", "def456"]  # Specific JA3 fingerprints
      # Geographic filtering
      countries: ["US", "CA"]  # Only US and Canada
      countriesExclude: ["CN", "RU"]  # Exclude specific countries
      # Request body analysis
      bodySize: "gt:1024"  # Larger than 1KB
      bodyContains: "suspicious_pattern"
  threshold: 100
  period: 300
  action:
    mode: "challenge"
```

### Comprehensive Rate Limiting with All Features

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: comprehensive-protection
spec:
  zoneId: "production.example.com"
  match:
    request:
      # URL matching with multiple patterns
      urlStartsWithAny: ["/api", "/v1", "/v2"]
      urlEndsWithAny: [".json", ".xml"]
      # HTTP methods and schemes
      methods: ["POST", "PUT", "DELETE"]
      schemes: ["https"]
      # Header matching
      headers:
        "x-api-key": "exists:"  # Check if header exists
        "user-agent": "contains:bot"  # Contains "bot"
      headersAny:
        "x-forwarded-for": "eq:127.0.0.1"  # Specific IP
      # Geographic and network filtering
      countries: ["US", "GB", "DE"]
      asns: [13335, 15169]  # Cloudflare and Google ASNs
      ipSources: ["192.168.1.0/24"]
      ipSourcesExclude: ["10.0.0.0/8"]
      # Bot management (Enterprise)
      verifiedBot: false
      botScore: "ge:50"  # Greater than or equal to 50
      ja3Fingerprint: ["fingerprint1", "fingerprint2"]
      ja4Fingerprint: ["ja4_fingerprint"]
      # Request analysis
      bodySize: "lt:5000"  # Less than 5KB
      bodyFormFields:
        "username": "exists:"
        "password": "exists:"
      bodyJSONFields:
        "$.user.id": "exists:"
      # Custom expression for complex logic
      customExpression: "(http.request.headers[\"x-custom\"][0] eq \"value\")"
  threshold: 50
  period: 600  # 10 minutes
  action:
    mode: "block"
  disabled: false
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
2025-07-07T21:16:29Z INFO Rate limit not found in Cloudflare, recreating {"cloudflareId": "abc123"}
2025-07-07T21:16:31Z INFO Created rate limit in Cloudflare {"cloudflareId": "def456"}
2025-07-07T21:18:31Z INFO Rate limit is in sync with Cloudflare {"cloudflareId": "def456"}
```

**How it works:**
1. Controller checks Cloudflare state during every reconciliation cycle (configurable via `RECONCILE_INTERVAL`)
2. If rule is missing, automatically recreates it with same configuration
3. Updates Kubernetes status with new Cloudflare ID
4. Logs all drift detection events for monitoring

**Configure drift detection speed:**
```yaml
# Fast drift detection (check every 30 seconds)
env:
- name: RECONCILE_INTERVAL
  value: "30s"

# Conservative (check every 30 minutes)  
env:
- name: RECONCILE_INTERVAL
  value: "30m"
```

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
- `RECONCILE_INTERVAL` - How often to check for drift (default: 10m, e.g., "2m", "30s")
- `ERROR_RETRY_INTERVAL` - How often to retry on errors (default: 5m, e.g., "1m", "30s")

### CRD Fields

#### Basic Configuration
- `zoneId` - Domain name or Cloudflare zone ID (resolved via ConfigMap)
- `threshold` - Requests per period (1-1000000)
- `period` - Time period in seconds (10, 60, 120, 300, 600, 3600)
- `action.mode` - Action to take: "block", "challenge", "js_challenge", "log"
- `disabled` - Whether the rule is disabled (default: false)

#### URL Matching (Choose One or Combine with Arrays)
- `urlPattern` - Legacy wildcard pattern (e.g., "/api/*")
- `urlWildcard` - Wildcard matching (e.g., "/api/*")
- `urlStartsWith` - Path starts with (e.g., "/auth")
- `urlEndsWith` - Path ends with (e.g., ".json")
- `urlContains` - Path contains (e.g., "admin")
- `urlEquals` - Exact path match (e.g., "/health")
- `urlMatches` - Regex matching (Enterprise)
- `urlStartsWithAny` - Array of start patterns (OR condition)
- `urlEndsWithAny` - Array of end patterns (OR condition)
- `urlContainsAny` - Array of contains patterns (OR condition)
- `urlEqualsAny` - Array of exact matches (OR condition)
- `urlMatchesAny` - Array of regex patterns (OR condition)
- `urlWildcardAny` - Array of wildcard patterns (OR condition)

#### HTTP Request Matching
- `methods` - HTTP methods array (e.g., ["GET", "POST"])
- `schemes` - URL schemes array (e.g., ["https"])
- `host` - Exact hostname match
- `hostAny` - Array of hostnames (OR condition)
- `userAgent` - User-Agent contains check
- `referer` - Referer header exact match

#### Advanced Header/Cookie/Query Matching
- `headers` - Map of header rules (e.g., "x-api-key": "exists:")
- `headersAny` - Map of header rules (OR condition)
- `cookies` - Map of cookie rules (e.g., "session": "exists:")
- `queryParams` - Map of query parameter rules

#### Geographic and Network Filtering
- `countries` - Country codes array (e.g., ["US", "CA"])
- `countriesExclude` - Excluded countries (AND NOT condition)
- `asns` - ASN numbers array (e.g., [13335, 15169])
- `ipSources` - IP ranges array (e.g., ["192.168.1.0/24"])
- `ipSourcesExclude` - Excluded IP ranges

#### Bot Management (Enterprise)
- `verifiedBot` - Boolean for verified bot status
- `botScore` - Bot score comparison (e.g., "lt:30", "ge:50")
- `ja3Fingerprint` - Array of JA3 fingerprints
- `ja4Fingerprint` - Array of JA4 fingerprints

#### Request Body Analysis (Enterprise)
- `bodySize` - Body size comparison (e.g., "gt:1024", "lt:5000")
- `bodyContains` - Body content contains check
- `bodyFormFields` - Map of form field rules
- `bodyJSONFields` - Map of JSON path rules (e.g., "$.user.id": "exists:")

#### Custom Logic
- `customExpression` - Raw Cloudflare expression for complex logic

#### Status Fields
- `cloudflareId` - Cloudflare rule ID (updated after drift detection)
- `observedGeneration` - Last processed generation  
- `conditions` - Ready status with reasons like "Created", "Updated", "Synced"

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
