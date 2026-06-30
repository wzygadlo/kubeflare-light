package client

import (
	"context"
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
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
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

// CreateRateLimit creates a new rate limit rule using rulesets API
// CRITICAL: This now uses the modern rulesets API to replace the deprecated Rate Limiting API (sunset June 15, 2025)
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

	// Validate period is supported (10, 60, 120, 300, 600, 3600 seconds)
	validPeriods := map[int]bool{10: true, 60: true, 120: true, 300: true, 600: true, 3600: true}
	if !validPeriods[rateLimit.Period] {
		return nil, fmt.Errorf("period must be one of: 10, 60, 120, 300, 600, 3600 seconds")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	// Create a new rate limiting rule using the correct parameter type for the action
	newRule, err := c.createRuleParamForAction(rateLimit.Action, "", rateLimit.Description, rateLimit.Expression, !rateLimit.Disabled)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule param for action %s: %w", rateLimit.Action, err)
	}

	// Set the rate limiting configuration based on the rule type
	characteristics := rateLimit.Characteristics
	if len(characteristics) == 0 {
		characteristics = []string{"ip.src", "cf.colo.id"}
	}

	// Use provided mitigation timeout:
	// - mitigation_timeout = 0: Throttle mode (block only excess requests)
	// - mitigation_timeout > 0: Ban mode (block all requests for duration)
	// Note: Only Enterprise customers can use throttle mode with block action
	newRule, err = c.setRateLimitConfig(newRule, rulesets.BlockRuleRatelimitParam{
		Characteristics:   cloudflare.F(characteristics),
		Period:            cloudflare.F(int64(rateLimit.Period)),
		RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
		MitigationTimeout: cloudflare.F(int64(rateLimit.MitigationTimeout)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set rate limit config: %w", err)
	}

	newRuleParams := []rulesets.PhaseUpdateParamsRuleUnion{newRule}

	// Add existing rules to preserve them (with their rate limiting configuration)
	for _, rule := range currentRuleset.Rules {
		// ALL rules in http_ratelimit phase MUST have ratelimit configuration
		// Skip any rules that don't have ratelimit (they shouldn't exist, but safety check)
		if rule.Ratelimit == nil {
			continue // Skip rules without ratelimit configuration
		}

		// Use the correct parameter type for the action to preserve existing rules
		ruleParam, err := c.createRuleParamForAction(string(rule.Action), rule.ID, rule.Description, rule.Expression, rule.Enabled)
		if err != nil {
			// Fallback to BlockRuleParam if we can't determine the correct type
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(rule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleActionBlock), // Use "block" as safe fallback
				Description: cloudflare.F(rule.Description),
				Expression:  cloudflare.F(rule.Expression),
				Enabled:     cloudflare.F(rule.Enabled),
			}
		}

		// CRITICAL: ALL rules in http_ratelimit phase MUST have ratelimit configuration
		// Extract existing rate limit configuration and preserve it
		rateLimitConfig := rulesets.BlockRuleRatelimitParam{
			Characteristics:   cloudflare.F([]string{"ip.src", "cf.colo.id"}),
			Period:            cloudflare.F(int64(60)),
			RequestsPerPeriod: cloudflare.F(int64(100)),
			MitigationTimeout: cloudflare.F(int64(60)),
		}

		// Try to extract the existing rate limit configuration
		if blockRateLimit, ok := rule.Ratelimit.(rulesets.BlockRuleRatelimit); ok {
			rateLimitConfig = rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F(blockRateLimit.Characteristics),
				Period:            cloudflare.F(blockRateLimit.Period),
				RequestsPerPeriod: cloudflare.F(blockRateLimit.RequestsPerPeriod),
				MitigationTimeout: cloudflare.F(blockRateLimit.MitigationTimeout),
			}
		}

		// Set the rate limit configuration using the helper function
		ruleParam, err = c.setRateLimitConfig(ruleParam, rateLimitConfig)
		if err != nil {
			// If we can't set the rate limit config, skip this rule to avoid API errors
			continue
		}

		newRuleParams = append([]rulesets.PhaseUpdateParamsRuleUnion{ruleParam}, newRuleParams...)
	}

	// Update the ruleset with the new rule
	updatedRuleset, err := c.client.Rulesets.Phases.Update(ctx, phase, rulesets.PhaseUpdateParams{
		ZoneID: cloudflare.F(zoneID),
		Rules:  cloudflare.F(newRuleParams),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update ruleset with new rate limit rule: %w", err)
	}

	// Find the newly created rule in the response
	var createdRuleID string
	if len(updatedRuleset.Rules) > 0 {
		// The new rule should be the last one added
		lastRule := updatedRuleset.Rules[len(updatedRuleset.Rules)-1]
		createdRuleID = lastRule.ID
	}

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

	// Validate period is supported (10, 60, 120, 300, 600, 3600 seconds)
	validPeriods := map[int]bool{10: true, 60: true, 120: true, 300: true, 600: true, 3600: true}
	if !validPeriods[rateLimit.Period] {
		return nil, fmt.Errorf("period must be one of: 10, 60, 120, 300, 600, 3600 seconds")
	}

	// Get the http_ratelimit phase ruleset for the zone
	phase := rulesets.PhaseHTTPRatelimit
	currentRuleset, err := c.client.Rulesets.Phases.Get(ctx, phase, rulesets.PhaseGetParams{
		ZoneID: cloudflare.F(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit ruleset: %w", err)
	}

	// Build new rules list with updated rule
	var newRuleParams []rulesets.PhaseUpdateParamsRuleUnion
	var ruleFound bool

	for _, rule := range currentRuleset.Rules {
		if rule.ID == rateLimitID {
			// Replace the rule with updated version using the provided action
			// Use BlockRuleParam as the base type but change the action dynamically
			characteristics := rateLimit.Characteristics
			if len(characteristics) == 0 {
				characteristics = []string{"ip.src", "cf.colo.id"}
			}

			// Use provided mitigation timeout:
			// - mitigation_timeout = 0: Throttle mode (block only excess requests)
			// - mitigation_timeout > 0: Ban mode (block all requests for duration)
			newRuleParams = append(newRuleParams, rulesets.BlockRuleParam{
				ID:          cloudflare.F(rateLimitID),
				Action:      cloudflare.F(rulesets.BlockRuleAction(rateLimit.Action)), // Use provided action (challenge, block, etc.)
				Description: cloudflare.F(rateLimit.Description),
				Expression:  cloudflare.F(rateLimit.Expression),
				Enabled:     cloudflare.F(!rateLimit.Disabled), // CRITICAL: Enabled field is required
				Ratelimit: cloudflare.F(rulesets.BlockRuleRatelimitParam{
					Characteristics:   cloudflare.F(characteristics),
					Period:            cloudflare.F(int64(rateLimit.Period)),
					RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
					MitigationTimeout: cloudflare.F(int64(rateLimit.MitigationTimeout)),
				}),
			})
			ruleFound = true
		} else {
			// Keep existing rule (with its rate limiting configuration)
			// ALL rules in http_ratelimit phase MUST have ratelimit configuration
			if rule.Ratelimit == nil {
				continue // Skip rules without ratelimit configuration (shouldn't exist)
			}

			// Use consistent rule structure regardless of action type
			ruleParam := rulesets.BlockRuleParam{
				ID:          cloudflare.F(rule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleAction(rule.Action)),
				Description: cloudflare.F(rule.Description),
				Expression:  cloudflare.F(rule.Expression),
				Enabled:     cloudflare.F(rule.Enabled), // CRITICAL: Preserve the enabled state
			}

			// CRITICAL: ALL rules in http_ratelimit phase MUST have ratelimit configuration
			rateLimitExtracted := false

			// Try BlockRuleRatelimit (for block/challenge actions)
			if existingRateLimit, ok := rule.Ratelimit.(rulesets.BlockRuleRatelimit); ok {
				ruleParam.Ratelimit = cloudflare.F(rulesets.BlockRuleRatelimitParam{
					Characteristics:   cloudflare.F(existingRateLimit.Characteristics),
					Period:            cloudflare.F(existingRateLimit.Period),
					RequestsPerPeriod: cloudflare.F(existingRateLimit.RequestsPerPeriod),
					MitigationTimeout: cloudflare.F(existingRateLimit.MitigationTimeout),
				})
				rateLimitExtracted = true
			}

			// If we couldn't extract the ratelimit config, use safe defaults
			if !rateLimitExtracted {
				// Use default ratelimit configuration to ensure the rule is valid
				ruleParam.Ratelimit = cloudflare.F(rulesets.BlockRuleRatelimitParam{
					Characteristics:   cloudflare.F([]string{"ip.src", "cf.colo.id"}),
					Period:            cloudflare.F(int64(60)),
					RequestsPerPeriod: cloudflare.F(int64(100)),
					MitigationTimeout: cloudflare.F(int64(60)),
				})
			}

			newRuleParams = append(newRuleParams, ruleParam)
		}
	}

	if !ruleFound {
		return nil, fmt.Errorf("rate limit rule with ID %s not found", rateLimitID)
	}

	// Update the ruleset
	_, err = c.client.Rulesets.Phases.Update(ctx, phase, rulesets.PhaseUpdateParams{
		ZoneID: cloudflare.F(zoneID),
		Rules:  cloudflare.F(newRuleParams),
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

	// Build new rules list without the rule to delete
	var newRuleParams []rulesets.PhaseUpdateParamsRuleUnion
	var ruleFound bool

	for _, rule := range currentRuleset.Rules {
		if rule.ID == rateLimitID {
			// Skip this rule (delete it)
			ruleFound = true
			continue
		}

		// Keep existing rule (with its rate limiting configuration)
		// ALL rules in http_ratelimit phase MUST have ratelimit configuration
		if rule.Ratelimit == nil {
			continue // Skip rules without ratelimit configuration (shouldn't exist)
		}

		// Use consistent rule structure regardless of action type
		ruleParam := rulesets.BlockRuleParam{
			ID:          cloudflare.F(rule.ID),
			Action:      cloudflare.F(rulesets.BlockRuleAction(rule.Action)),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(rule.Enabled), // CRITICAL: Preserve the enabled state
		}

		// CRITICAL: ALL rules in http_ratelimit phase MUST have ratelimit configuration
		rateLimitExtracted := false

		// Try BlockRuleRatelimit (for block/challenge actions)
		if existingRateLimit, ok := rule.Ratelimit.(rulesets.BlockRuleRatelimit); ok {
			ruleParam.Ratelimit = cloudflare.F(rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F(existingRateLimit.Characteristics),
				Period:            cloudflare.F(existingRateLimit.Period),
				RequestsPerPeriod: cloudflare.F(existingRateLimit.RequestsPerPeriod),
				MitigationTimeout: cloudflare.F(existingRateLimit.MitigationTimeout),
			})
			rateLimitExtracted = true
		}

		// If we couldn't extract the ratelimit config, use safe defaults
		if !rateLimitExtracted {
			// Use default ratelimit configuration to ensure the rule is valid
			ruleParam.Ratelimit = cloudflare.F(rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F([]string{"ip.src", "cf.colo.id"}),
				Period:            cloudflare.F(int64(60)),
				RequestsPerPeriod: cloudflare.F(int64(100)),
				MitigationTimeout: cloudflare.F(int64(60)),
			})
		}

		newRuleParams = append(newRuleParams, ruleParam)
	}

	if !ruleFound {
		return fmt.Errorf("rate limit rule with ID %s not found", rateLimitID)
	}

	// Update the ruleset without the deleted rule
	_, err = c.client.Rulesets.Phases.Update(ctx, phase, rulesets.PhaseUpdateParams{
		ZoneID: cloudflare.F(zoneID),
		Rules:  cloudflare.F(newRuleParams),
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
	var newRuleParam rulesets.PhaseUpdateParamsRuleUnion

	switch rule.Action {
	case "block":
		newRuleParam = rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(!rule.Paused),
		}
	case "challenge":
		// Use BlockRuleParam with challenge action
		newRuleParam = rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleAction("challenge")),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(!rule.Paused),
		}
	case "js_challenge":
		// Use BlockRuleParam with js_challenge action
		newRuleParam = rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleAction("js_challenge")),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(!rule.Paused),
		}
	case "managed_challenge":
		newRuleParam = rulesets.ManagedChallengeRuleParam{
			Action:      cloudflare.F(rulesets.ManagedChallengeRuleActionManagedChallenge),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(!rule.Paused),
		}
	case "log":
		newRuleParam = rulesets.LogRuleParam{
			Action:      cloudflare.F(rulesets.LogRuleActionLog),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(!rule.Paused),
		}
	default:
		return nil, fmt.Errorf("unsupported action: %s", rule.Action)
	}

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
		// Re-create the rule parameter based on its action type to preserve it
		var ruleParam rulesets.PhaseUpdateParamsRuleUnion

		switch existingRule.Action {
		case "block":
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "challenge":
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleAction("challenge")),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "js_challenge":
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleAction("js_challenge")),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "managed_challenge":
			ruleParam = rulesets.ManagedChallengeRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.ManagedChallengeRuleActionManagedChallenge),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "log":
			ruleParam = rulesets.LogRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.LogRuleActionLog),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		default:
			return nil, fmt.Errorf("cannot preserve unsupported firewall rule action %q for rule %s", existingRule.Action, existingRule.ID)
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

			switch rule.Action {
			case "block":
				ruleParam = rulesets.BlockRuleParam{
					ID:          cloudflare.F(ruleID),
					Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
					Enabled:     cloudflare.F(!rule.Paused),
				}
			case "challenge":
				ruleParam = rulesets.BlockRuleParam{
					ID:          cloudflare.F(ruleID),
					Action:      cloudflare.F(rulesets.BlockRuleAction("challenge")),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
					Enabled:     cloudflare.F(!rule.Paused),
				}
			case "js_challenge":
				ruleParam = rulesets.BlockRuleParam{
					ID:          cloudflare.F(ruleID),
					Action:      cloudflare.F(rulesets.BlockRuleAction("js_challenge")),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
					Enabled:     cloudflare.F(!rule.Paused),
				}
			case "managed_challenge":
				ruleParam = rulesets.ManagedChallengeRuleParam{
					ID:          cloudflare.F(ruleID),
					Action:      cloudflare.F(rulesets.ManagedChallengeRuleActionManagedChallenge),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
					Enabled:     cloudflare.F(!rule.Paused),
				}
			case "log":
				ruleParam = rulesets.LogRuleParam{
					ID:          cloudflare.F(ruleID),
					Action:      cloudflare.F(rulesets.LogRuleActionLog),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
					Enabled:     cloudflare.F(!rule.Paused),
				}
			default:
				return nil, fmt.Errorf("unsupported action: %s", rule.Action)
			}
		} else {
			// Preserve other existing rules
			switch existingRule.Action {
			case "block":
				ruleParam = rulesets.BlockRuleParam{
					ID:          cloudflare.F(existingRule.ID),
					Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
					Description: cloudflare.F(existingRule.Description),
					Expression:  cloudflare.F(existingRule.Expression),
					Enabled:     cloudflare.F(existingRule.Enabled),
				}
			case "challenge":
				ruleParam = rulesets.BlockRuleParam{
					ID:          cloudflare.F(existingRule.ID),
					Action:      cloudflare.F(rulesets.BlockRuleAction("challenge")),
					Description: cloudflare.F(existingRule.Description),
					Expression:  cloudflare.F(existingRule.Expression),
					Enabled:     cloudflare.F(existingRule.Enabled),
				}
			case "js_challenge":
				ruleParam = rulesets.BlockRuleParam{
					ID:          cloudflare.F(existingRule.ID),
					Action:      cloudflare.F(rulesets.BlockRuleAction("js_challenge")),
					Description: cloudflare.F(existingRule.Description),
					Expression:  cloudflare.F(existingRule.Expression),
					Enabled:     cloudflare.F(existingRule.Enabled),
				}
			case "managed_challenge":
				ruleParam = rulesets.ManagedChallengeRuleParam{
					ID:          cloudflare.F(existingRule.ID),
					Action:      cloudflare.F(rulesets.ManagedChallengeRuleActionManagedChallenge),
					Description: cloudflare.F(existingRule.Description),
					Expression:  cloudflare.F(existingRule.Expression),
					Enabled:     cloudflare.F(existingRule.Enabled),
				}
			case "log":
				ruleParam = rulesets.LogRuleParam{
					ID:          cloudflare.F(existingRule.ID),
					Action:      cloudflare.F(rulesets.LogRuleActionLog),
					Description: cloudflare.F(existingRule.Description),
					Expression:  cloudflare.F(existingRule.Expression),
					Enabled:     cloudflare.F(existingRule.Enabled),
				}
			default:
				return nil, fmt.Errorf("cannot preserve unsupported firewall rule action %q for rule %s", existingRule.Action, existingRule.ID)
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

		// Preserve all other existing rules
		var ruleParam rulesets.PhaseUpdateParamsRuleUnion

		switch existingRule.Action {
		case "block":
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "challenge":
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleAction("challenge")),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "js_challenge":
			ruleParam = rulesets.BlockRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleAction("js_challenge")),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "managed_challenge":
			ruleParam = rulesets.ManagedChallengeRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.ManagedChallengeRuleActionManagedChallenge),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		case "log":
			ruleParam = rulesets.LogRuleParam{
				ID:          cloudflare.F(existingRule.ID),
				Action:      cloudflare.F(rulesets.LogRuleActionLog),
				Description: cloudflare.F(existingRule.Description),
				Expression:  cloudflare.F(existingRule.Expression),
				Enabled:     cloudflare.F(existingRule.Enabled),
			}
		default:
			return fmt.Errorf("cannot preserve unsupported firewall rule action %q for rule %s", existingRule.Action, existingRule.ID)
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

