package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WAFRuleSpec defines the desired state of WAFRule
type WAFRuleSpec struct {
	// ZoneID is the Cloudflare Zone ID
	ZoneID string `json:"zoneId"`

	// PackageID is the WAF rule package ID
	PackageID string `json:"packageId"`

	// RuleID is the WAF rule ID
	RuleID string `json:"ruleId"`

	// Mode is the action to take when the rule is triggered
	// Valid values are: "on", "off", "block", "challenge", "js_challenge", "simulate"
	Mode string `json:"mode"`

	// Description is a human-readable description of the WAF rule
	Description string `json:"description,omitempty"`
}

// WAFRuleStatus defines the observed state of WAFRule
type WAFRuleStatus struct {
	// Conditions represents the current state of the WAF rule
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Zone",type=string,JSONPath=`.spec.zoneId`
// +kubebuilder:printcolumn:name="Package",type=string,JSONPath=`.spec.packageId`
// +kubebuilder:printcolumn:name="Rule",type=string,JSONPath=`.spec.ruleId`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
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