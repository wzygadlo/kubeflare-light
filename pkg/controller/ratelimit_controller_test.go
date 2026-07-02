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
	"github.com/wzygadlo/kubeflare-light/pkg/utils"
)

func TestBuildCloudflareExpressionRejectsEmptyRequest(t *testing.T) {
	expr, err := (&RateLimitReconciler{}).buildCloudflareExpression(&cloudflareV1alpha1.RateLimitMatchRequest{})
	if err == nil {
		t.Fatal("expected error for empty match request")
	}
	if expr != "" {
		t.Fatalf("expected empty expression, got %q", expr)
	}
}

func TestBuildCloudflareExpressionWrapsCustomExpression(t *testing.T) {
	expr, err := (&RateLimitReconciler{}).buildCloudflareExpression(&cloudflareV1alpha1.RateLimitMatchRequest{
		CustomExpression: `http.request.method eq "GET"`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `(http.request.method eq "GET")`; expr != want {
		t.Fatalf("expression = %q, want %q", expr, want)
	}
}

func TestNormalizeRateLimitActionMapsLegacySimulateToLog(t *testing.T) {
	if got := normalizeRateLimitAction("simulate"); got != "log" {
		t.Fatalf("action = %q, want log", got)
	}
	if got := normalizeRateLimitAction("block"); got != "block" {
		t.Fatalf("action = %q, want block", got)
	}
}

func TestBuildCloudflareExpressionRejectsMalformedPredicate(t *testing.T) {
	_, err := (&RateLimitReconciler{}).buildCloudflareExpression(&cloudflareV1alpha1.RateLimitMatchRequest{
		URLPattern: "/api/*",
		Headers:    map[string]string{"x-api-key": "equals:secret"}, // "equals" is not a supported operator
	})
	if err == nil {
		t.Fatal("expected error for header rule with unsupported operator, got nil (predicate would be silently dropped)")
	}
}

func TestBuildCloudflareExpressionAcceptsBareExists(t *testing.T) {
	// The CRD's own doc comment documents {"session": "exists"} with no colon.
	expr, err := (&RateLimitReconciler{}).buildCloudflareExpression(&cloudflareV1alpha1.RateLimitMatchRequest{
		Cookies: map[string]string{"session": "exists"},
	})
	if err != nil {
		t.Fatalf("unexpected error for documented bare \"exists\" syntax: %v", err)
	}
	if want := `(http.cookie["session"] ne "")`; expr != want {
		t.Fatalf("expression = %q, want %q", expr, want)
	}
}

func TestBuildCloudflareExpressionWrapsCustomExpressionWithOr(t *testing.T) {
	expr, err := (&RateLimitReconciler{}).buildCloudflareExpression(&cloudflareV1alpha1.RateLimitMatchRequest{
		URLPattern:       "/login*",
		CustomExpression: `(ip.src eq 1.2.3.4) or (ip.src eq 5.6.7.8)`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `(http.request.uri wildcard "/login*") and ((ip.src eq 1.2.3.4) or (ip.src eq 5.6.7.8))`
	if expr != want {
		t.Fatalf("expression = %q, want %q (top-level \"or\" must stay inside the AND conjunction)", expr, want)
	}
}

func TestReconcileRateLimitRejectsNilMatch(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := cloudflareV1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}

	rateLimit := &cloudflareV1alpha1.RateLimit{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: cloudflareV1alpha1.RateLimitSpec{
			ZoneID:    "0123456789abcdef0123456789abcdef", // 32 hex chars: resolves as-is, no ConfigMap lookup
			Threshold: 100,
			Period:    60,
			// Match intentionally omitted.
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rateLimit).WithStatusSubresource(rateLimit).Build()
	r := &RateLimitReconciler{
		Client:            fakeClient,
		Log:               logr.Discard(),
		ZoneResolver:      utils.NewZoneResolver(fakeClient),
		ReconcileInterval: 10 * time.Minute,
	}

	result, err := r.reconcileRateLimit(context.Background(), rateLimit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected a non-zero requeue for an invalid RateLimit, got none (resource would stop being reconciled)")
	}
	found := false
	for _, c := range rateLimit.Status.Conditions {
		if c.Type == "Ready" {
			found = true
			if c.Status != metav1.ConditionFalse || c.Reason != "InvalidMatch" {
				t.Fatalf("condition = %+v, want Ready=False/InvalidMatch", c)
			}
		}
	}
	if !found {
		t.Fatal("expected a Ready condition to be set")
	}
}

func TestUpdateConditionSkipsIdenticalCondition(t *testing.T) {
	wantTime := metav1.Unix(1, 0)
	rateLimit := &cloudflareV1alpha1.RateLimit{
		Status: cloudflareV1alpha1.RateLimitStatus{
			Conditions: []metav1.Condition{{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "InvalidMatch",
				Message:            "match.request is required when match is set",
				LastTransitionTime: wantTime,
			}},
		},
	}

	changed := (&RateLimitReconciler{}).updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "InvalidMatch", "match.request is required when match is set")
	if changed {
		t.Fatal("expected identical condition to be unchanged")
	}
	if got := rateLimit.Status.Conditions[0].LastTransitionTime; !got.Equal(&wantTime) {
		t.Fatalf("last transition time changed: %v", got)
	}
}
