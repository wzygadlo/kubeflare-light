package controller

import (
	"testing"

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
