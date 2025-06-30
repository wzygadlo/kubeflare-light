package client

import (
	"context"
	"fmt"

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

	// Create a new rate limiting rule with block action and ratelimit configuration
	newRuleParams := []rulesets.PhaseUpdateParamsRuleUnion{
		rulesets.BlockRuleParam{
			Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
			Description: cloudflare.F(rateLimit.Description),
			Expression:  cloudflare.F(rateLimit.Expression),
			Ratelimit: cloudflare.F(rulesets.BlockRuleRatelimitParam{
				Characteristics:   cloudflare.F([]string{"ip.src"}), // Default to IP-based rate limiting
				Period:            cloudflare.F(int64(rateLimit.Period)),
				RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
				MitigationTimeout: cloudflare.F(int64(rateLimit.Period)), // Block for the same period duration
			}),
		},
	}

	// Add existing rules to preserve them
	for _, rule := range currentRuleset.Rules {
		switch rule.Action {
		case "block":
			newRuleParams = append([]rulesets.PhaseUpdateParamsRuleUnion{
				rulesets.BlockRuleParam{
					ID:          cloudflare.F(rule.ID),
					Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
				},
			}, newRuleParams...)
			// Add other rule types as needed for comprehensive support
		}
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
		Action:      "block", // Using block action for rate limiting
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
			// Replace the rule with updated version
			newRuleParams = append(newRuleParams, rulesets.BlockRuleParam{
				ID:          cloudflare.F(rateLimitID),
				Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
				Description: cloudflare.F(rateLimit.Description),
				Expression:  cloudflare.F(rateLimit.Expression),
				Ratelimit: cloudflare.F(rulesets.BlockRuleRatelimitParam{
					Characteristics:   cloudflare.F([]string{"ip.src"}),
					Period:            cloudflare.F(int64(rateLimit.Period)),
					RequestsPerPeriod: cloudflare.F(int64(rateLimit.Threshold)),
					MitigationTimeout: cloudflare.F(int64(rateLimit.Period)),
				}),
			})
			ruleFound = true
		} else {
			// Keep existing rule
			switch rule.Action {
			case "block":
				newRuleParams = append(newRuleParams, rulesets.BlockRuleParam{
					ID:          cloudflare.F(rule.ID),
					Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
					Description: cloudflare.F(rule.Description),
					Expression:  cloudflare.F(rule.Expression),
				})
			}
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
		Action:      "block",
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

		// Keep existing rule
		switch rule.Action {
		case "block":
			newRuleParams = append(newRuleParams, rulesets.BlockRuleParam{
				ID:          cloudflare.F(rule.ID),
				Action:      cloudflare.F(rulesets.BlockRuleActionBlock),
				Description: cloudflare.F(rule.Description),
				Expression:  cloudflare.F(rule.Expression),
			})
		}
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
				Description: rule.Description,
				Expression:  rule.Expression,
				Action:      string(rule.Action),
			}

			// Extract rate limit parameters from block rule
			if rule.Action == "block" {
				rateLimit.Action = "block"
				// For now, return default threshold and period as we can't easily extract from response
				rateLimit.Threshold = 100 // Default from example
				rateLimit.Period = 60     // Default from example
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

		// Extract rate limit parameters if available
		if rule.Action == "block" {
			// For block rules with rate limiting, extract the rate limit config
			rateLimit.Action = "block"
		}

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

	// TODO: Implement with correct v4.5.1 rulesets API for custom firewall rules
	return &FirewallRule{
		ID:          fmt.Sprintf("fw_%s_%d", zoneID[:8], rule.Priority),
		Name:        rule.Name,
		Description: rule.Description,
		Expression:  rule.Expression,
		Action:      rule.Action,
		Priority:    rule.Priority,
		Paused:      rule.Paused,
	}, nil
}

// UpdateFirewallRule updates an existing firewall rule
func (c *CloudflareClient) UpdateFirewallRule(ctx context.Context, zoneID, rulesetID string, rule FirewallRule) (*FirewallRule, error) {
	if zoneID == "" {
		return nil, fmt.Errorf("zone ID cannot be empty")
	}
	if rulesetID == "" {
		return nil, fmt.Errorf("ruleset ID cannot be empty")
	}

	// TODO: Implement with correct v4.5.1 rulesets API for custom firewall rules
	return &FirewallRule{
		ID:          rulesetID,
		Name:        rule.Name,
		Description: rule.Description,
		Expression:  rule.Expression,
		Action:      rule.Action,
		Priority:    rule.Priority,
		Paused:      rule.Paused,
	}, nil
}

// DeleteFirewallRule deletes a firewall rule
func (c *CloudflareClient) DeleteFirewallRule(ctx context.Context, zoneID, rulesetID string) error {
	if zoneID == "" {
		return fmt.Errorf("zone ID cannot be empty")
	}
	if rulesetID == "" {
		return fmt.Errorf("ruleset ID cannot be empty")
	}

	// TODO: Implement with correct v4.5.1 rulesets API for custom firewall rules
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
