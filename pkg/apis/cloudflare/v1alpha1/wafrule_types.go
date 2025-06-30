package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WAFRuleSpec defines the desired state of WAFRule
type WAFRuleSpec struct {
	// ZoneID is the Cloudflare Zone ID
	ZoneID string `json:"zoneId"`

	// Description is a human-readable description of the WAF rule
	Description string `json:"description,omitempty"`

	// Expression is the filter expression that matches traffic
	Expression string `json:"expression"`

	// Action is the action to take when the rule is triggered
	// Valid values are: "block", "challenge", "js_challenge", "managed_challenge", "log", "skip"
	Action string `json:"action"`

	// Paused indicates whether this WAF rule is currently disabled
	Paused bool `json:"paused,omitempty"`

	// Priority defines the priority of the rule
	Priority int `json:"priority,omitempty"`

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
	// CloudflareID is the Cloudflare WAF Rule ID
	CloudflareID string `json:"cloudflareId,omitempty"`

	// ObservedGeneration is the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represents the current state of the WAF rule
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zoneId`
// +kubebuilder:printcolumn:name="Action",type=string,JSONPath=`.spec.action`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.spec.paused`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// WAFRule is the Schema for the wafrules API
type WAFRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WAFRuleSpec   `json:"spec,omitempty"`
	Status WAFRuleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WAFRuleList contains a list of WAFRule
type WAFRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WAFRule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WAFRule{}, &WAFRuleList{})
}
