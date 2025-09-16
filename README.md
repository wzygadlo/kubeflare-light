# kubeflare-light


A Kubernetes controller that manages Cloudflare security resources (rate limits and WAF rules) through Custom Resource Definitions (CRDs).

> **Note**: This project is based on the original Kubeflare but has been refactored with all features removed and improved focus on rate limits and WAF rules.

## Features

- **Drift Detection**: Automatically recreates rules deleted from Cloudflare dashboard
- **Zone Resolution**: Domain-to-zone-ID mapping via ConfigMaps
- **Finalizer Cleanup**: Proper Cloudflare resource cleanup on deletion
- **Advanced Matching**: 50+ rate limiting fields including bot management and geographic filtering

## Installation

### Prerequisites

- Kubernetes 1.19+
- Cloudflare API Token with Zone:Edit permissions

### Quick Start with Helm

```bash
helm install kubeflare-light ./chart/kubeflare-light \
  --set cloudflare.apiToken="your-cloudflare-api-token" \
  --set zones.defaultZoneId="your-zone-id"
```

### Manual Installation

```bash
git clone https://github.com/wzygadlo/kubeflare-light
cd kubeflare-light
export CLOUDFLARE_API_TOKEN="your-token"
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
  description: "Rate limit for API endpoints"
  match:
    request:
      urlStartsWith: "/api/v1"
      methods: ["POST", "PUT"]
  threshold: 100
  period: 60
  action:
    mode: "block"
```

### Advanced Pattern Matching

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: auth-endpoints-limit
spec:
  zoneId: "auth.example.com"
  description: "Rate limit for authentication endpoints"
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

## Development

```bash
make build              # Build manager binary
make test               # Run tests with coverage
make run                # Run controller locally
make deploy             # Deploy to cluster
```

## Monitoring

```bash
# View controller logs
kubectl logs -n kubeflare-light-system deployment/kubeflare-light-controller-manager

# Check resources
kubectl get ratelimits,wafrules -A
```

## Configuration

### Environment Variables

- `CLOUDFLARE_API_TOKEN` - Required for Cloudflare API access
- `RECONCILE_INTERVAL` - Drift detection frequency (default: 10m)
- `ERROR_RETRY_INTERVAL` - Error retry frequency (default: 5m)

### Key Fields

- `zoneId` - Domain name or zone ID (resolved via ConfigMap)
- `threshold` - Requests per period (1-1000000)
- `period` - Time period in seconds (10, 60, 120, 300, 600, 3600)
- `action.mode` - "simulate", "block", "challenge", "js_challenge", "managed_challenge"

### URL Matching

- `urlWildcard` - Wildcard pattern (e.g., "/api/*")
- `urlStartsWith` - Path prefix (e.g., "/auth")
- `urlStartsWithAny` - Multiple patterns (OR condition)
- `urlContains`, `urlEquals`, `urlMatches` - Various matching modes

### Advanced Features

- Geographic filtering: `countries`, `countriesExclude`
- Network filtering: `ipSources`, `asns`
- Bot management: `verifiedBot`, `botScore`, `ja3Fingerprint`
- Header matching: `headers`, `userAgent`, `cookies`

## Troubleshooting

**Zone Resolution Failures:**
```bash
kubectl patch configmap kubeflare-zones -n kubeflare-light-system \
  --type=merge -p '{"data":{"new-domain.com":"zone-id"}}'
```

**Debug Logging:**
```bash
kubectl set env deployment/kubeflare-light-controller-manager DEBUG=true -n kubeflare-light-system
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
