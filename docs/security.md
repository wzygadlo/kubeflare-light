# 🛡️ Security Guide

This document outlines security considerations, best practices, and vulnerability information for kubeflare-light.

## Table of Contents

- [Security Overview](#security-overview)
- [Known Vulnerabilities](#known-vulnerabilities)
- [Security Best Practices](#security-best-practices)
- [RBAC Configuration](#rbac-configuration)
- [Network Security](#network-security)
- [Secrets Management](#secrets-management)
- [Monitoring & Auditing](#monitoring--auditing)
- [Incident Response](#incident-response)

---

## Security Overview

### Security Model

kubeflare-light operates within the Kubernetes security model and interacts with external Cloudflare APIs:

```
┌─────────────────────────────────────────────────────────┐
│                 Kubernetes Cluster                      │
│                                                         │
│  ┌─────────────────┐     ┌─────────────────────────┐   │
│  │   Users/Ops     │────▶│    RBAC Policies        │   │
│  │   Teams         │     │                         │   │
│  └─────────────────┘     └─────────────────────────┘   │
│           │                           │                 │
│           ▼                           ▼                 │
│  ┌─────────────────┐     ┌─────────────────────────┐   │
│  │   kubectl       │     │  kubeflare-light        │   │
│  │   apply CRDs    │────▶│  Controller             │   │
│  └─────────────────┘     └─────────────────────────┘   │
│                                       │                 │
│  ┌─────────────────────────────────────┼─────────────┐  │
│  │           Secrets                   │             │  │
│  │  ┌───────────────────────────────┐  │             │  │
│  │  │  Cloudflare API Token         │  │             │  │
│  │  │  (Encrypted at Rest)          │  │             │  │
│  │  └───────────────────────────────┘  │             │  │
│  └─────────────────────────────────────┼─────────────┘  │
└─────────────────────────────────────────┼─────────────────┘
                                          │
                                          ▼ TLS 1.3
┌─────────────────────────────────────────────────────────┐
│                 Cloudflare API                          │
│  ┌─────────────────┐   ┌─────────────────────────────┐ │
│  │  Authentication │   │      Zone Management        │ │
│  │  & Authorization│   │      Rule Configuration     │ │
│  └─────────────────┘   └─────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### Attack Surface

**Internal (Kubernetes)**:
- CRD input validation
- RBAC bypass attempts
- Secret exposure
- Container security

**External (Cloudflare)**:
- API token compromise
- Expression injection
- Rate limit bypass
- Zone access escalation

---

## Known Vulnerabilities

### 🚨 CRITICAL: Expression Injection (CVE-Pending)

**Severity**: High  
**Status**: ⚠️ **UNPATCHED**  
**CVSS Score**: 8.8 (High)

#### Description
The RateLimit controller directly interpolates user input into Cloudflare expressions without proper sanitization, allowing expression injection attacks.

#### Affected Code
```go
// pkg/controller/ratelimit_controller.go:197
expression = fmt.Sprintf("(http.request.uri.path wildcard \"%s\")", 
    rateLimit.Spec.Match.Request.URLPattern)  // ← VULNERABLE
```

#### Attack Vector
```yaml
# Malicious CRD
spec:
  match:
    request:
      urlPattern: '") or (true'  # Injection payload
```

**Result**: Bypasses intended rate limiting logic
```javascript
// Generated expression
(http.request.uri.path wildcard "") or (true")
```

#### Impact
- **Confidentiality**: Low (no data exposure)
- **Integrity**: High (security policy bypass)
- **Availability**: Medium (rate limit evasion)

#### Mitigation
```go
// Immediate fix needed
func sanitizeURLPattern(pattern string) (string, error) {
    // Validate pattern format
    if !regexp.MustCompile(`^[a-zA-Z0-9\/*?._:-]+$`).MatchString(pattern) {
        return "", fmt.Errorf("invalid characters in URL pattern")
    }
    
    // Escape special characters
    pattern = strings.ReplaceAll(pattern, `\`, `\\`)
    pattern = strings.ReplaceAll(pattern, `"`, `\"`)
    
    return pattern, nil
}
```

### 🔶 MEDIUM: Incomplete WAF Implementation

**Severity**: Medium  
**Status**: ⚠️ **INCOMPLETE**

#### Description
WAF rule methods contain placeholder implementations that return hardcoded data instead of making actual API calls.

#### Affected Code
```go
// pkg/cloudflare/client/client.go:419-428
func (c *CloudflareClient) CreateFirewallRule(ctx context.Context, zoneID string, rule FirewallRule) (*FirewallRule, error) {
    // TODO: Implement with correct v4.5.1 rulesets API
    return &FirewallRule{
        ID: fmt.Sprintf("fw_%s_%d", zoneID[:8], rule.Priority),
        // ... hardcoded values
    }, nil
}
```

#### Impact
- WAF rules are not actually created in Cloudflare
- False sense of security for users
- No actual protection against attacks

#### Mitigation
Complete the WAF implementation using the Rulesets API.

---

## Security Best Practices

### Input Validation

#### 1. Validate All User Input

```go
// Example validation function
func validateRateLimitSpec(spec *RateLimitSpec) error {
    // Zone ID validation
    if !isValidZoneID(spec.ZoneID) && !isValidDomain(spec.ZoneID) {
        return fmt.Errorf("invalid zone ID or domain")
    }
    
    // Threshold validation
    if spec.Threshold < 1 || spec.Threshold > 1000000 {
        return fmt.Errorf("threshold must be between 1 and 1,000,000")
    }
    
    // Period validation
    if spec.Period < 10 || spec.Period > 86400 {
        return fmt.Errorf("period must be between 10 and 86,400 seconds")
    }
    
    return nil
}
```

#### 2. Sanitize Expressions

```go
type ExpressionSanitizer struct {
    allowedFields map[string]bool
    maxLength     int
}

func (s *ExpressionSanitizer) Sanitize(expr string) (string, error) {
    if len(expr) > s.maxLength {
        return "", fmt.Errorf("expression too long")
    }
    
    // Parse and validate expression
    parsed, err := s.parseExpression(expr)
    if err != nil {
        return "", fmt.Errorf("invalid expression: %w", err)
    }
    
    // Check for allowed fields only
    for field := range parsed.Fields {
        if !s.allowedFields[field] {
            return "", fmt.Errorf("field not allowed: %s", field)
        }
    }
    
    return expr, nil
}
```

#### 3. Implement Rate Limiting

```go
// Rate limit API calls per zone
type ZoneRateLimiter struct {
    limiters map[string]*rate.Limiter
    mutex    sync.RWMutex
}

func (r *ZoneRateLimiter) Allow(zoneID string) bool {
    r.mutex.RLock()
    limiter, exists := r.limiters[zoneID]
    r.mutex.RUnlock()
    
    if !exists {
        r.mutex.Lock()
        limiter = rate.NewLimiter(rate.Every(time.Second), 10) // 10 req/sec
        r.limiters[zoneID] = limiter
        r.mutex.Unlock()
    }
    
    return limiter.Allow()
}
```

### Secure Coding Practices

#### 1. Error Handling

```go
// DON'T: Expose internal details
if err != nil {
    return fmt.Errorf("failed to call Cloudflare API: %v", err)
}

// DO: Generic error messages
if err != nil {
    log.Error(err, "Cloudflare API call failed", "operation", "CreateRateLimit")
    return fmt.Errorf("failed to create rate limit rule")
}
```

#### 2. Logging Security

```go
// DON'T: Log sensitive data
log.Info("Creating rate limit", "token", apiToken, "zoneID", zoneID)

// DO: Log safely
log.Info("Creating rate limit", "zoneID", maskZoneID(zoneID), "rule", ruleName)

func maskZoneID(zoneID string) string {
    if len(zoneID) < 8 {
        return "****"
    }
    return zoneID[:4] + "****" + zoneID[len(zoneID)-4:]
}
```

#### 3. Timeout Management

```go
// Always use timeouts for external calls
func (c *CloudflareClient) createRuleset(ctx context.Context, zoneID string) error {
    // Set reasonable timeout
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    // Make API call
    resp, err := c.api.CreateRuleset(ctx, cloudflare.ZoneIdentifier(zoneID), params)
    if err != nil {
        return err
    }
    
    return nil
}
```

---

## RBAC Configuration

### Principle of Least Privilege

#### 1. Operator Service Account

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubeflare-light-controller
  namespace: kubeflare-light-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubeflare-light-controller
rules:
# Only necessary permissions
- apiGroups: ["cloudflare.kubeflare.io"]
  resources: ["ratelimits", "wafrules"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["cloudflare.kubeflare.io"]
  resources: ["ratelimits/status", "wafrules/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

#### 2. User Access Control

**Production Environment:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: production
  name: cloudflare-admin
rules:
- apiGroups: ["cloudflare.kubeflare.io"]
  resources: ["ratelimits", "wafrules"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: production
  name: cloudflare-viewer
rules:
- apiGroups: ["cloudflare.kubeflare.io"]
  resources: ["ratelimits", "wafrules"]
  verbs: ["get", "list", "watch"]
```

**Development Environment:**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cloudflare-dev
rules:
- apiGroups: ["cloudflare.kubeflare.io"]
  resources: ["*"]
  verbs: ["*"]
  resourceNames: [] # Restrict to specific resources if needed
```

### Namespace Isolation

```yaml
# Separate namespaces for different environments
apiVersion: v1
kind: Namespace
metadata:
  name: cloudflare-production
  labels:
    security.policy: "strict"
    network.policy: "isolated"
---
apiVersion: v1
kind: Namespace
metadata:
  name: cloudflare-staging
  labels:
    security.policy: "moderate"
---
apiVersion: v1
kind: Namespace
metadata:
  name: cloudflare-development
  labels:
    security.policy: "permissive"
```

---

## Network Security

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubeflare-light-controller
  namespace: kubeflare-light-system
spec:
  podSelector:
    matchLabels:
      app: kubeflare-light-controller
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8080  # Metrics
    - protocol: TCP
      port: 8081  # Health checks
  egress:
  - to: []  # Kubernetes API
    ports:
    - protocol: TCP
      port: 443
  - to: []  # Cloudflare API
    ports:
    - protocol: TCP
      port: 443
```

### TLS Configuration

```yaml
# Ensure TLS for all communications
apiVersion: v1
kind: ConfigMap
metadata:
  name: tls-config
data:
  tls.conf: |
    minimum_tls_version = "1.2"
    ciphers = [
      "TLS_AES_256_GCM_SHA384",
      "TLS_CHACHA20_POLY1305_SHA256",
      "TLS_AES_128_GCM_SHA256"
    ]
```

---

## Secrets Management

### API Token Storage

#### 1. Kubernetes Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-api-token
  namespace: kubeflare-light-system
type: Opaque
data:
  token: <base64-encoded-token>
```

#### 2. External Secret Management

**Using External Secrets Operator:**
```yaml
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
spec:
  provider:
    vault:
      server: "https://vault.company.com"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "kubeflare-light"
---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: cloudflare-token
spec:
  refreshInterval: 15m
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: cloudflare-api-token
    creationPolicy: Owner
  data:
  - secretKey: token
    remoteRef:
      key: cloudflare
      property: api-token
```

#### 3. Token Rotation

```go
// Implement token rotation monitoring
type TokenManager struct {
    client     kubernetes.Interface
    secretName string
    namespace  string
    rotationInterval time.Duration
}

func (tm *TokenManager) MonitorToken(ctx context.Context) error {
    ticker := time.NewTicker(tm.rotationInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := tm.checkTokenExpiry(); err != nil {
                log.Error(err, "Token expiry check failed")
            }
        }
    }
}
```

### Secret Security

#### 1. Encryption at Rest

Ensure etcd encryption is enabled:
```yaml
apiVersion: apiserver.k8s.io/v1
kind: EncryptionConfiguration
resources:
- resources:
  - secrets
  providers:
  - aescbc:
      keys:
      - name: key1
        secret: <base64-encoded-key>
  - identity: {}
```

#### 2. Secret Access Logging

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: Metadata
  resources:
  - group: ""
    resources: ["secrets"]
  namespaces: ["kubeflare-light-system"]
```

---

## Monitoring & Auditing

### Security Monitoring

#### 1. Prometheus Metrics

```go
// Security-related metrics
var (
    apiCallsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubeflare_api_calls_total",
            Help: "Total number of Cloudflare API calls",
        },
        []string{"operation", "status", "zone_id"},
    )
    
    authFailuresTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubeflare_auth_failures_total",
            Help: "Total number of authentication failures",
        },
        []string{"reason"},
    )
    
    invalidInputTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubeflare_invalid_input_total",
            Help: "Total number of invalid input attempts",
        },
        []string{"field", "reason"},
    )
)
```

#### 2. Alerting Rules

```yaml
# Prometheus alerting rules
groups:
- name: kubeflare-security
  rules:
  - alert: HighAuthFailureRate
    expr: rate(kubeflare_auth_failures_total[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High authentication failure rate detected"
      
  - alert: SuspiciousInputPattern
    expr: rate(kubeflare_invalid_input_total[5m]) > 1
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Potential injection attack detected"
      
  - alert: APIRateLimitExceeded
    expr: kubeflare_api_calls_total{status="rate_limited"} > 0
    for: 0m
    labels:
      severity: warning
    annotations:
      summary: "Cloudflare API rate limit exceeded"
```

### Audit Logging

#### 1. Kubernetes Audit

```yaml
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
# Audit all CRD operations
- level: RequestResponse
  resources:
  - group: "cloudflare.kubeflare.io"
    resources: ["*"]
    
# Audit secret access
- level: Metadata
  resources:
  - group: ""
    resources: ["secrets"]
  namespaces: ["kubeflare-light-system"]
  
# Audit RBAC changes
- level: RequestResponse
  resources:
  - group: "rbac.authorization.k8s.io"
    resources: ["*"]
```

#### 2. Application Audit

```go
// Audit log structure
type AuditEvent struct {
    Timestamp   time.Time `json:"timestamp"`
    User        string    `json:"user"`
    Action      string    `json:"action"`
    Resource    string    `json:"resource"`
    Success     bool      `json:"success"`
    Reason      string    `json:"reason,omitempty"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (a *Auditor) LogEvent(ctx context.Context, event AuditEvent) {
    event.Timestamp = time.Now()
    
    // Add request context
    if user, ok := ctx.Value("user").(string); ok {
        event.User = user
    }
    
    // Log to structured logger
    log.Info("Audit event", "event", event)
    
    // Send to SIEM if configured
    if a.siemEndpoint != "" {
        a.sendToSIEM(event)
    }
}
```

---

## Incident Response

### Security Incident Playbook

#### 1. Expression Injection Detected

**Detection Indicators:**
- Unusual expression patterns in logs
- Unexpected rate limit behavior
- Failed expression validation

**Immediate Response:**
1. **Isolate**: Disable affected rate limit rules
2. **Investigate**: Review audit logs and CRD changes
3. **Contain**: Implement emergency expression validation
4. **Communicate**: Notify security team and stakeholders

**Recovery Steps:**
1. Apply expression sanitization patch
2. Review all existing expressions
3. Implement admission webhooks
4. Update monitoring rules

#### 2. API Token Compromise

**Detection Indicators:**
- Unexpected API calls from unknown sources
- Rate limiting from Cloudflare
- Unauthorized zone modifications

**Immediate Response:**
1. **Revoke**: Immediately revoke compromised token
2. **Generate**: Create new API token with minimal permissions
3. **Update**: Update Kubernetes secrets
4. **Monitor**: Watch for continued unauthorized activity

**Recovery Steps:**
1. Audit all recent zone changes
2. Implement token rotation
3. Review access logs
4. Update security procedures

#### 3. Container Security Issue

**Detection Indicators:**
- Unusual container behavior
- Unexpected network connections
- File system modifications

**Immediate Response:**
1. **Stop**: Stop affected pods
2. **Isolate**: Network isolation
3. **Preserve**: Capture forensic data
4. **Replace**: Deploy clean images

### Emergency Contacts

```yaml
# Emergency response contacts
security_team:
  primary: "security@company.com"
  secondary: "incident-response@company.com"
  
cloudflare_support:
  enterprise: "support@cloudflare.com"
  phone: "+1-xxx-xxx-xxxx"
  
on_call:
  pagerduty: "https://company.pagerduty.com"
  slack: "#security-incidents"
```

---

## Security Checklist

### Pre-Deployment

- [ ] Input validation implemented
- [ ] Expression sanitization added
- [ ] RBAC policies configured
- [ ] Secrets properly managed
- [ ] Network policies applied
- [ ] Monitoring configured
- [ ] Audit logging enabled

### Regular Security Review

- [ ] Review audit logs weekly
- [ ] Update dependencies monthly
- [ ] Rotate API tokens quarterly
- [ ] Security assessment annually
- [ ] Penetration testing annually
- [ ] Incident response drill quarterly

### Compliance

- [ ] GDPR compliance (if applicable)
- [ ] SOC 2 requirements (if applicable)
- [ ] Industry-specific requirements
- [ ] Internal security policies
- [ ] Change management process

---

This security guide provides comprehensive coverage of security considerations for kubeflare-light. Regular review and updates are essential as threats evolve and new vulnerabilities are discovered.