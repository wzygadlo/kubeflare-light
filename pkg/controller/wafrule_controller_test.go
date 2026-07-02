package controller

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
	cfclient "github.com/wzygadlo/kubeflare-light/pkg/cloudflare/client"
	"github.com/wzygadlo/kubeflare-light/pkg/utils"
)

func TestConvertToCloudflareWAFRuleWiresResponse(t *testing.T) {
	wafRule := &cloudflareV1alpha1.WAFRule{
		Spec: cloudflareV1alpha1.WAFRuleSpec{
			Action: "block",
			ActionParameters: &cloudflareV1alpha1.WAFActionParameters{
				Response: &cloudflareV1alpha1.WAFActionResponse{
					StatusCode:  403,
					ContentType: "text/plain",
					Content:     "blocked",
				},
			},
		},
	}

	rule := (&WAFRuleReconciler{}).convertToCloudflareWAFRule(wafRule)
	if rule.Response == nil {
		t.Fatal("expected Response to be set, got nil (spec.actionParameters.response was dropped)")
	}
	want := cfclient.FirewallRuleResponse{StatusCode: 403, ContentType: "text/plain", Content: "blocked"}
	if *rule.Response != want {
		t.Fatalf("Response = %+v, want %+v", *rule.Response, want)
	}
}

func TestReconcileWAFRuleRejectsResponseOnNonBlockAction(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := cloudflareV1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	wafRule := &cloudflareV1alpha1.WAFRule{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: cloudflareV1alpha1.WAFRuleSpec{
			ZoneID:     "0123456789abcdef0123456789abcdef",
			Expression: `http.request.uri.path eq "/login"`,
			Action:     "js_challenge",
			ActionParameters: &cloudflareV1alpha1.WAFActionParameters{
				Response: &cloudflareV1alpha1.WAFActionResponse{StatusCode: 403, ContentType: "text/plain", Content: "blocked"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wafRule).WithStatusSubresource(wafRule).Build()
	r := &WAFRuleReconciler{
		Client:            fakeClient,
		Log:               logr.Discard(),
		ZoneResolver:      utils.NewZoneResolver(fakeClient),
		ReconcileInterval: 10 * time.Minute,
	}

	result, err := r.reconcileWAFRule(context.Background(), wafRule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected a non-zero requeue for an invalid WAFRule, got none (resource would stop being reconciled)")
	}
	found := false
	for _, c := range wafRule.Status.Conditions {
		if c.Type == "Ready" {
			found = true
			if c.Status != metav1.ConditionFalse || c.Reason != "InvalidActionParameters" {
				t.Fatalf("condition = %+v, want Ready=False/InvalidActionParameters", c)
			}
		}
	}
	if !found {
		t.Fatal("expected a Ready condition to be set")
	}
}

func TestFirewallRuleResponsesEqual(t *testing.T) {
	a := &cfclient.FirewallRuleResponse{StatusCode: 403, ContentType: "text/plain", Content: "no"}
	b := &cfclient.FirewallRuleResponse{StatusCode: 403, ContentType: "text/plain", Content: "no"}
	if !firewallRuleResponsesEqual(a, b) {
		t.Fatal("expected equal responses to compare equal")
	}
	if !firewallRuleResponsesEqual(nil, nil) {
		t.Fatal("expected nil, nil to compare equal")
	}
	if firewallRuleResponsesEqual(a, nil) {
		t.Fatal("expected non-nil vs nil to compare unequal")
	}
	c := &cfclient.FirewallRuleResponse{StatusCode: 500, ContentType: "text/plain", Content: "no"}
	if firewallRuleResponsesEqual(a, c) {
		t.Fatal("expected differing responses to compare unequal")
	}
}
