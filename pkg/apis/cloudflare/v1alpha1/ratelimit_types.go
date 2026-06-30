package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RateLimitSpec defines the desired state of RateLimit
type RateLimitSpec struct {
	// ZoneID is the Cloudflare Zone ID
	ZoneID string `json:"zoneId"`

	// Description is a human-readable description of the rate limit
	Description string `json:"description,omitempty"`

	// Threshold is the number of requests in the specified period that will trigger the rate limit action
	Threshold int `json:"threshold"`

	// Period is the time in seconds to count matching requests
	Period int `json:"period"`

	// Action is the action to perform when the threshold is reached
	// +optional
	Action *RateLimitAction `json:"action,omitempty"`

	// Match defines the characteristics of requests that will be counted towards the threshold
	// +optional
	Match *RateLimitMatch `json:"match,omitempty"`

	// Characteristics defines how Cloudflare tracks the request rate.
	// Determines how requests are grouped for rate limiting (e.g., per IP, per JA3 fingerprint, etc.)
	// Available values: "ip.src", "cf.unique_visitor_id", "cf.bot_management.ja3_hash", "cf.bot_management.ja4",
	// "http.request.headers[\"<name>\"]", "http.request.cookies[\"<name>\"]", "http.host", "ip.src.asnum", etc.
	// Default: ["ip.src", "cf.colo.id"] if not specified.
	// Note: "cf.colo.id" is always included automatically by Cloudflare.
	// +optional
	Characteristics []string `json:"characteristics,omitempty"`

	// Disabled indicates whether this rate limit is currently disabled
	// +optional
	Disabled bool `json:"disabled,omitempty"`
}

// RateLimitAction defines the action to take when the rate limit is triggered
type RateLimitAction struct {
	// Mode is the type of action to perform
	// Valid values are: "log", "block", "challenge", "js_challenge", "managed_challenge"
	Mode string `json:"mode"`

	// Timeout is the time in seconds for the action to be in effect
	// Required for "ban" or "block" mode
	// +optional
	Timeout int `json:"timeout,omitempty"`

	// Response is a custom response to be sent
	// +optional
	Response *RateLimitActionResponse `json:"response,omitempty"`
}

// RateLimitActionResponse defines a custom response when the rate limit is triggered
type RateLimitActionResponse struct {
	// ContentType is the Content-Type header value
	ContentType string `json:"contentType"`

	// Body is the response body to return
	Body string `json:"body"`
}

// RateLimitMatch defines the characteristics of requests that will be counted
type RateLimitMatch struct {
	// Request defines the request characteristics to match
	// +optional
	Request *RateLimitMatchRequest `json:"request,omitempty"`
	// Response defines the response characteristics to match
	// +optional
	Response *RateLimitMatchResponse `json:"response,omitempty"`
}

