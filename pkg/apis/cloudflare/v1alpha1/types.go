package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RateLimit represents a Cloudflare rate limiting rule
type RateLimit struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RateLimitSpec   `json:"spec,omitempty"`
	Status RateLimitStatus `json:"status,omitempty"`
}

// RateLimitSpec defines the desired state of RateLimit
type RateLimitSpec struct {
	// ZoneID is the Cloudflare zone identifier
	ZoneID string `json:"zoneId"`

	// Name is a descriptive name for the rate limit rule
	Name string `json:"name"`

	// Description provides additional context for the rate limit rule
	Description string `json:"description,omitempty"`

	// Match defines the criteria for triggering the rate limit
	Match RateLimitMatch `json:"match"`

	// Threshold defines the rate limit threshold
	Threshold int `json:"threshold"`

	// Period defines the time window for the rate limit in seconds
	Period int `json:"period"`

	// Action defines what action to take when the rate limit is exceeded
	Action RateLimitAction `json:"action"`

	// Disabled indicates if the rate limit rule is disabled
	Disabled bool `json:"disabled,omitempty"`
}

// RateLimitMatch defines the matching criteria for a rate limit rule
type RateLimitMatch struct {
	// Request defines request-based matching criteria
	Request RateLimitRequest `json:"request,omitempty"`

	// Response defines response-based matching criteria
	Response RateLimitResponse `json:"response,omitempty"`
}

// RateLimitRequest defines request matching criteria
type RateLimitRequest struct {
	// Methods specifies HTTP methods to match
	Methods []string `json:"methods,omitempty"`

	// Schemes specifies URL schemes to match (http, https)
	Schemes []string `json:"schemes,omitempty"`

	// URLPattern specifies URL pattern to match
	URLPattern string `json:"urlPattern,omitempty"`
}

// RateLimitResponse defines response matching criteria
type RateLimitResponse struct {
	// StatusCodes specifies HTTP status codes to match
	StatusCodes []int `json:"statusCodes,omitempty"`

	// Headers specifies response headers to match
	Headers map[string]string `json:"headers,omitempty"`
}

// RateLimitAction defines the action to take when rate limit is exceeded
type RateLimitAction struct {
	// Mode specifies the action mode (simulate, ban, challenge, js_challenge)
	Mode string `json:"mode"`

	// Timeout specifies the timeout duration in seconds for ban action
	Timeout int `json:"timeout,omitempty"`

	// Response defines custom response for the action
	Response *RateLimitActionResponse `json:"response,omitempty"`
}

// RateLimitActionResponse defines custom response for rate limit actions
type RateLimitActionResponse struct {
	// ContentType specifies the content type of the response
	ContentType string `json:"contentType,omitempty"`

	// Body specifies the response body
	Body string `json:"body,omitempty"`
}

// RateLimitStatus defines the observed state of RateLimit
type RateLimitStatus struct {
	// CloudflareID is the ID of the rate limit rule in Cloudflare
	CloudflareID string `json:"cloudflareId,omitempty"`

	// Conditions represent the latest available observations of the rate limit's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed RateLimit
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RateLimitList contains a list of RateLimit
type RateLimitList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RateLimit `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WAFRule represents a Cloudflare WAF rule
type WAFRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WAFRuleSpec   `json:"spec,omitempty"`
	Status WAFRuleStatus `json:"status,omitempty"`
}

// WAFRuleSpec defines the desired state of WAFRule
type WAFRuleSpec struct {
	// ZoneID is the Cloudflare zone identifier
	ZoneID string `json:"zoneId"`

	// Name is a descriptive name for the WAF rule
	Name string `json:"name"`

	// Description provides additional context for the WAF rule
	Description string `json:"description,omitempty"`

	// Expression is the Wireshark-like filter expression
	Expression string `json:"expression"`

	// Action defines what action to take when the rule matches
	Action string `json:"action"`

	// Priority defines the rule priority (lower numbers are higher priority)
	Priority int `json:"priority,omitempty"`

	// Paused indicates if the rule is paused
	Paused bool `json:"paused,omitempty"`

	// ActionParameters defines additional parameters for the action
	ActionParameters *WAFActionParameters `json:"actionParameters,omitempty"`
}

// WAFActionParameters defines additional parameters for WAF rule actions
type WAFActionParameters struct {
	// ID specifies the action parameter ID
	ID string `json:"id,omitempty"`

	// Overrides specifies rule overrides
	Overrides map[string]interface{} `json:"overrides,omitempty"`

	// Response defines custom response for block actions
	Response *WAFActionResponse `json:"response,omitempty"`
}

// WAFActionResponse defines custom response for WAF rule actions
type WAFActionResponse struct {
	// StatusCode specifies the HTTP status code
	StatusCode int `json:"statusCode,omitempty"`

	// ContentType specifies the content type of the response
	ContentType string `json:"contentType,omitempty"`

	// Content specifies the response content
	Content string `json:"content,omitempty"`
}

// WAFRuleStatus defines the observed state of WAFRule
type WAFRuleStatus struct {
	// CloudflareID is the ID of the WAF rule in Cloudflare
	CloudflareID string `json:"cloudflareId,omitempty"`

	// Conditions represent the latest available observations of the WAF rule's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed WAFRule
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// WAFRuleList contains a list of WAFRule
type WAFRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WAFRule `json:"items"`
}
