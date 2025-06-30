# 📚 Kubeflare Light API Reference

This document provides comprehensive API reference for all Custom Resource Definitions (CRDs) in kubeflare-light.

## Table of Contents

- [RateLimit API](#ratelimit-api)
- [WAFRule API](#wafrule-api)
- [Common Types](#common-types)
- [Status Conditions](#status-conditions)
- [Examples](#examples)

---

## RateLimit API

### Overview

The `RateLimit` custom resource manages Cloudflare rate limiting rules using the modern Rulesets API v4.5.1.

### API Version

- **API Version**: `cloudflare.kubeflare.io/v1alpha1`
- **Kind**: `RateLimit`
- **Plural**: `ratelimits`
- **Singular**: `ratelimit`

### Resource Spec

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: string
  namespace: string
spec:
  zoneId: string                    # Required: Zone ID or domain name
  description: string               # Optional: Human-readable description
  match: MatchCriteria             # Optional: Request matching criteria
  threshold: integer               # Required: Requests per period
  period: integer                  # Required: Time period in seconds
  action: ActionConfiguration      # Optional: Action when threshold exceeded
  disabled: boolean                # Optional: Whether rule is disabled
```

#### MatchCriteria

Defines criteria for matching requests against the rate limit rule.

```yaml
match:
  request:
    methods: []string              # HTTP methods (GET, POST, etc.)
    schemes: []string              # URI schemes (http, https)
    urlPattern: string             # URL pattern with wildcards
  response:
    statuses: []integer            # HTTP response status codes
```

**Field Details:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `request.methods` | `[]string` | No | List of HTTP methods to match. Valid: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS |
| `request.schemes` | `[]string` | No | URI schemes to match. Valid: http, https |
| `request.urlPattern` | `string` | No | URL pattern with wildcard support (e.g., `/api/v1/*`) |
| `response.statuses` | `[]integer` | No | HTTP response status codes (100-599) |

#### ActionConfiguration

Defines the action to take when the rate limit is exceeded.

```yaml
action:
  mode: string                     # Action mode
  timeout: integer                 # Optional: Timeout duration
```

**Action Modes:**

| Mode | Description |
|------|-------------|
| `block` | Block the request completely |
| `challenge` | Present a challenge page |
| `js_challenge` | JavaScript challenge |
| `log` | Log the event without blocking |

**Field Details:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `mode` | `string` | No | Action mode (default: "block") |
| `timeout` | `integer` | No | Challenge timeout in seconds (default: 86400) |

#### Complete Example

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
  namespace: production
spec:
  zoneId: "example.com"
  description: "Rate limit for API endpoints"
  match:
    request:
      methods: ["GET", "POST", "PUT", "DELETE"]
      schemes: ["https"]
      urlPattern: "/api/v1/*"
    response:
      statuses: [200, 201, 202, 204]
  threshold: 100
  period: 60
  action:
    mode: "challenge"
    timeout: 3600
  disabled: false
```

---

## WAFRule API

### Overview

The `WAFRule` custom resource manages Cloudflare Web Application Firewall rules using the Rulesets API.

### API Version

- **API Version**: `cloudflare.kubeflare.io/v1alpha1`
- **Kind**: `WAFRule`
- **Plural**: `wafrules`
- **Singular**: `wafrule`

### Resource Spec

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: string
  namespace: string
spec:
  zoneId: string                   # Required: Zone ID or domain name
  name: string                     # Required: Rule name in Cloudflare
  description: string              # Optional: Human-readable description
  expression: string               # Required: Cloudflare expression
  action: string                   # Required: Action to take
  priority: integer                # Optional: Rule priority (1-1000000)
  paused: boolean                  # Optional: Whether rule is paused
  actionParameters: ActionParams   # Optional: Additional action parameters
```

#### ActionParams

Additional parameters for WAF rule actions.

```yaml
actionParameters:
  response:
    statusCode: integer            # HTTP status code
    contentType: string            # Response content type
    content: string                # Response body content
```

**Field Details:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `response.statusCode` | `integer` | No | HTTP status code (100-599) |
| `response.contentType` | `string` | No | MIME type (e.g., "application/json") |
| `response.content` | `string` | No | Response body content |

#### WAF Actions

| Action | Description |
|--------|-------------|
| `block` | Block the request |
| `challenge` | Present a challenge page |
| `js_challenge` | JavaScript challenge |
| `allow` | Allow the request to pass |
| `log` | Log the event without action |

#### Complete Example

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-malicious-ips
  namespace: security
spec:
  zoneId: "example.com"
  name: "Block Known Bad Actors"
  description: "Block requests from known malicious IP addresses"
  expression: '(ip.src in {192.0.2.1 192.0.2.2}) or (cf.threat_score gt 14)'
  action: "block"
  priority: 1
  paused: false
  actionParameters:
    response:
      statusCode: 403
      contentType: "application/json"
      content: '{"error": "Access denied", "reason": "Security policy violation"}'
```

---

## Common Types

### Zone Resolution

Both `RateLimit` and `WAFRule` resources support flexible zone ID resolution:

1. **Direct Zone ID**: Use the actual Cloudflare zone ID
   ```yaml
   spec:
     zoneId: "4b5c7c7ca06cddfe0fe680cf0b570920"
   ```

2. **Domain Name**: Use domain name (resolved via ConfigMap)
   ```yaml
   spec:
     zoneId: "example.com"
   ```

3. **Default Zone**: Leave empty to use default zone
   ```yaml
   spec:
     zoneId: ""
   ```

### Status Fields

All resources include standard Kubernetes status fields:

```yaml
status:
  cloudflareId: string             # Cloudflare resource ID
  observedGeneration: integer      # Last observed generation
  conditions:                      # Status conditions array
  - type: string
    status: string
    reason: string
    message: string
    lastTransitionTime: string
```

---

## Status Conditions

### Condition Types

| Type | Description |
|------|-------------|
| `Ready` | Resource is successfully synchronized with Cloudflare |
| `Error` | An error occurred during synchronization |

### Condition Status

| Status | Description |
|--------|-------------|
| `True` | Condition is true |
| `False` | Condition is false |
| `Unknown` | Status cannot be determined |

### Common Reasons

| Reason | Description |
|--------|-------------|
| `Created` | Resource created successfully |
| `Updated` | Resource updated successfully |
| `CreateFailed` | Failed to create resource |
| `UpdateFailed` | Failed to update resource |
| `ZoneResolutionFailed` | Could not resolve zone ID |

---

## Cloudflare Expression Language

### Basic Syntax

Cloudflare expressions use a SQL-like syntax for matching requests:

```javascript
// Field comparison
http.request.method eq "POST"

// Pattern matching
http.request.uri.path wildcard "/api/*"

// IP address matching
ip.src in {192.168.1.1 10.0.0.0/8}

// Logical operators
(http.request.method eq "POST") and (http.request.uri.path wildcard "/api/*")
```

### Available Fields

#### HTTP Request Fields

| Field | Type | Description |
|-------|------|-------------|
| `http.request.method` | String | HTTP method (GET, POST, etc.) |
| `http.request.uri.path` | String | URI path |
| `http.request.uri.query` | String | Query string |
| `http.request.headers["name"]` | String | HTTP header value |
| `http.user_agent` | String | User-Agent header |

#### IP and Geolocation

| Field | Type | Description |
|-------|------|-------------|
| `ip.src` | IP | Source IP address |
| `ip.geoip.country` | String | Country code (ISO 3166-1 alpha-2) |
| `ip.geoip.asnum` | Number | AS number |

#### Cloudflare Specific

| Field | Type | Description |
|-------|------|-------------|
| `cf.threat_score` | Number | Threat score (0-100) |
| `cf.bot_score` | Number | Bot score (1-99) |
| `cf.zone.id` | String | Zone ID |

### Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `eq` | Equals | `http.request.method eq "GET"` |
| `ne` | Not equals | `http.request.method ne "POST"` |
| `in` | In list/range | `ip.src in {1.1.1.1 2.2.2.2}` |
| `contains` | Contains substring | `http.request.uri.path contains "/admin"` |
| `wildcard` | Wildcard matching | `http.request.uri.path wildcard "/api/*"` |
| `matches` | Regex matching | `http.user_agent matches "bot\|crawler"` |

---

## Examples

### Rate Limiting Examples

#### Basic API Rate Limiting
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: basic-api-limit
spec:
  zoneId: "example.com"
  description: "Basic API rate limiting"
  match:
    request:
      urlPattern: "/api/*"
  threshold: 1000
  period: 60
  action:
    mode: "block"
```

#### Per-Method Rate Limiting
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: post-method-limit
spec:
  zoneId: "example.com"
  description: "Stricter limits for POST requests"
  match:
    request:
      methods: ["POST", "PUT", "DELETE"]
      urlPattern: "/api/*"
  threshold: 100
  period: 60
  action:
    mode: "challenge"
    timeout: 3600
```

#### Admin Panel Protection
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: admin-protection
spec:
  zoneId: "example.com"
  description: "Protect admin panel from brute force"
  match:
    request:
      urlPattern: "/admin/*"
  threshold: 10
  period: 300
  action:
    mode: "block"
```

### WAF Rule Examples

#### Block Known Bad IPs
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-bad-ips
spec:
  zoneId: "example.com"
  name: "Block Known Bad IPs"
  expression: 'ip.src in {192.0.2.1 198.51.100.0/24}'
  action: "block"
  priority: 1
```

#### Challenge High Threat Score
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: challenge-threats
spec:
  zoneId: "example.com"
  name: "Challenge High Threat Score"
  expression: 'cf.threat_score gt 14'
  action: "challenge"
  priority: 10
```

#### Block Specific User Agents
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-bad-bots
spec:
  zoneId: "example.com"
  name: "Block Malicious Bots"
  expression: 'http.user_agent matches "(badbot|scraper|scanner)"'
  action: "block"
  priority: 5
```

#### Geographic Restrictions
```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: geo-restrictions
spec:
  zoneId: "example.com"
  name: "Block Specific Countries"
  expression: 'ip.geoip.country in {"CN" "RU" "KP"}'
  action: "block"
  priority: 2
```

---

## Best Practices

### Expression Writing

1. **Use specific patterns**: Prefer specific URL patterns over broad wildcards
2. **Combine conditions**: Use logical operators for complex matching
3. **Test expressions**: Validate expressions in Cloudflare dashboard first
4. **Performance**: Simpler expressions perform better

### Resource Naming

1. **Descriptive names**: Use clear, descriptive resource names
2. **Environment prefixes**: Consider prefixing with environment (prod-, staging-)
3. **Consistent convention**: Establish naming conventions across team

### Zone Management

1. **Use ConfigMap**: Configure zone mappings in ConfigMap for flexibility
2. **Environment separation**: Use different namespaces for different environments
3. **RBAC**: Apply appropriate RBAC policies for security

### Monitoring

1. **Status conditions**: Monitor resource status conditions
2. **Controller logs**: Watch controller logs for errors
3. **Cloudflare dashboard**: Verify rules in Cloudflare dashboard
4. **Metrics**: Implement monitoring for rule effectiveness