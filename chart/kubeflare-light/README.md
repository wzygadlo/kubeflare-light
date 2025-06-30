# Kubeflare Light Helm Chart

A Kubernetes controller for managing Cloudflare rate limits and WAF rules.

## Installation

### Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- Cloudflare API token with Zone:Edit permissions

### Quick Start

1. **Add your Cloudflare API token:**

```bash
helm install kubeflare-light ./chart/kubeflare-light \
  --set cloudflare.apiToken="your-cloudflare-api-token-here" \
  --set zones.defaultZoneId="your-zone-id-here"
```

2. **Or use an existing secret:**

```bash
kubectl create secret generic cloudflare-api-token \
  --from-literal=token="your-cloudflare-api-token-here"

helm install kubeflare-light ./chart/kubeflare-light \
  --set cloudflare.existingSecret="cloudflare-api-token" \
  --set zones.defaultZoneId="your-zone-id-here"
```

### Configuration

#### Cloudflare Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `cloudflare.apiToken` | Cloudflare API token | `""` |
| `cloudflare.existingSecret` | Use existing secret for API token | `""` |
| `cloudflare.secretKey` | Key in secret containing the token | `"token"` |

#### Zone Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `zones.defaultZoneId` | Default Cloudflare zone ID | `""` |
| `zones.configMapName` | ConfigMap name for zone mappings | `"kubeflare-zones"` |
| `zones.createConfigMap` | Create ConfigMap for zones | `true` |
| `zones.mappings` | Domain to zone ID mappings | `{}` |

Example zone mappings:
```yaml
zones:
  defaultZoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"
  mappings:
    example.com: "4b5c7c7ca06cddfe0fe680cf0b570920"
    api.example.com: "5c6d8d8db07dedfe0fe680cf0b570930"
```

#### Controller Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.maxConcurrentReconciles` | Max concurrent reconcilers | `1` |
| `controller.leaderElection` | Enable leader election | `true` |
| `controller.metricsAddr` | Metrics server address | `"0.0.0.0:8080"` |

#### Image Settings

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `"kubeflare-light"` |
| `image.tag` | Image tag | `"latest"` |
| `image.pullPolicy` | Image pull policy | `"IfNotPresent"` |

#### Resource Limits

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `"100m"` |
| `resources.limits.memory` | Memory limit | `"128Mi"` |
| `resources.requests.cpu` | CPU request | `"100m"` |
| `resources.requests.memory` | Memory request | `"64Mi"` |

### Usage Examples

#### Rate Limit Example

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
spec:
  zoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"  # Or use domain name if configured in zones ConfigMap
  description: "Rate limit for API endpoints"
  match:
    request:
      urlPattern: "/api/v1/*"
      methods: ["GET", "POST"]
  threshold: 100
  period: 60
  action:
    mode: "block"
    timeout: 300
```

#### WAF Rule Example

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-malicious-ips
spec:
  zoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"
  name: "Block Malicious IPs"
  description: "Block known malicious IP addresses"
  expression: 'ip.src in {192.0.2.1 198.51.100.1}'
  action: "block"
  priority: 1
```

### Zone ID Resolution

The controller supports multiple ways to specify zone IDs:

1. **Direct Zone ID**: Use the actual Cloudflare zone ID in the resource
2. **Domain Mapping**: Configure domain-to-zone mappings in the ConfigMap
3. **Default Zone**: Set a default zone ID for all resources

Priority order:
1. If `spec.zoneId` is a valid zone ID → use directly
2. If `spec.zoneId` matches a domain in ConfigMap → use mapped zone ID  
3. Use `default` key from ConfigMap
4. Use `DEFAULT_ZONE_ID` environment variable

### Uninstallation

```bash
helm uninstall kubeflare-light
```

**Note**: CRDs are preserved by default. To remove them:

```bash
kubectl delete crd ratelimits.cloudflare.kubeflare.io wafrules.cloudflare.kubeflare.io
```