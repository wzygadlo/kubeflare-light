package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	cloudflare "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"golang.org/x/time/rate"
)

func TestRateLimitMutationsUseSingleRuleEndpoints(t *testing.T) {
	rateLimit := RateLimit{
		Name:              "test-target",
		Description:       "test-target",
		Expression:        `http.request.uri.path eq "/test"`,
		Action:            "log",
		Threshold:         321,
		Period:            60,
		Characteristics:   []string{"ip.src", "cf.colo.id"},
		MitigationTimeout: 0,
	}

	tests := []struct {
		name   string
		method string
		path   string
		call   func(*CloudflareClient) error
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/zones/zone-1/rulesets/ruleset-1/rules",
			call: func(client *CloudflareClient) error {
				created, err := client.CreateRateLimit(context.Background(), "zone-1", rateLimit)
				if err == nil && created.ID != "new-rule" {
					return fmt.Errorf("created ID = %q, want new-rule", created.ID)
				}
				return err
			},
		},
		{
			name:   "update",
			method: http.MethodPatch,
			path:   "/zones/zone-1/rulesets/ruleset-1/rules/target-rule",
			call: func(client *CloudflareClient) error {
				updated, err := client.UpdateRateLimit(context.Background(), "zone-1", "target-rule", rateLimit)
				if err == nil && updated.ID != "target-rule" {
					return fmt.Errorf("updated ID = %q, want target-rule", updated.ID)
				}
				return err
			},
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   "/zones/zone-1/rulesets/ruleset-1/rules/target-rule",
			call: func(client *CloudflareClient) error {
				return client.DeleteRateLimit(context.Background(), "zone-1", "target-rule")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				if requests == 1 {
					if r.Method != http.MethodGet || r.URL.Path != "/zones/zone-1/rulesets/phases/http_ratelimit/entrypoint" {
						t.Errorf("phase lookup = %s %s", r.Method, r.URL.Path)
					}
					writeRulesetResponse(t, w, []map[string]any{rateLimitRule("sibling-log", 4900)})
					return
				}

				if r.Method != test.method || r.URL.Path != test.path {
					t.Errorf("mutation = %s %s, want %s %s", r.Method, r.URL.Path, test.method, test.path)
				}
				if test.method != http.MethodDelete {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatalf("decode request: %v", err)
					}
					assertRateLimitBody(t, body, rateLimit)
				}

				rules := []map[string]any{rateLimitRule("sibling-log", 4900)}
				switch test.method {
				case http.MethodPost:
					rules = append(rules, rateLimitRule("new-rule", rateLimit.Threshold))
				case http.MethodPatch:
					rules = append(rules, rateLimitRule("target-rule", rateLimit.Threshold))
				}
				writeRulesetResponse(t, w, rules)
			}))
			defer server.Close()

			client := &CloudflareClient{
				client: cloudflare.NewClient(
					option.WithAPIToken("test-token"),
					option.WithBaseURL(server.URL),
				),
				rateLimiter: rate.NewLimiter(rate.Inf, 1),
			}
			if err := test.call(client); err != nil {
				t.Fatal(err)
			}
			if requests != 2 {
				t.Fatalf("requests = %d, want phase lookup plus one mutation", requests)
			}
		})
	}
}

func TestDeleteFirewallRuleUsesSingleRuleEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		lookupStatus int
		deleteStatus int
		targetExists bool
		wantError    bool
	}{
		{"preserves unsupported sibling", http.StatusOK, http.StatusOK, true, false},
		{"already absent rule", http.StatusOK, 0, false, false},
		{"rule disappears after lookup", http.StatusOK, http.StatusNotFound, true, false},
		{"already absent ruleset", http.StatusNotFound, 0, false, false},
		{"lookup forbidden", http.StatusForbidden, 0, false, true},
		{"delete forbidden", http.StatusOK, http.StatusForbidden, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sibling := map[string]any{
				"id": "sibling-execute", "action": "execute", "expression": "true", "enabled": true,
				"action_parameters": map[string]any{"id": "custom-ruleset"},
			}
			rules := []map[string]any{sibling}
			if test.targetExists {
				rules = append(rules, map[string]any{
					"id": "target-rule", "action": "block", "expression": "true", "enabled": true,
				})
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				status := test.lookupStatus
				if requests == 1 {
					if r.Method != http.MethodGet || r.URL.Path != "/zones/zone-1/rulesets/phases/http_request_firewall_custom/entrypoint" {
						t.Errorf("unexpected phase lookup: %s %s", r.Method, r.URL.Path)
					}
					if status == http.StatusOK {
						writeRulesetResponse(t, w, rules)
						return
					}
				} else {
					if r.Method != http.MethodDelete || r.URL.Path != "/zones/zone-1/rulesets/ruleset-1/rules/target-rule" {
						t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					status = test.deleteStatus
				}
				if status == http.StatusOK {
					writeRulesetResponse(t, w, []map[string]any{sibling})
					return
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `{"success":false,"errors":[{"code":1000,"message":"request failed"}]}`)
			}))
			defer server.Close()

			client := &CloudflareClient{
				client: cloudflare.NewClient(
					option.WithAPIToken("test-token"),
					option.WithBaseURL(server.URL),
				),
				rateLimiter: rate.NewLimiter(rate.Inf, 1),
			}
			err := client.DeleteFirewallRule(context.Background(), "zone-1", "target-rule")
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError = %v", err, test.wantError)
			}
			wantRequests := 2
			if test.lookupStatus != http.StatusOK || !test.targetExists {
				wantRequests = 1
			}
			if requests != wantRequests {
				t.Fatalf("requests = %d, want %d", requests, wantRequests)
			}
		})
	}
}

