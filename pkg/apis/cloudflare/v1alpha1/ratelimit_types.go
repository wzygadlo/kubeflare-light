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
	Action RateLimitAction `json:"action"`

	// Match defines the characteristics of requests that will be counted towards the threshold
	Match RateLimitMatch `json:"match,omitempty"`

	// Disabled indicates whether this rate limit is currently disabled
	Disabled bool `json:"disabled,omitempty"`
}

// RateLimitAction defines the action to take when the rate limit is triggered
type RateLimitAction struct {
	// Mode is the type of action to perform
	// Valid values are: "simulate", "ban", "challenge", "js_challenge", "managed_challenge"
	Mode string `json:"mode"`

	// Timeout is the time in seconds for the action to be in effect
	// Required for "ban" mode
	Timeout int `json:"timeout,omitempty"`

	// Response is a custom response to be sent
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
	Request *RateLimitMatchRequest `json:"request,omitempty"`
}

// RateLimitMatchRequest defines the request characteristics to match
type RateLimitMatchRequest struct {
	// Methods is a list of HTTP methods to match
	Methods []string `json:"methods,omitempty"`

	// Schemes is a list of HTTP schemes to match
	Schemes []string `json:"schemes,omitempty"`

	// URLPattern is a URL pattern to match
	URLPattern string `json:"urlPattern,omitempty"`
}

// RateLimitStatus defines the observed state of RateLimit
type RateLimitStatus struct {
	// ID is the Cloudflare Rate Limit ID
	ID string `json:"id,omitempty"`

	// Conditions represents the current state of the rate limit
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zoneId`
// +kubebuilder:printcolumn:name="Threshold",type=integer,JSONPath=`.spec.threshold`
// +kubebuilder:printcolumn:name="Period",type=integer,JSONPath=`.spec.period`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.action.mode`
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