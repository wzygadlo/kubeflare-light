# API Reference

Complete reference for kubeflare-light Custom Resource Definitions (CRDs).

## RateLimit

**API Version**: `cloudflare.kubeflare.io/v1alpha1`
**Kind**: `RateLimit`

Rate limits control the number of requests from clients to your website or API.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `zoneId` | string | Yes | Zone ID or domain name (resolved via ConfigMap) |
| `description` | string | No | Human-readable description |
| `threshold` | int | Yes | Number of requests that trigger the limit (1-1000000) |
| `period` | int | Yes | Time period in seconds (10, 60, 120, 300, 600, 3600) |
| `action` | [RateLimitAction](#ratelimitaction) | No | Action when threshold exceeded |
| `match` | [RateLimitMatch](#ratelimitmatch) | No | Request matching criteria |
| `disabled` | bool | No | Whether rule is disabled (default: false) |

### RateLimitAction

| Field | Type | Description |
|-------|------|-------------|
| `mode` | string | Action type: `log`, `block`, `challenge`, `js_challenge`, `managed_challenge` |
| `timeout` | int | Duration in seconds for action effect |
| `response` | [RateLimitActionResponse](#ratelimitactionresponse) | Custom response configuration |

### RateLimitActionResponse

| Field | Type | Description |
|-------|------|-------------|
| `contentType` | string | Content-Type header value |
| `body` | string | Response body content |

### RateLimitMatch

| Field | Type | Description |
|-------|------|-------------|
| `request` | [RateLimitMatchRequest](#ratelimitmatchrequest) | Request characteristics to match |
| `response` | [RateLimitMatchResponse](#ratelimitmatchresponse) | Response characteristics to match |

### RateLimitMatchRequest

#### URL Matching

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `urlWildcard` | string | Wildcard pattern match | `/api/*` |
| `urlStartsWith` | string | Path prefix match | `/auth` |
| `urlEndsWith` | string | Path suffix match | `.json` |
| `urlContains` | string | Substring match | `admin` |
| `urlEquals` | string | Exact path match | `/health` |
| `urlMatches` | string | Regex pattern match | `^/api/v[0-9]+/` |

#### Multiple URL Patterns (OR logic)

| Field | Type | Description |
|-------|------|-------------|
| `urlWildcardAny` | []string | Match any wildcard pattern |
| `urlStartsWithAny` | []string | Match any prefix |
| `urlEndsWithAny` | []string | Match any suffix |
| `urlContainsAny` | []string | Match any substring |
| `urlEqualsAny` | []string | Match any exact path |
| `urlMatchesAny` | []string | Match any regex |

#### HTTP Characteristics

| Field | Type | Description |
|-------|------|-------------|
| `methods` | []string | HTTP methods (`GET`, `POST`, etc.) |
| `schemes` | []string | URI schemes (`http`, `https`) |
| `host` | string | Host header value |
| `hostAny` | []string | Match any host |
| `userAgent` | string | User-Agent substring |
| `referer` | string | Referer header value |

#### Headers, Cookies, Query Parameters

| Field | Type | Description | Format |
|-------|------|-------------|--------|
| `headers` | map[string]string | Header matching rules | `"x-api-key": "exists:"` |
| `headersAny` | map[string]string | OR header matching | `"authorization": "contains:Bearer"` |
| `cookies` | map[string]string | Cookie matching rules | `"session": "eq:abc123"` |
| `queryParams` | map[string]string | Query parameter rules | `"version": "eq:v1"` |

**Rule Format**: `operator:value`
- `eq:value` - Equals
- `contains:value` - Contains substring
- `exists:` - Header/cookie/param exists

#### Geography and Network

| Field | Type | Description |
|-------|------|-------------|
| `countries` | []string | Country codes (ISO 3166-1 alpha-2) |
| `countriesExclude` | []string | Excluded countries |
| `ipSources` | []string | IP addresses/ranges |
| `ipSourcesExclude` | []string | Excluded IP addresses/ranges |
| `asns` | []int | Autonomous System Numbers |

#### Bot Management (Enterprise)

| Field | Type | Description |
|-------|------|-------------|
| `verifiedBot` | *bool | Verified bot traffic |
| `botScore` | string | Bot score comparison (`gt:50`, `lt:30`) |
| `ja3Fingerprint` | []string | JA3 TLS fingerprints |
| `ja4Fingerprint` | []string | JA4 fingerprints |

#### Request Body Analysis (Enterprise)

| Field | Type | Description | Format |
|-------|------|-------------|--------|
| `bodySize` | string | Body size comparison | `gt:1024`, `lt:5000` |
| `bodyContains` | string | Body content substring | `malicious_payload` |
| `bodyFormFields` | map[string]string | Form field rules | `"username": "exists:"` |
| `bodyJSONFields` | map[string]string | JSON path rules | `"$.user.id": "eq:123"` |

#### Advanced

| Field | Type | Description |
|-------|------|-------------|
| `customExpression` | string | Raw Cloudflare expression |

### RateLimitMatchResponse

| Field | Type | Description |
|-------|------|-------------|
| `statusCode` | int | HTTP status code |
| `headers` | map[string]string | Response header matching |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `cloudflareId` | string | Cloudflare rule ID |
| `observedGeneration` | int64 | Last processed generation |
| `conditions` | []Condition | Status conditions |

## WAFRule

**API Version**: `cloudflare.kubeflare.io/v1alpha1`
**Kind**: `WAFRule`

Web Application Firewall rules filter and block malicious traffic.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `zoneId` | string | Yes | Zone ID or domain name |
| `description` | string | No | Human-readable description |
| `expression` | string | Yes | Cloudflare filter expression |
| `action` | string | Yes | Action: `block`, `challenge`, `js_challenge`, `managed_challenge`, `log`, `skip` |
| `priority` | int | No | Rule priority (lower = higher precedence) |
| `paused` | bool | No | Whether rule is paused |
| `actionParameters` | [WAFActionParameters](#wafactionparameters) | No | Additional action configuration |

### WAFActionParameters

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Action parameter ID |
| `overrides` | map[string]string | Rule overrides |
| `response` | [WAFActionResponse](#wafactionresponse) | Custom response |

### WAFActionResponse

| Field | Type | Description |
|-------|------|-------------|
| `statusCode` | int | HTTP status code |
| `contentType` | string | Content-Type header |
| `body` | string | Response body |

### Status Fields

Same as RateLimit status structure.

## Examples

### Basic Rate Limit

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: api-rate-limit
  namespace: production
spec:
  zoneId: "example.com"
  description: "API endpoint rate limiting"
  threshold: 100
  period: 60
  action:
    mode: "block"
  match:
    request:
      urlStartsWith: "/api/"
      methods: ["POST", "PUT", "DELETE"]
```

### Advanced Rate Limit with Bot Protection

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: RateLimit
metadata:
  name: bot-protection
spec:
  zoneId: "api.example.com"
  description: "Bot protection with geography filtering"
  threshold: 50
  period: 300
  action:
    mode: "challenge"
  match:
    request:
      urlWildcard: "/api/*"
      verifiedBot: false
      botScore: "lt:30"
      countries: ["US", "CA", "GB"]
      countriesExclude: ["CN", "RU"]
      headers:
        "x-api-key": "exists:"
        "user-agent": "contains:bot"
```

### WAF Rule

```yaml
apiVersion: cloudflare.kubeflare.io/v1alpha1
kind: WAFRule
metadata:
  name: block-sql-injection
spec:
  zoneId: "webapp.example.com"
  description: "Block SQL injection attempts"
  expression: '(http.request.uri.query contains "union select") or (http.request.body.raw contains "drop table")'
  action: "block"
  priority: 1
  actionParameters:
    response:
      statusCode: 403
      contentType: "application/json"
      body: '{"error": "Request blocked by security policy"}'
```

### Zone Resolution

Controllers resolve zone IDs via ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubeflare-zones
  namespace: kubeflare-light-system
data:
  default: "abc123def456ghi789"
  example.com: "zone-id-for-example-com"
  api.example.com: "zone-id-for-api-subdomain"
  staging.example.com: "zone-id-for-staging"
```

## Validation Rules

### RateLimit
- `threshold`: 1-1,000,000
- `period`: 10, 60, 120, 300, 600, 3600 seconds
- `action.mode`: Valid action types only
- URL patterns: No injection characters

### WAFRule
- `expression`: Valid Cloudflare expression syntax
- `action`: Valid action types only
- `priority`: 0-1000000

## Status Conditions

Both resources use standard Kubernetes conditions:

```yaml
status:
  conditions:
  - type: "Ready"
    status: "True"
    reason: "Created"
    message: "Rule created successfully in Cloudflare"
    lastTransitionTime: "2024-01-15T10:30:00Z"
```

**Condition Types**:
- `Ready` - Resource synchronized with Cloudflare

**Reasons**:
- `Created` - New rule created
- `Updated` - Existing rule updated
- `Synced` - No changes needed
- `CreateFailed` - Creation failed
- `UpdateFailed` - Update failed
- `ZoneResolutionFailed` - Zone ID lookup failed
