package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
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
