# Cloudflare Expressions Reference Guide

This reference guide provides common expressions and patterns you can use when creating WAF Rules and Rate Limit Rules with Kubeflare Light.

## Cloudflare Expression Language

Cloudflare uses a filter expression language similar to Wireshark for defining WAF and Rate Limit rules. This guide provides common patterns and examples to help you create effective rules.

## Common Fields

| Field | Description | Example |
|-------|-------------|---------|
| `http.request.uri.path` | The URL path | `http.request.uri.path eq "/admin"` |
| `http.request.uri.query` | The query string | `http.request.uri.query contains "debug=true"` |
| `http.request.method` | The HTTP method | `http.request.method eq "POST"` |
| `http.request.headers` | HTTP headers | `http.request.headers["user-agent"] contains "bot"` |
| `http.host` | The Host header | `http.host eq "api.example.com"` |
| `ip.src` | The source IP address | `ip.src eq 192.0.2.1` |
| `ip.geoip.country` | The country code | `ip.geoip.country eq "US"` |
| `cf.threat_score` | Cloudflare threat score (0-100) | `cf.threat_score gt 50` |

## Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `eq` | Equals | `http.request.method eq "GET"` |
| `ne` | Not equals | `http.request.method ne "GET"` |
| `lt` | Less than | `cf.threat_score lt 10` |
| `le` | Less than or equal | `cf.threat_score le 10` |
| `gt` | Greater than | `cf.threat_score gt 50` |
| `ge` | Greater than or equal | `cf.threat_score ge 50` |
| `contains` | String contains | `http.request.uri.path contains "wp-admin"` |
| `matches` | Regex match | `http.request.uri.path matches "^/api/v[0-9]+"` |
| `in` | In a set | `ip.src in {192.0.2.1 192.0.2.2}` |
| `and` | Logical AND | `http.host eq "api.example.com" and http.request.method eq "POST"` |
| `or` | Logical OR | `ip.src eq 192.0.2.1 or ip.src eq 192.0.2.2` |
| `not` | Logical NOT | `not http.request.method eq "GET"` |

## WAF Rule Examples

### Block Known Bad IPs

```
ip.src in {192.0.2.1 192.0.2.2 198.51.100.0/24}
```

### Block SQL Injection Attempts

```
http.request.uri.query contains "SELECT" or http.request.uri.query contains "UNION" or http.request.uri.query contains "DROP"
```

### Block Access to Admin Pages Except from Specific IPs

```
http.request.uri.path contains "/admin" and not ip.src in {192.0.2.10 192.0.2.11}
```

### Block Suspicious User Agents

```
(http.request.headers["user-agent"] contains "bot") or (http.request.headers["user-agent"] contains "crawler") or (http.request.headers["user-agent"] eq "")
```

### Challenge High Threat Score

```
cf.threat_score gt 30
```

### Block Requests from Specific Countries

```
ip.geoip.country in {"RU" "CN" "IR"}
```

## Rate Limit Examples

### Limit API Requests

```
http.request.uri.path contains "/api/"
```

### Limit Login Attempts

```
http.request.uri.path eq "/login" and http.request.method eq "POST"
```

### Limit Requests by User Agent

```
http.request.headers["user-agent"] contains "python-requests"
```

### Limit by IP and Path

```
http.request.uri.path contains "/wp-login.php"
```

### Limit Failed Login Attempts

```
http.request.uri.path eq "/login" and http.response.status eq 401
```

## Combining Expressions

You can create complex rules by combining expressions:

### Example 1: Block Admin Access from Outside Office

```
http.request.uri.path contains "/admin" and not ip.src in {192.0.2.0/24 198.51.100.0/24}
```

### Example 2: Challenge Suspicious Requests

```
(cf.threat_score gt 10) and (http.request.method eq "POST") and (http.host eq "api.example.com")
```

### Example 3: Complex Rate Limiting

```
(http.request.uri.path contains "/api/") and (http.request.method in {"POST" "PUT" "DELETE"})
```

## Common Use Cases

### Web Application Firewall (WAF)

1. **Block vulnerability scanners**: 
   ```
   http.request.headers["user-agent"] contains "nmap" or http.request.headers["user-agent"] contains "nikto"
   ```

2. **Protect against path traversal**:
   ```
   http.request.uri.path contains "../" or http.request.uri.path contains "..%2f"
   ```

3. **Block XML/JSON attacks**:
   ```
   (http.request.headers["content-type"] contains "application/json" and http.request.body contains "\"constructor\"") or (http.request.headers["content-type"] contains "application/xml" and http.request.body contains "<!ENTITY")
   ```

### Rate Limiting

1. **Protect login endpoints**:
   ```
   http.request.uri.path eq "/login" or http.request.uri.path eq "/wp-login.php" or http.request.uri.path eq "/admin/login"
   ```

2. **Limit expensive API operations**:
   ```
   http.request.uri.path matches "^/api/.*/(export|report|search)"
   ```

3. **Protect against credential stuffing**:
   ```
   http.request.uri.path eq "/login" and http.response.status eq 401
   ```

## Testing Expressions

You can test your expressions in the Cloudflare dashboard before implementing them in Kubeflare Light:

1. Log in to your Cloudflare dashboard
2. Go to Security > WAF > Custom rules
3. Create a new rule with the "Log" action to test your expression
4. Review the logs to verify your expression matches the expected traffic

## Additional Resources

- [Cloudflare Firewall Rules Language](https://developers.cloudflare.com/firewall/cf-firewall-language/)
- [Cloudflare Expression Reference](https://developers.cloudflare.com/ruleset-engine/rules-language/fields/)
- [Cloudflare Rate Limiting Documentation](https://developers.cloudflare.com/waf/rate-limiting-rules/)