func TestUpdateFirewallRulePreservesSkipOnlyLogging(t *testing.T) {
	rules := []map[string]any{}
	for i, action := range []string{"skip", "skip", "block", "log", "managed_challenge", "challenge", "js_challenge"} {
		rule := map[string]any{
			"id": fmt.Sprintf("sibling-%d", i), "action": action,
			"description": action, "expression": "true", "enabled": true,
			"ref": fmt.Sprintf("ref-%d", i),
		}
		if action == "skip" {
			rule["logging"] = map[string]any{"enabled": i == 0}
			rule["action_parameters"] = map[string]any{"ruleset": "current"}
		}
		rules = append(rules, rule)
	}
	rules = append(rules, map[string]any{
		"id": "target-rule", "action": "block", "description": "target",
		"expression": "true", "enabled": true,
	})

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/zones/zone-1/rulesets/phases/http_request_firewall_custom/entrypoint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			writeRulesetResponse(t, w, rules)
		case http.MethodPut:
			var body struct {
				Rules []map[string]any `json:"rules"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rules[len(rules)-1]["expression"] = `http.host ne "vidiq.com"`
			if !reflect.DeepEqual(body.Rules, rules) {
				t.Errorf("rules = %#v, want %#v", body.Rules, rules)
			}
			writeRulesetResponse(t, w, body.Rules)
		default:
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := &CloudflareClient{
		client: cloudflare.NewClient(
			option.WithAPIToken("test-token"),
			option.WithBaseURL(server.URL),
		),
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
	}
	updated, err := client.UpdateFirewallRule(context.Background(), "zone-1", "target-rule", FirewallRule{
		Action: "block", Description: "target", Expression: `http.host ne "vidiq.com"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != "target-rule" || updated.Expression != `http.host ne "vidiq.com"` {
		t.Fatalf("updated rule = %#v", updated)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want phase lookup plus one mutation", requests)
	}
}

func assertRateLimitBody(t *testing.T, body map[string]any, expected RateLimit) {
	t.Helper()
	if _, ok := body["rules"]; ok {
		t.Fatal("single-rule mutation unexpectedly contains a rules list")
	}
	if _, ok := body["id"]; ok {
		t.Fatal("single-rule mutation must identify the existing rule only through the request path")
	}
	encoded, _ := json.Marshal(body)
	if strings.Contains(string(encoded), "sibling-log") {
		t.Fatal("single-rule mutation unexpectedly contains a sibling rule")
	}
	if body["action"] != expected.Action ||
		body["description"] != expected.Description ||
		body["expression"] != expected.Expression ||
		body["enabled"] != !expected.Disabled {
		t.Fatalf("incomplete rule definition: %s", encoded)
	}
	if _, ok := body["position"]; ok {
		t.Fatal("update unexpectedly changes rule position")
	}
	config, ok := body["ratelimit"].(map[string]any)
	if !ok {
		t.Fatalf("rate limit config = %#v", body["ratelimit"])
	}
	characteristics, ok := config["characteristics"].([]any)
	if !ok || len(characteristics) != len(expected.Characteristics) {
		t.Fatalf("characteristics = %#v", config["characteristics"])
	}
	for i, want := range expected.Characteristics {
		if characteristics[i] != want {
			t.Fatalf("characteristics = %#v, want %#v", characteristics, expected.Characteristics)
		}
	}
	if config["requests_per_period"] != float64(expected.Threshold) ||
		config["period"] != float64(expected.Period) ||
		config["mitigation_timeout"] != float64(expected.MitigationTimeout) {
		t.Fatalf("rate limit config = %#v", config)
	}
}

func rateLimitRule(id string, threshold int) map[string]any {
	return map[string]any{
		"id":          id,
		"action":      "log",
		"description": id,
		"enabled":     true,
		"expression":  "true",
		"ratelimit": map[string]any{
			"characteristics":     []string{"ip.src", "cf.colo.id"},
			"period":              60,
			"requests_per_period": threshold,
			"mitigation_timeout":  0,
		},
	}
}

func writeRulesetResponse(t *testing.T, w http.ResponseWriter, rules []map[string]any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result": map[string]any{
			"id":      "ruleset-1",
			"kind":    "zone",
			"name":    "zone rate limiting",
			"phase":   "http_ratelimit",
			"version": "1",
			"rules":   rules,
		},
	}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
