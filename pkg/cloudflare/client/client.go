package client

import (
	"context"
	"fmt"
	"reflect"

	"github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/rulesets"
	"github.com/cloudflare/cloudflare-go/v4/zones"
)

// CloudflareClient wraps the official Cloudflare Go v4 client
type CloudflareClient struct {
	client *cloudflare.Client
}

// RateLimit represents a rate limiting rule structure compatible with v4 API
type RateLimit struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Expression  string                 `json:"expression"`
	Action      string                 `json:"action"`
	Threshold   int                    `json:"threshold"`
	Period      int                    `json:"period"`
	Disabled    bool                   `json:"disabled,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// FirewallRule represents a firewall rule structure compatible with v4 API
type FirewallRule struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Expression  string                 `json:"expression"`
	Action      string                 `json:"action"`
	Priority    int                    `json:"priority,omitempty"`
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
	CreateWAFRule(ctx context.Context, zoneID string, rule *FirewallRule) (*FirewallRule, error)
	GetWAFRule(ctx context.Context, zoneID, ruleID string) (*FirewallRule, error)
	UpdateWAFRule(ctx context.Context, zoneID, ruleID string, rule *FirewallRule) (*FirewallRule, error)
	DeleteWAFRule(ctx context.Context, zoneID, ruleID string) error
	ListWAFRules(ctx context.Context, zoneID string) ([]FirewallRule, error)
}

// NewClient creates a new Cloudflare v4 client with API token
func NewClient(apiToken string) (*CloudflareClient, error) {
	if apiToken == "" {
		return nil, fmt.Errorf("API token cannot be empty")
	}

	client := cloudflare.NewClient(
		option.WithAPIToken(apiToken),
	)

	return &CloudflareClient{
		client: client,
	}, nil
}

// GetClient returns the underlying Cloudflare v4 client
func (c *CloudflareClient) GetClient() *cloudflare.Client {
	return c.client
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

// CreateRateLimit creates a new rate limit rule using rulesets API
// CRITICAL: This now uses the modern rulesets API to replace the deprecated Rate Limiting API (sunset June 15, 2025)
func (c *CloudflareClient) CreateRateLimit(ctx context.Context, zoneID string, rateLimit RateLimit) (*RateLimit, error) {
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

	// Create a new rate limiting rule using BlockRuleParam but with dynamic action
	// Note: Using BlockRuleParam as the base type but changing the action field dynamically
	newRuleParams := []rulesets.PhaseUpdateParamsRuleUnion{
		rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleAction(rateLimit.Action)), // Use provided action (challenge, block, etc.)
			Description: cloudflare.F(rateLimit.Description),
			Expression:  cloudflare.F(rateLimit.Expression),
			Enabled:     cloudflare.F(!rateLimit.Disabled), // CRITICAL: Enabled field is required
			Ratelimit: cloudflare.F(rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F([]string{"ip.src", "cf.colo.id"}), // Required by Cloudflare API
				Period:            cloudflare.F(int64(rateLimit.Period)),
				RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
				MitigationTimeout: cloudflare.F(int64(rateLimit.Period)),
			}),
		},
	}

	// Add existing rules to preserve them (with their rate limiting configuration)
	for _, rule := range currentRuleset.Rules {
		// ALL rules in http_ratelimit phase MUST have ratelimit configuration
		// Skip any rules that don't have ratelimit (they shouldn't exist, but safety check)
		if rule.Ratelimit == nil {
			continue // Skip rules without ratelimit configuration
		}

		// All rate limiting rules use the same consistent structure regardless of action type
		// Based on official Cloudflare documentation: https://developers.cloudflare.com/waf/rate-limiting-rules/create-api/
		ruleParam := rulesets.BlockRuleParam{
			ID:          cloudflare.F(rule.ID),
			Action:      cloudflare.F(rulesets.BlockRuleAction(rule.Action)),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
			Enabled:     cloudflare.F(rule.Enabled), // CRITICAL: Preserve the enabled state
		}

		// CRITICAL: ALL rules in http_ratelimit phase MUST have ratelimit configuration
		// Try multiple type assertions to handle different action types
		rateLimitExtracted := false
		
		// Try BlockRuleRatelimit (for block actions)
		if blockRateLimit, ok := rule.Ratelimit.(rulesets.BlockRuleRatelimit); ok {
			ruleParam.Ratelimit = cloudflare.F(rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F(blockRateLimit.Characteristics),
				Period:            cloudflare.F(blockRateLimit.Period),
				RequestsPerPeriod: cloudflare.F(blockRateLimit.RequestsPerPeriod),
				MitigationTimeout: cloudflare.F(blockRateLimit.MitigationTimeout),
			})
			rateLimitExtracted = true
		}
		
		// If we couldn't extract the ratelimit config, use safe defaults
		// This ensures ALL rules have ratelimit configuration
		if !rateLimitExtracted {
			// Use default ratelimit configuration to ensure the rule is valid
			// These defaults match common rate limit settings
			ruleParam.Ratelimit = cloudflare.F(rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F([]string{"ip.src", "cf.colo.id"}),
				Period:            cloudflare.F(int64(60)),
				RequestsPerPeriod: cloudflare.F(int64(100)),
				MitigationTimeout: cloudflare.F(int64(60)),
			})
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
			newRuleParams = append(newRuleParams, rulesets.BlockRuleParam{
				ID:          cloudflare.F(rateLimitID),
				Action:      cloudflare.F(rulesets.BlockRuleAction(rateLimit.Action)), // Use provided action (challenge, block, etc.)
				Description: cloudflare.F(rateLimit.Description),
				Expression:  cloudflare.F(rateLimit.Expression),
				Enabled:     cloudflare.F(!rateLimit.Disabled), // CRITICAL: Enabled field is required
				Ratelimit: cloudflare.F(rulesets.BlockRuleRatelimitParam{
					Characteristics:   cloudflare.F([]string{"ip.src", "cf.colo.id"}), // Required by Cloudflare API
					Period:            cloudflare.F(int64(rateLimit.Period)),
					RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
					MitigationTimeout: cloudflare.F(int64(rateLimit.Period)),
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
				Description: rule.Description, // Keep description same as name for consistency
				Expression:  rule.Expression,
				Action:      string(rule.Action),
				Disabled:    !rule.Enabled, // Convert enabled to disabled
			}

			// Extract rate limit parameters from the ratelimit configuration
			// NOTE: All actions (block, challenge, js_challenge, managed_challenge, log) can have rate limiting
			if rule.Ratelimit != nil {
				// Try to cast to different ratelimit struct types
				extracted := false

				// Try BlockRuleRatelimit (for block actions)
				if blockRateLimit, ok := rule.Ratelimit.(rulesets.BlockRuleRatelimit); ok {
					rateLimit.Threshold = int(blockRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(blockRateLimit.Period)
					extracted = true
				} else if logRateLimit, ok := rule.Ratelimit.(rulesets.LogRuleRatelimit); ok {
					// Handle log action ratelimit type
					rateLimit.Threshold = int(logRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(logRateLimit.Period)
					extracted = true
				} else if managedChallengeRateLimit, ok := rule.Ratelimit.(rulesets.ManagedChallengeRuleRatelimit); ok {
					// Handle managed_challenge action ratelimit type
					rateLimit.Threshold = int(managedChallengeRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(managedChallengeRateLimit.Period)
					extracted = true
				} else if challengeRateLimit, ok := rule.Ratelimit.(rulesets.ChallengeRuleRatelimit); ok {
					// Handle challenge action ratelimit type
					rateLimit.Threshold = int(challengeRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(challengeRateLimit.Period)
					extracted = true
				} else if jsRateLimit, ok := rule.Ratelimit.(rulesets.JSChallengeRuleRatelimit); ok {
					// Handle js_challenge action ratelimit type
					rateLimit.Threshold = int(jsRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(jsRateLimit.Period)
					extracted = true
				} else if phaseGetRespRateLimit, ok := rule.Ratelimit.(rulesets.PhaseGetResponseRulesRulesetsLogRuleRatelimit); ok {
					// Handle PhaseGetResponse log ratelimit type (from API response)
					rateLimit.Threshold = int(phaseGetRespRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(phaseGetRespRateLimit.Period)
					extracted = true
				} else if phaseGetRespChallengeRateLimit, ok := rule.Ratelimit.(rulesets.PhaseGetResponseRulesRulesetsChallengeRuleRatelimit); ok {
					// Handle PhaseGetResponse challenge ratelimit type
					rateLimit.Threshold = int(phaseGetRespChallengeRateLimit.RequestsPerPeriod)
					rateLimit.Period = int(phaseGetRespChallengeRateLimit.Period)
					extracted = true
				} else {
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
		}
	case "challenge":
		// Use BlockRuleParam with challenge action
		newRuleParam = rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleAction("challenge")),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
		}
	case "js_challenge":
		// Use BlockRuleParam with js_challenge action
		newRuleParam = rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleAction("js_challenge")),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
		}
	case "managed_challenge":
		newRuleParam = rulesets.ManagedChallengeRuleParam{
			Action:      cloudflare.F(rulesets.ManagedChallengeRuleActionManagedChallenge),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
		}
	case "log":
		newRuleParam = rulesets.LogRuleParam{
			Action:      cloudflare.F(rulesets.LogRuleActionLog),
			Description: cloudflare.F(rule.Description),
			Expression:  cloudflare.F(rule.Expression),
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
			// Skip unsupported rule types
			continue
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
				// Skip unsupported rule types
				continue
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
			// Skip unsupported rule types
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
func (c *CloudflareClient) GetFirewallRule(ctx context.Context, zoneID, rulesetID string) (*FirewallRule, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if rulesetID == "" {
		return nil, fmt.Errorf("ruleset ID cannot be empty")
	}

	// TODO: Implement with correct v4.5.1 rulesets API for custom firewall rules
	return &FirewallRule{
		ID:          rulesetID,
		Name:        "Example WAF Rule",
		Description: "Example WAF rule",
		Expression:  "true",
		Action:      "block",
		Priority:    1,
		Paused:      false,
	}, nil
}

// ListFirewallRules lists all firewall rules for a zone
func (c *CloudflareClient) ListFirewallRules(ctx context.Context, zoneID string) ([]FirewallRule, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}

	// TODO: Implement with correct v4.5.1 rulesets API for custom firewall rules
	return []FirewallRule{}, nil
}