// createRuleParamForAction creates the appropriate rule parameter type based on the action
func (c *CloudflareClient) createRuleParamForAction(action, id, description, expression string, enabled bool) (rulesets.PhaseUpdateParamsRuleUnion, error) {
	// Validate action type before processing
	validActions := map[string]bool{
		"log":               true,
		"block":             true,
		"challenge":         true,
		"js_challenge":      true,
		"managed_challenge": true,
	}
	if !validActions[action] {
		return nil, fmt.Errorf("unsupported action type: %s (valid: log, block, challenge, js_challenge, managed_challenge)", action)
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

// setRateLimitConfig sets the rate limiting configuration on a rule parameter and returns the updated rule
func (c *CloudflareClient) setRateLimitConfig(ruleParam rulesets.PhaseUpdateParamsRuleUnion, rateLimitConfig rulesets.BlockRuleRatelimitParam) (rulesets.PhaseUpdateParamsRuleUnion, error) {
	switch rule := ruleParam.(type) {
	case rulesets.LogRuleParam:
		rule.Ratelimit = cloudflare.F(rulesets.LogRuleRatelimitParam{
			Characteristics:   rateLimitConfig.Characteristics,
			Period:            rateLimitConfig.Period,
			RequestsPerPeriod: rateLimitConfig.RequestsPerPeriod,
			MitigationTimeout: rateLimitConfig.MitigationTimeout,
		})
		return rule, nil
	case rulesets.BlockRuleParam:
		rule.Ratelimit = cloudflare.F(rateLimitConfig)
		return rule, nil
	default:
		return nil, fmt.Errorf("unsupported rule parameter type: %T", ruleParam)
	}
}