// RateLimitMatchRequest defines the request characteristics to match
type RateLimitMatchRequest struct {
	// Methods is a list of HTTP methods to match
	// +optional
	Methods []string `json:"methods,omitempty"`

	// Schemes is a list of HTTP schemes to match
	// +optional
	Schemes []string `json:"schemes,omitempty"`

	// URLPattern is a URL pattern to match (legacy - use specific operators below)
	// +optional
	URLPattern string `json:"urlPattern,omitempty"`

	// === URL Operators (All Cloudflare Plans) ===

	// URLWildcard matches paths using wildcard pattern (case-insensitive, free plan compatible)
	// Example: "/api/*" matches "/api/users", "/api/orders"
	// +optional
	URLWildcard string `json:"urlWildcard,omitempty"`

	// URLStartsWith matches paths starting with prefix (starts_with function)
	// Example: "/api" matches "/api/users", "/api/v1/orders"
	// +optional
	URLStartsWith string `json:"urlStartsWith,omitempty"`

	// URLEndsWith matches paths ending with suffix (ends_with function)
	// Example: ".json" matches "/api/users.json", "/data.json"
	// +optional
	URLEndsWith string `json:"urlEndsWith,omitempty"`

	// URLContains matches paths containing substring (contains function)
	// Example: "admin" matches "/user/admin/panel", "/admin/settings"
	// +optional
	URLContains string `json:"urlContains,omitempty"`

	// URLEquals matches exact path (eq operator)
	// Example: "/health" matches only "/health"
	// +optional
	URLEquals string `json:"urlEquals,omitempty"`

	// URLMatches matches paths using regex (requires paid plan)
	// Example: "^/api/v[0-9]+/" matches "/api/v1/", "/api/v2/"
	// +optional
	URLMatches string `json:"urlMatches,omitempty"`

	// === Multiple Pattern Arrays (OR conditions) ===

	// URLWildcardAny matches paths using any of the wildcard patterns
	// +optional
	URLWildcardAny []string `json:"urlWildcardAny,omitempty"`

	// URLStartsWithAny matches paths starting with any of the prefixes
	// +optional
	URLStartsWithAny []string `json:"urlStartsWithAny,omitempty"`

	// URLEndsWithAny matches paths ending with any of the suffixes
	// +optional
	URLEndsWithAny []string `json:"urlEndsWithAny,omitempty"`

	// URLContainsAny matches paths containing any of the substrings
	// +optional
	URLContainsAny []string `json:"urlContainsAny,omitempty"`

	// URLEqualsAny matches any of the exact paths
	// +optional
	URLEqualsAny []string `json:"urlEqualsAny,omitempty"`

	// URLMatchesAny matches paths using any of the regex patterns (requires paid plan)
	// +optional
	URLMatchesAny []string `json:"urlMatchesAny,omitempty"`

	// === Header Matching ===

	// Headers matches specific header values
	// Key: header name (lowercase), Value: matching rule
	// Examples: {"user-agent": "contains:bot", "x-api-key": "eq:secret123"}
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// HeadersAny matches if any header rule matches (OR condition)
	// +optional
	HeadersAny map[string]string `json:"headersAny,omitempty"`

	// Host matches the Host header value
	// +optional
	Host string `json:"host,omitempty"`

	// HostAny matches any of the Host header values
	// +optional
	HostAny []string `json:"hostAny,omitempty"`

	// UserAgent matches User-Agent header
	// +optional
	UserAgent string `json:"userAgent,omitempty"`

	// Referer matches Referer header
	// +optional
	Referer string `json:"referer,omitempty"`

	// === Cookie Matching ===

	// Cookies matches specific cookie values
	// Key: cookie name, Value: matching rule
	// Examples: {"session": "exists", "user_id": "eq:123"}
	// +optional
	Cookies map[string]string `json:"cookies,omitempty"`

	// === Query Parameter Matching ===

	// QueryParams matches specific query parameter values
	// Key: parameter name, Value: matching rule
	// Examples: {"debug": "eq:true", "version": "contains:v1"}
	// +optional
	QueryParams map[string]string `json:"queryParams,omitempty"`

	// === IP and Geography ===

	// IPSources matches specific IP addresses or CIDR ranges
	// Examples: ["192.168.1.1", "10.0.0.0/8"]
	// +optional
	IPSources []string `json:"ipSources,omitempty"`

	// IPSourcesExclude excludes specific IP addresses or CIDR ranges
	// +optional
	IPSourcesExclude []string `json:"ipSourcesExclude,omitempty"`

	// Countries matches requests from specific countries (ISO 3166-1 alpha-2)
	// Examples: ["US", "CA", "GB"]
	// +optional
	Countries []string `json:"countries,omitempty"`

	// CountriesExclude excludes requests from specific countries
	// +optional
	CountriesExclude []string `json:"countriesExclude,omitempty"`

	// ASNs matches requests from specific Autonomous System Numbers
	// +optional
	ASNs []int `json:"asns,omitempty"`

	// === Request Body (Enterprise Plans) ===

	// BodySize matches request body size
	// Examples: "gt:1024", "le:10240"
	// +optional
	BodySize string `json:"bodySize,omitempty"`

	// BodyContains matches if request body contains text
	// +optional
	BodyContains string `json:"bodyContains,omitempty"`

	// BodyFormFields matches form field values
	// Key: field name, Value: matching rule
	// +optional
	BodyFormFields map[string]string `json:"bodyFormFields,omitempty"`

	// BodyJSONFields matches JSON field values using JSONPath
	// Key: JSON path, Value: matching rule
	// Examples: {"user.id": "eq:123", "order.total": "gt:100"}
	// +optional
	BodyJSONFields map[string]string `json:"bodyJSONFields,omitempty"`

	// === Bot Management (Enterprise Plans) ===

	// BotScore matches requests with bot score in range
	// Examples: "gt:30", "le:80"
	// +optional
	BotScore string `json:"botScore,omitempty"`

	// VerifiedBot matches verified bot status
	// +optional
	VerifiedBot *bool `json:"verifiedBot,omitempty"`

	// JA3Fingerprint matches specific JA3 TLS fingerprints
	// +optional
	JA3Fingerprint []string `json:"ja3Fingerprint,omitempty"`

	// JA4Fingerprint matches specific JA4 TLS fingerprints
	// +optional
	JA4Fingerprint []string `json:"ja4Fingerprint,omitempty"`

	// === Custom Expression ===

	// CustomExpression allows writing raw Cloudflare expressions
	// Use this for complex logic not covered by above fields
	// +optional
	CustomExpression string `json:"customExpression,omitempty"`
}

// RateLimitMatchResponse defines the response characteristics to match
type RateLimitMatchResponse struct {
	// Statuses is a list of HTTP status codes to match
	// +optional
	Statuses []int `json:"statuses,omitempty"`

	// OriginTraffic indicates whether to match responses from origin servers
	// +optional
	OriginTraffic bool `json:"originTraffic,omitempty"`
}

// RateLimitStatus defines the observed state of RateLimit
type RateLimitStatus struct {
	// CloudflareID is the Cloudflare Rate Limit ID
	CloudflareID string `json:"cloudflareId,omitempty"`

	// ObservedGeneration is the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the current state of the rate limit
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zoneId`
// +kubebuilder:printcolumn:name="Threshold",type=integer,JSONPath=`.spec.threshold`
// +kubebuilder:printcolumn:name="Period",type=integer,JSONPath=`.spec.period`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// RateLimit is the Schema for the ratelimits API
type RateLimit struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RateLimitSpec   `json:"spec,omitempty"`
	Status RateLimitStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RateLimitList contains a list of RateLimit
type RateLimitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RateLimit `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RateLimit{}, &RateLimitList{})
}
