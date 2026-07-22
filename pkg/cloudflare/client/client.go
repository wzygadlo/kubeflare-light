package client

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/rulesets"
	"github.com/cloudflare/cloudflare-go/v4/zones"
	"golang.org/x/time/rate"
)

// CloudflareClient wraps the official Cloudflare Go v4 client
type CloudflareClient struct {
	client      *cloudflare.Client
	rateLimiter *rate.Limiter
}

// RateLimit represents a rate limiting rule structure compatible with v4 API
type RateLimit struct {
	ID                string                 `json:"id,omitempty"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description,omitempty"`
	Expression        string                 `json:"expression"`
	Action            string                 `json:"action"`
	Threshold         int                    `json:"threshold"`
	Period            int                    `json:"period"`
	MitigationTimeout int                    `json:"mitigation_timeout,omitempty"`
	Characteristics   []string               `json:"characteristics,omitempty"`
	Disabled          bool                   `json:"disabled,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// FirewallRule represents a firewall rule structure compatible with v4 API
type FirewallRule struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Expression  string                 `json:"expression"`
	Action      string                 `json:"action"`
	Priority    int                    `json:"priority,omitempty"` // DEPRECATED: Not used by Cloudflare Rulesets API - priority is determined by rule order in ruleset
	Paused      bool                   `json:"paused,omitempty"`
	Response    *FirewallRuleResponse  `json:"response,omitempty"` // Only honored by Cloudflare for action "block"
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// FirewallRuleResponse configures a custom response for a "block" action WAF rule.
// Cloudflare only honors a custom response for the "block" action; other actions
// (challenge, js_challenge, managed_challenge, log) render Cloudflare's own page.
type FirewallRuleResponse struct {
	StatusCode  int
	ContentType string
	Content     string
}

// Zone represents zone information
type Zone = zones.Zone

// CloudflareClientInterface defines the contract for Cloudflare API operations.
type CloudflareClientInterface interface {
	// Token and Zone operations
	VerifyToken(ctx context.Context) error
	GetZoneByName(ctx context.Context, zoneName string) (*Zone, error)

	// Rate Limit operations
	CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimit, error)
	GetRateLimit(ctx context.Context, zoneID, rateLimitID string) (*RateLimit, error)
	UpdateRateLimit(ctx context.Context, zoneID, rateLimitID string, rateLimit RateLimit) (*RateLimit, error)
	DeleteRateLimit(ctx context.Context, zoneID, rateLimitID string) error
	ListRateLimits(ctx context.Context, zoneID string) ([]RateLimit, error)

	// WAF Rule operations
	CreateFirewallRule(ctx context.Context, zoneID string, rule FirewallRule) (*FirewallRule, error)
	GetFirewallRule(ctx context.Context, zoneID, ruleID string) (*FirewallRule, error)
	UpdateFirewallRule(ctx context.Context, zoneID, ruleID string, rule FirewallRule) (*FirewallRule, error)
	DeleteFirewallRule(ctx context.Context, zoneID, ruleID string) error
	ListFirewallRules(ctx context.Context, zoneID string) ([]FirewallRule, error)
}

// NewClient creates a new Cloudflare v4 client with API token
func NewClient(apiToken string) (*CloudflareClient, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("API token cannot be empty")
	}

	client := cloudflare.NewClient(
		option.WithAPIToken(apiToken),
	)

	// Create rate limiter to prevent API rate limit errors
	// Cloudflare enterprise typically allows 1200 req/5min
	// Using 5 req/sec = 300 req/min as safe limit with burst capacity
	rateLimiter := rate.NewLimiter(rate.Every(200*time.Millisecond), 5)

	return &CloudflareClient{
		client:      client,
		rateLimiter: rateLimiter,
	}, nil
}

// GetClient returns the underlying Cloudflare v4 client
func (c *CloudflareClient) GetClient() *cloudflare.Client {
	return c.client
}

// waitForRateLimit waits if necessary to respect rate limits
func (c *CloudflareClient) waitForRateLimit(ctx context.Context) error {
	fmt.Printf("Rate limiter: waiting for permission to make API call...\n")
	err := c.rateLimiter.Wait(ctx)
	if err != nil {
		fmt.Printf("Rate limiter: context cancelled or error: %v\n", err)
		return err
	}
	fmt.Printf("Rate limiter: permission granted, making API call\n")
	return nil
}

// VerifyToken verifies the API token is valid by making a test API call
func (c *CloudflareClient) VerifyToken(ctx context.Context) error {
	// Make a simple API call to verify the token works
	_, err := c.client.Zones.List(ctx, zones.ZoneListParams{})
	if err != nil {
		return fmt.Errorf("failed to verify API token: %w", err)
	}
	return nil
}

// GetZone retrieves zone information by zone ID
func (c *CloudflareClient) GetZone(ctx context.Context, zoneID string) (Zone, error) {
	zone, err := c.client.Zones.Get(ctx, zones.ZoneGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return Zone{}, fmt.Errorf("failed to get zone details: %w", err)
	}
	return *zone, nil
}

// GetZoneByName retrieves zone information by zone name
func (c *CloudflareClient) GetZoneByName(ctx context.Context, zoneName string) (*Zone, error) {
	// List zones and find the one with matching name
	zonesList, err := c.client.Zones.List(ctx, zones.ZoneListParams{
		Name: cloudflare.F(zoneName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	if len(zonesList.Result) == 0 {
		return nil, fmt.Errorf("zone %s not found", zoneName)
	}

	// Return the first matching zone
	return &zonesList.Result[0], nil
}

func validateRuleAction(action string) error {
	switch action {
	case "log", "block", "challenge", "js_challenge", "managed_challenge":
		return nil
	default:
		return fmt.Errorf("unsupported action type: %s (valid: log, block, challenge, js_challenge, managed_challenge)", action)
	}
}

func rateLimitConfig(rateLimit RateLimit) rulesets.BlockRuleRatelimitParam {
	characteristics := rateLimit.Characteristics
	if len(characteristics) == 0 {
		characteristics = []string{"ip.src", "cf.colo.id"}
	}
	return rulesets.BlockRuleRatelimitParam{
		Characteristics:   cloudflare.F(characteristics),
		Period:            cloudflare.F(int64(rateLimit.Period)),
		RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
		MitigationTimeout: cloudflare.F(int64(rateLimit.MitigationTimeout)),
	}
}

func newRateLimitRuleBody(rateLimit RateLimit) (rulesets.RuleNewParamsBodyUnion, error) {
	if err := validateRuleAction(rateLimit.Action); err != nil {
		return nil, err
	}
	return rulesets.RuleNewParamsBody{
		Action:      cloudflare.F(rulesets.RuleNewParamsBodyAction(rateLimit.Action)),
		Description: cloudflare.F(rateLimit.Description),
		Expression:  cloudflare.F(rateLimit.Expression),
		Enabled:     cloudflare.F(!rateLimit.Disabled),
		Ratelimit:   cloudflare.F[interface{}](rateLimitConfig(rateLimit)),
	}, nil
}

func editRateLimitRuleBody(rateLimit RateLimit) (rulesets.RuleEditParamsBodyUnion, error) {
	if err := validateRuleAction(rateLimit.Action); err != nil {
		return nil, err
	}
	return rulesets.RuleEditParamsBody{
		Action:      cloudflare.F(rulesets.RuleEditParamsBodyAction(rateLimit.Action)),
		Description: cloudflare.F(rateLimit.Description),
		Expression:  cloudflare.F(rateLimit.Expression),
		Enabled:     cloudflare.F(!rateLimit.Disabled),
		Ratelimit:   cloudflare.F[interface{}](rateLimitConfig(rateLimit)),
	}, nil
}

// CreateRateLimit creates a single rule without replacing its ruleset siblings.
func (c *CloudflareClient) CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimit, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if rateLimit.Expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}
	if rateLimit.Threshold <= 0 {
		return nil, fmt.Errorf("threshold must be greater than 0")
	}
	if rateLimit.Period <= 0 {
		return nil, fmt.Errorf("period must be greater than 0")
	}

	// Validate period is supported by Cloudflare API
	// Counting period (time window for rate limiting): Maximum 3600 seconds (1 hour)
	// Note: 65535 and 86400 are for mitigation timeout, NOT counting period
	// Plan limits: Free: 10s only, Pro: up to 60s, Business: up to 600s, Enterprise: up to 3600s
	validPeriods := map[int]bool{
		10: true, 15: true, 20: true, 30: true, 40: true, 45: true,
		60: true, 90: true, 120: true, 180: true, 240: true, 300: true,
		480: true, 600: true, 900: true, 1200: true, 1800: true, 2400: true,
		3600: true,
	}
	if !validPeriods[rateLimit.Period] {
		return nil, fmt.Errorf("period must be one of the supported values: 10, 15, 20, 30, 40, 45, 60, 90, 120, 180, 240, 300, 480, 600, 900, 1200, 1800, 2400, 3600 seconds (max 1 hour for counting period)")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	body, err := newRateLimitRuleBody(rateLimit)
	if err != nil {
		return nil, err
	}
	updatedRuleset, err := c.client.Rulesets.Rules.New(ctx, currentRuleset.ID, rulesets.RuleNewParams{
		ZoneID: cloudflare.F(zoneID),
		Body:   body,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create rate limit rule: %w", err)
	}
	if len(updatedRuleset.Rules) == 0 || updatedRuleset.Rules[len(updatedRuleset.Rules)-1].ID == "" {
		return nil, fmt.Errorf("created rate limit rule missing from Cloudflare response")
	}
	createdRuleID := updatedRuleset.Rules[len(updatedRuleset.Rules)-1].ID

	return &RateLimit{
		ID:          createdRuleID,
		Name:        rateLimit.Name,
		Description: rateLimit.Description,
		Expression:  rateLimit.Expression,
		Action:      rateLimit.Action, // Use the provided action (challenge, block, etc.)
		Threshold:   rateLimit.Threshold,
		Period:      rateLimit.Period,
		Disabled:    rateLimit.Disabled,
	}, nil
}

// UpdateRateLimit updates an existing rate limit rule using rulesets API
func (c *CloudflareClient) UpdateRateLimit(ctx context.Context, zoneID, rateLimitID string, rateLimit RateLimit) (*RateLimit, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if rateLimitID == "" {
		return nil, fmt.Errorf("rate limit ID cannot be empty")
	}
	if rateLimit.Expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}
	if rateLimit.Threshold <= 0 {
		return nil, fmt.Errorf("threshold must be greater than 0")
	}
	if rateLimit.Period <= 0 {
		return nil, fmt.Errorf("period must be greater than 0")
	}

	// Validate period is supported by Cloudflare API
	// Counting period (time window for rate limiting): Maximum 3600 seconds (1 hour)
	// Note: 65535 and 86400 are for mitigation timeout, NOT counting period
	// Plan limits: Free: 10s only, Pro: up to 60s, Business: up to 600s, Enterprise: up to 3600s
	validPeriods := map[int]bool{
		10: true, 15: true, 20: true, 30: true, 40: true, 45: true,
		60: true, 90: true, 120: true, 180: true, 240: true, 300: true,
		480: true, 600: true, 900: true, 1200: true, 1800: true, 2400: true,
		3600: true,
	}
	if !validPeriods[rateLimit.Period] {
		return nil, fmt.Errorf("period must be one of the supported values: 10, 15, 20, 30, 40, 45, 60, 90, 120, 180, 240, 300, 480, 600, 900, 1200, 1800, 2400, 3600 seconds (max 1 hour for counting period)")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	body, err := editRateLimitRuleBody(rateLimit)
	if err != nil {
		return nil, err
	}
	_, err = c.client.Rulesets.Rules.Edit(ctx, currentRuleset.ID, rateLimitID, rulesets.RuleEditParams{
		ZoneID: cloudflare.F(zoneID),
		Body:   body,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update rate limit rule: %w", err)
	}

	return &RateLimit{
		ID:          rateLimitID,
		Name:        rateLimit.Name,
		Description: rateLimit.Description,
		Expression:  rateLimit.Expression,
		Action:      rateLimit.Action, // Use the provided action (challenge, block, etc.)
		Threshold:   rateLimit.Threshold,
		Period:      rateLimit.Period,
		Disabled:    rateLimit.Disabled,
	}, nil
}

// DeleteRateLimit deletes a rate limit rule using rulesets API
func (c *CloudflareClient) DeleteRateLimit(ctx context.Context, zoneID, rateLimitID string) error {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return fmt.Errorf("zone ID cannot be empty")
	}
	if rateLimitID == "" {
		return fmt.Errorf("rate limit ID cannot be empty")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	_, err = c.client.Rulesets.Rules.Delete(ctx, currentRuleset.ID, rateLimitID, rulesets.RuleDeleteParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to delete rate limit rule: %w", err)
	}

	return nil
}

// GetRateLimit retrieves a specific rate limit rule using rulesets API
func (c *CloudflareClient) GetRateLimit(ctx context.Context, zoneID, rateLimitID string) (*RateLimit, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if rateLimitID == "" {
		return nil, fmt.Errorf("rate limit ID cannot be empty")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	// Find the specific rule
	for _, rule := range currentRuleset.Rules {
		if rule.ID == rateLimitID {
			rateLimit := RateLimit{
				ID:          rule.ID,
				Name:        rule.Description, // Cloudflare stores name in description field
				Description: rule.Description, // Use Cloudflare description as-is for proper comparison
				Expression:  rule.Expression,
				Action:      string(rule.Action),
				Disabled:    !rule.Enabled, // Convert enabled to disabled
			}

			// Extract rate limit parameters from the ratelimit configuration
			// NOTE: All actions (block, challenge, js_challenge, managed_challenge, log) can have rate limiting
			if rule.Ratelimit != nil {
				extracted := false

				// First try the types we KNOW exist in v4.5.1 (based on successful compilation)
				if blockRateLimit, ok := rule.Ratelimit.(rulesets.BlockRuleRatelimit); ok {
					rateLimit.Threshold = int(blockRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(blockRateLimit.Period)
					rateLimit.MitigationTimeout = int(blockRateLimit.MitigationTimeout)
					rateLimit.Characteristics = blockRateLimit.Characteristics
					extracted = true
				} else if logRateLimit, ok := rule.Ratelimit.(rulesets.LogRuleRatelimit); ok {
					rateLimit.Threshold = int(logRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(logRateLimit.Period)
					rateLimit.MitigationTimeout = int(logRateLimit.MitigationTimeout)
					rateLimit.Characteristics = logRateLimit.Characteristics
					extracted = true
				} else if managedChallengeRateLimit, ok := rule.Ratelimit.(rulesets.ManagedChallengeRuleRatelimit); ok {
					rateLimit.Threshold = int(managedChallengeRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(managedChallengeRateLimit.Period)
					rateLimit.MitigationTimeout = int(managedChallengeRateLimit.MitigationTimeout)
					rateLimit.Characteristics = managedChallengeRateLimit.Characteristics
					extracted = true
				} else if phaseGetRespChallengeRateLimit, ok := rule.Ratelimit.(rulesets.PhaseGetResponseRulesRulesetsChallengeRuleRatelimit); ok {
					rateLimit.Threshold = int(phaseGetRespChallengeRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(phaseGetRespChallengeRateLimit.Period)
					rateLimit.MitigationTimeout = int(phaseGetRespChallengeRateLimit.MitigationTimeout)
					rateLimit.Characteristics = phaseGetRespChallengeRateLimit.Characteristics
					extracted = true
				}

				// If none of the known types matched, use reflection
				if !extracted {
					// Advanced extraction using reflection for any unhandled types
					// This is a more robust approach that works with any struct that has the required fields
					v := reflect.ValueOf(rule.Ratelimit)
					// Handle pointer types
					if v.Kind() == reflect.Ptr {
						v = v.Elem()
					}

					// Try to get fields by name using reflection
					if v.Kind() == reflect.Struct {
						// Try RequestsPerPeriod field
						if reqField := v.FieldByName("RequestsPerPeriod"); reqField.IsValid() {
							if reqField.CanInterface() {
								switch val := reqField.Interface().(type) {
								case int64:
									rateLimit.Threshold = int(val)
									extracted = true
								case int:
									rateLimit.Threshold = val
									extracted = true
								case float64:
									rateLimit.Threshold = int(val)
									extracted = true
								}
							}
						}

						// Try Period field
						if periodField := v.FieldByName("Period"); periodField.IsValid() {
							if periodField.CanInterface() {
								switch val := periodField.Interface().(type) {
								case int64:
									rateLimit.Period = int(val)
									extracted = true
								case int:
									rateLimit.Period = val
									extracted = true
								case float64:
									rateLimit.Period = int(val)
									extracted = true
								}
							}
						}

						// Try MitigationTimeout field
						if timeoutField := v.FieldByName("MitigationTimeout"); timeoutField.IsValid() {
							if timeoutField.CanInterface() {
								switch val := timeoutField.Interface().(type) {
								case int64:
									rateLimit.MitigationTimeout = int(val)
									extracted = true
								case int:
									rateLimit.MitigationTimeout = val
									extracted = true
								case float64:
									rateLimit.MitigationTimeout = int(val)
									extracted = true
								}
							}
						}
					}

					// Fallback: Try to extract via map interface
					if !extracted {
						if rateLimitMap, ok := rule.Ratelimit.(map[string]interface{}); ok {
							if reqPerPeriod, ok := rateLimitMap["requests_per_period"].(float64); ok {
								rateLimit.Threshold = int(reqPerPeriod)
								extracted = true
							}
							if period, ok := rateLimitMap["period"].(float64); ok {
								rateLimit.Period = int(period)
								extracted = true
							}
							if mitigationTimeout, ok := rateLimitMap["mitigation_timeout"].(float64); ok {
								rateLimit.MitigationTimeout = int(mitigationTimeout)
								extracted = true
							}
						}
					}

					if !extracted {
						// Log unknown type for debugging only if extraction completely failed
						fmt.Printf("WARNING: Could not extract ratelimit from type %T for rule %s (action: %s), using defaults\n", rule.Ratelimit, rule.ID, rule.Action)
					}
				}

				// Set defaults if extraction failed
				if !extracted || rateLimit.Threshold == 0 {
					rateLimit.Threshold = 100
				}
				if !extracted || rateLimit.Period == 0 {
					rateLimit.Period = 60
				}
			} else {
				// Fallback defaults if ratelimit config is missing
				rateLimit.Threshold = 100
				rateLimit.Period = 60
			}

			return &rateLimit, nil
		}
	}

	return nil, fmt.Errorf("rate limit rule with ID %s not found", rateLimitID)
}

// ListRateLimits lists all rate limit rules for a zone
func (c *CloudflareClient) ListRateLimits(ctx context.Context, zoneID string) ([]RateLimit, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	var rateLimits []RateLimit
	for _, rule := range currentRuleset.Rules {
		// Convert rulesets API rules back to our RateLimit structure
		rateLimit := RateLimit{
			ID:          rule.ID,
			Description: rule.Description,
			Expression:  rule.Expression,
			Action:      string(rule.Action),
		}

		// Extract rate limit parameters if available and set the actual action
		rateLimit.Action = string(rule.Action) // Use the actual action from Cloudflare (block, challenge, etc.)

		rateLimits = append(rateLimits, rateLimit)
	}

	return rateLimits, nil
}

// CreateFirewallRule creates a new firewall rule (for WAF) using rulesets API
func (c *CloudflareClient) CreateFirewallRule(ctx context.Context, zoneID string, rule FirewallRule) (*FirewallRule, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if rule.Expression == "" {
		return nil, fmt.Errorf("expression cannot be empty")
	}
	if rule.Action == "" {
		return nil, fmt.Errorf("action cannot be empty")
	}

	// Get the http_request_firewall_custom phase ruleset for the zone
	phase := rulesets.PhaseHTTPRequestFirewallCustom
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall custom ruleset: %w", err)
	}

	// Create new WAF rule based on action type
	newRuleParam, err := c.createRuleParamForAction(rule.Action, "", rule.Description, rule.Expression, !rule.Paused)
	if err != nil {
		return nil, err
	}
	newRuleParam = applyFirewallRuleResponse(newRuleParam, rule.Response)

	// Check for duplicate rules by description and expression
	for _, existingRule := range currentRuleset.Rules {
		if existingRule.Description == rule.Description && existingRule.Expression == rule.Expression {
			// Rule already exists, return it instead of creating a duplicate
			return &FirewallRule{
				ID:          existingRule.ID,
				Name:        rule.Name,
				Description: existingRule.Description,
				Expression:  existingRule.Expression,
				Action:      string(existingRule.Action),
				Priority:    rule.Priority,
				Paused:      !existingRule.Enabled,
			}, nil
		}
	}

	// Collect existing rules to preserve them
	var existingRules []rulesets.PhaseUpdateParamsRuleUnion
	for _, existingRule := range currentRuleset.Rules {
		ruleParam, err := c.preserveExistingRule(existingRule)
		if err != nil {
			return nil, err
		}
		existingRules = append(existingRules, ruleParam)
	}

	// Add the new rule to the beginning of the list (higher priority)
	allRules := append([]rulesets.PhaseUpdateParamsRuleUnion{newRuleParam}, existingRules...)

	// Update the ruleset with all rules (existing + new)
	updatedRuleset, err := c.client.Rulesets.Phases.Update(ctx, phase, rulesets.PhaseUpdateParams{
		ZoneID: cloudflare.F(zoneID),
		Rules:  cloudflare.F(allRules),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update firewall custom ruleset: %w", err)
	}

	// Find the newly created rule (it should be the first one since we added it first)
	if len(updatedRuleset.Rules) > 0 {
		newRule := updatedRuleset.Rules[0]
		return &FirewallRule{
			ID:          newRule.ID,
			Name:        rule.Name,
			Description: newRule.Description,
			Expression:  newRule.Expression,
			Action:      string(newRule.Action),
			Priority:    rule.Priority, // Keep original priority
			Paused:      !newRule.Enabled,
		}, nil
	}

	return nil, fmt.Errorf("failed to find created firewall rule in response")
}

// UpdateFirewallRule updates an existing firewall rule
func (c *CloudflareClient) UpdateFirewallRule(ctx context.Context, zoneID, ruleID string, rule FirewallRule) (*FirewallRule, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if ruleID == "" {
		return nil, fmt.Errorf("rule ID cannot be empty")
	}

	// Get the http_request_firewall_custom phase ruleset for the zone
	phase := rulesets.PhaseHTTPRequestFirewallCustom
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall custom ruleset: %w", err)
	}

	// Find the rule to update and preserve other rules
	var allRules []rulesets.PhaseUpdateParamsRuleUnion
	var targetRuleFound bool

	for _, existingRule := range currentRuleset.Rules {
		var ruleParam rulesets.PhaseUpdateParamsRuleUnion

		if existingRule.ID == ruleID {
			// This is the rule we want to update
			targetRuleFound = true

			ruleParam, err = c.createRuleParamForAction(rule.Action, ruleID, rule.Description, rule.Expression, !rule.Paused)
			if err != nil {
				return nil, err
			}
			ruleParam = applyFirewallRuleResponse(ruleParam, rule.Response)
		} else {
			// Preserve other existing rules
			ruleParam, err = c.preserveExistingRule(existingRule)
			if err != nil {
				return nil, err
			}
		}

		allRules = append(allRules, ruleParam)
	}

	if !targetRuleFound {
		return nil, fmt.Errorf("firewall rule with ID %s not found", ruleID)
	}

	// Update the ruleset with all rules (existing + updated)
	updatedRuleset, err := c.client.Rulesets.Phases.Update(ctx, phase, rulesets.PhaseUpdateParams{
		ZoneID: cloudflare.F(zoneID),
		Rules:  cloudflare.F(allRules),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update firewall custom ruleset: %w", err)
	}

	// Find the updated rule in the response
	for _, updatedRule := range updatedRuleset.Rules {
		if updatedRule.ID == ruleID {
			return &FirewallRule{
				ID:          updatedRule.ID,
				Name:        rule.Name,
				Description: updatedRule.Description,
				Expression:  updatedRule.Expression,
				Action:      string(updatedRule.Action),
				Priority:    rule.Priority, // Keep original priority
				Paused:      !updatedRule.Enabled,
			}, nil
		}
	}

	return nil, fmt.Errorf("failed to find updated firewall rule in response")
}

// DeleteFirewallRule deletes a firewall rule
func (c *CloudflareClient) DeleteFirewallRule(ctx context.Context, zoneID, ruleID string) error {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return fmt.Errorf("zone ID cannot be empty")
	}
	if ruleID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}

	// Get the http_request_firewall_custom phase ruleset for the zone
	phase := rulesets.PhaseHTTPRequestFirewallCustom
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return fmt.Errorf("failed to get firewall custom ruleset: %w", err)
	}

	// Build new rules list without the rule to delete
	var remainingRules []rulesets.PhaseUpdateParamsRuleUnion
	var targetRuleFound bool

	for _, existingRule := range currentRuleset.Rules {
		if existingRule.ID == ruleID {
			// Skip this rule (delete it)
			targetRuleFound = true
			continue
		}

		// Preserve all other existing rules. Unlike Create/UpdateFirewallRule, a rule we
		// can't round-trip must not block deletion of our own resource: log and drop it
		// (matching the historical behavior) instead of failing the whole operation, so the
		// WAFRule finalizer always completes.
		ruleParam, err := c.preserveExistingRule(existingRule)
		if err != nil {
			fmt.Printf("WARNING: dropping firewall rule %s (action %q) while deleting rule %s: %v\n", existingRule.ID, existingRule.Action, ruleID, err)
			continue
		}

		remainingRules = append(remainingRules, ruleParam)
	}

	if !targetRuleFound {
		return fmt.Errorf("firewall rule with ID %s not found", ruleID)
	}

	// Update the ruleset without the deleted rule
	_, err = c.client.Rulesets.Phases.Update(ctx, phase, rulesets.PhaseUpdateParams{
		ZoneID: cloudflare.F(zoneID),
		Rules:  cloudflare.F(remainingRules),
	})
	if err != nil {
		return fmt.Errorf("failed to delete firewall rule: %w", err)
	}

	return nil
}

// GetFirewallRule retrieves a specific firewall rule
func (c *CloudflareClient) GetFirewallRule(ctx context.Context, zoneID, ruleID string) (*FirewallRule, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if ruleID == "" {
		return nil, fmt.Errorf("rule ID cannot be empty")
	}

	// Get the http_request_firewall_custom phase ruleset for the zone
	phase := rulesets.PhaseHTTPRequestFirewallCustom
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall custom ruleset: %w", err)
	}

	// Find the specific rule by ID
	for _, rule := range currentRuleset.Rules {
		if rule.ID == ruleID {
			return &FirewallRule{
				ID:          rule.ID,
				Name:        rule.Description,
				Description: rule.Description,
				Expression:  rule.Expression,
				Action:      string(rule.Action),
				Priority:    0,
				Paused:      !rule.Enabled,
				Response:    extractFirewallRuleResponse(rule.ActionParameters),
			}, nil
		}
	}

	return nil, fmt.Errorf("firewall rule with ID %s not found", ruleID)
}

// ListFirewallRules lists all firewall rules for a zone
func (c *CloudflareClient) ListFirewallRules(ctx context.Context, zoneID string) ([]FirewallRule, error) {
	// Wait for rate limiter before making API call
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}

	// Get the http_request_firewall_custom phase ruleset for the zone
	phase := rulesets.PhaseHTTPRequestFirewallCustom
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall custom ruleset: %w", err)
	}

	var rules []FirewallRule
	for _, rule := range currentRuleset.Rules {
		rules = append(rules, FirewallRule{
			ID:          rule.ID,
			Name:        rule.Description,
			Description: rule.Description,
			Expression:  rule.Expression,
			Action:      string(rule.Action),
			Priority:    0,
			Paused:      !rule.Enabled,
		})
	}

	return rules, nil
}

func (c *CloudflareClient) createSkipRuleParam(existingRule rulesets.PhaseGetResponseRule) (rulesets.SkipRuleParam, error) {
	var params rulesets.SkipRuleActionParameters
	if typedParams, ok := existingRule.ActionParameters.(rulesets.SkipRuleActionParameters); ok {
		params = typedParams
	} else {
		data, err := json.Marshal(existingRule.ActionParameters)
		if err != nil {
			return rulesets.SkipRuleParam{}, fmt.Errorf("cannot preserve skip rule action parameters for rule %s: %w", existingRule.ID, err)
		}
		if err := json.Unmarshal(data, &params); err != nil {
			return rulesets.SkipRuleParam{}, fmt.Errorf("cannot preserve skip rule action parameters for rule %s: %w", existingRule.ID, err)
		}
	}

	actionParams := rulesets.SkipRuleActionParametersParam{}
	if len(params.Phases) > 0 {
		actionParams.Phases = cloudflare.F(params.Phases)
	}
	if len(params.Products) > 0 {
		actionParams.Products = cloudflare.F(params.Products)
	}
	if len(params.Rules) > 0 {
		actionParams.Rules = cloudflare.F(params.Rules)
	}
	if params.Ruleset != "" {
		actionParams.Ruleset = cloudflare.F(params.Ruleset)
	}
	if len(params.Rulesets) > 0 {
		actionParams.Rulesets = cloudflare.F(params.Rulesets)
	}

	skipParam := rulesets.SkipRuleParam{
		ID:               cloudflare.F(existingRule.ID),
		Action:           cloudflare.F(rulesets.SkipRuleActionSkip),
		ActionParameters: cloudflare.F(actionParams),
		Description:      cloudflare.F(existingRule.Description),
		Expression:       cloudflare.F(existingRule.Expression),
		Enabled:          cloudflare.F(existingRule.Enabled),
		Logging:          cloudflare.F(rulesets.LoggingParam{Enabled: cloudflare.F(existingRule.Logging.Enabled)}),
	}
	if existingRule.Ref != "" {
		skipParam.Ref = cloudflare.F(existingRule.Ref)
	}
	return skipParam, nil
}

// preserveExistingRule rebuilds a rule parameter for an existing Cloudflare rule so it can be
// resubmitted unchanged in a Phases.Update call. A phase update replaces the entire rules array,
// so omitting action_parameters/logging/ref here would silently reset them to Cloudflare's
// defaults on every unrelated create/update/delete in the same zone.
func (c *CloudflareClient) preserveExistingRule(existingRule rulesets.PhaseGetResponseRule) (rulesets.PhaseUpdateParamsRuleUnion, error) {
	if existingRule.Action == "skip" {
		return c.createSkipRuleParam(existingRule)
	}

	param, err := c.createRuleParamForAction(string(existingRule.Action), existingRule.ID, existingRule.Description, existingRule.Expression, existingRule.Enabled)
	if err != nil {
		return nil, fmt.Errorf("cannot preserve unsupported firewall rule action %q for rule %s", existingRule.Action, existingRule.ID)
	}

	logging := cloudflare.F(rulesets.LoggingParam{Enabled: cloudflare.F(existingRule.Logging.Enabled)})

	switch p := param.(type) {
	case rulesets.BlockRuleParam:
		p.Logging = logging
		if existingRule.Ref != "" {
			p.Ref = cloudflare.F(existingRule.Ref)
		}
		param = p
	case rulesets.LogRuleParam:
		p.Logging = logging
		if existingRule.Ref != "" {
			p.Ref = cloudflare.F(existingRule.Ref)
		}
		param = p
	case rulesets.ManagedChallengeRuleParam:
		p.Logging = logging
		if existingRule.Ref != "" {
			p.Ref = cloudflare.F(existingRule.Ref)
		}
		param = p
	}

	return applyFirewallRuleResponse(param, extractFirewallRuleResponse(existingRule.ActionParameters)), nil
}

// extractFirewallRuleResponse reads a custom block response off an existing rule's
// action_parameters, if any. Only "block" rules can carry one (see FirewallRuleResponse),
// so this returns nil for every other action or when no response is configured.
func extractFirewallRuleResponse(actionParameters interface{}) *FirewallRuleResponse {
	blockParams, ok := actionParameters.(rulesets.BlockRuleActionParameters)
	if !ok {
		return nil
	}
	if blockParams.Response.Content == "" && blockParams.Response.ContentType == "" && blockParams.Response.StatusCode == 0 {
		return nil
	}
	return &FirewallRuleResponse{
		StatusCode:  int(blockParams.Response.StatusCode),
		ContentType: blockParams.Response.ContentType,
		Content:     blockParams.Response.Content,
	}
}

// applyFirewallRuleResponse sets a custom response on a rule parameter, if the underlying
// param type supports one (currently only BlockRuleParam does). A no-op otherwise, since
// challenge/js_challenge/managed_challenge/log/skip never carry a configurable response.
func applyFirewallRuleResponse(param rulesets.PhaseUpdateParamsRuleUnion, response *FirewallRuleResponse) rulesets.PhaseUpdateParamsRuleUnion {
	if response == nil {
		return param
	}
	blockParam, ok := param.(rulesets.BlockRuleParam)
	if !ok {
		return param
	}
	blockParam.ActionParameters = cloudflare.F(rulesets.BlockRuleActionParametersParam{
		Response: cloudflare.F(rulesets.BlockRuleActionParametersResponseParam{
			Content:     cloudflare.F(response.Content),
			ContentType: cloudflare.F(response.ContentType),
			StatusCode:  cloudflare.F(int64(response.StatusCode)),
		}),
	})
	return blockParam
}

// createRuleParamForAction creates the appropriate rule parameter type based on the action
func (c *CloudflareClient) createRuleParamForAction(action, id, description, expression string, enabled bool) (rulesets.PhaseUpdateParamsRuleUnion, error) {
	if err := validateRuleAction(action); err != nil {
		return nil, err
	}

	switch action {
	case "log":
		param := rulesets.LogRuleParam{
			Action:      cloudflare.F(rulesets.LogRuleActionLog),
			Description: cloudflare.F(description),
			Expression:  cloudflare.F(expression),
			Enabled:     cloudflare.F(enabled),
		}
		if id != "" {
			param.ID = cloudflare.F(id)
		}
		return param, nil
	case "block":
		param := rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
			Description: cloudflare.F(description),
			Expression:  cloudflare.F(expression),
			Enabled:     cloudflare.F(enabled),
		}
		if id != "" {
			param.ID = cloudflare.F(id)
		}
		return param, nil
	case "challenge", "js_challenge", "managed_challenge":
		param := rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleAction(action)),
			Description: cloudflare.F(description),
			Expression:  cloudflare.F(expression),
			Enabled:     cloudflare.F(enabled),
		}
		if id != "" {
			param.ID = cloudflare.F(id)
		}
		return param, nil
	default:
		return nil, fmt.Errorf("unsupported action type: %s", action)
	}
}
