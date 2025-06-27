package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
)

const wafRuleFinalizer = "cloudflare.io/wafrule-finalizer"

// WAFRuleReconciler reconciles a WAFRule object
type WAFRuleReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	CF     *cloudflare.API
}

func SetupWAFRuleController(mgr ctrl.Manager) error {
	// Initialize Cloudflare API client
	cfAPI, err := cloudflare.NewWithAPIToken(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare API client: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudflareV1alpha1.WAFRule{}).
		Complete(&WAFRuleReconciler{
			Client: mgr.GetClient(),
			Log:    log.Log.WithName("controllers").WithName("WAFRule"),
			Scheme: mgr.GetScheme(),
			CF:     cfAPI,
		})
}

// +kubebuilder:rbac:groups=cloudflare.io,resources=wafrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudflare.io,resources=wafrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cloudflare.io,resources=wafrules/finalizers,verbs=update

// Reconcile handles WAFRule resource reconciliation
func (r *WAFRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("wafrule", req.NamespacedName)

	// Fetch the WAFRule instance
	wafRule := &cloudflareV1alpha1.WAFRule{}
	err := r.Get(ctx, req.NamespacedName, wafRule)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("WAFRule resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get WAFRule")
		return ctrl.Result{}, err
	}

	// Check if the WAFRule instance is marked to be deleted
	if wafRule.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, add the finalizer if it doesn't exist
		if !controllerutil.ContainsFinalizer(wafRule, wafRuleFinalizer) {
			controllerutil.AddFinalizer(wafRule, wafRuleFinalizer)
			err := r.Update(ctx, wafRule)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(wafRule, wafRuleFinalizer) {
			// Handle external dependency cleanup
			if err := r.finalizeWAFRule(ctx, wafRule); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow deletion
			controllerutil.RemoveFinalizer(wafRule, wafRuleFinalizer)
			err := r.Update(ctx, wafRule)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Handle creation or update
	return r.reconcileWAFRule(ctx, wafRule)
}

func (r *WAFRuleReconciler) reconcileWAFRule(ctx context.Context, wafRule *cloudflareV1alpha1.WAFRule) (ctrl.Result, error) {
	log := r.Log.WithValues("wafrule", wafRule.Name)

	// Convert Kubernetes WAFRule to Cloudflare WAF Rule
	cfWAFRule := r.convertToCloudflareWAFRule(wafRule)

	var cfID string
	var err error

	if wafRule.Status.CloudflareID == "" {
		// Create new WAF rule in Cloudflare using Rulesets API
		cfID, err = r.createCloudflareWAFRule(ctx, wafRule.Spec.ZoneID, cfWAFRule)
		if err != nil {
			log.Error(err, "Failed to create WAF rule in Cloudflare")
			r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "CreateFailed", err.Error())
			r.Status().Update(ctx, wafRule)
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}

		// Update status with Cloudflare ID
		wafRule.Status.CloudflareID = cfID
		wafRule.Status.ObservedGeneration = wafRule.Generation
		r.updateWAFCondition(wafRule, "Ready", metav1.ConditionTrue, "Created", "WAF rule created successfully in Cloudflare")

		log.Info("Created WAF rule in Cloudflare", "cloudflareId", cfID)
	} else {
		// Update existing WAF rule in Cloudflare
		err = r.updateCloudflareWAFRule(ctx, wafRule.Spec.ZoneID, wafRule.Status.CloudflareID, cfWAFRule)
		if err != nil {
			log.Error(err, "Failed to update WAF rule in Cloudflare")
			r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "UpdateFailed", err.Error())
			r.Status().Update(ctx, wafRule)
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}

		wafRule.Status.ObservedGeneration = wafRule.Generation
		r.updateWAFCondition(wafRule, "Ready", metav1.ConditionTrue, "Updated", "WAF rule updated successfully in Cloudflare")

		log.Info("Updated WAF rule in Cloudflare", "cloudflareId", wafRule.Status.CloudflareID)
	}

	// Update status
	err = r.Status().Update(ctx, wafRule)
	if err != nil {
		log.Error(err, "Failed to update WAFRule status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *WAFRuleReconciler) finalizeWAFRule(ctx context.Context, wafRule *cloudflareV1alpha1.WAFRule) error {
	log := r.Log.WithValues("wafrule", wafRule.Name)

	if wafRule.Status.CloudflareID != "" {
		err := r.deleteCloudflareWAFRule(ctx, wafRule.Spec.ZoneID, wafRule.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to delete WAF rule from Cloudflare")
			return err
		}
		log.Info("Deleted WAF rule from Cloudflare", "cloudflareId", wafRule.Status.CloudflareID)
	}

	return nil
}

func (r *WAFRuleReconciler) convertToCloudflareWAFRule(wafRule *cloudflareV1alpha1.WAFRule) cloudflare.ZoneRulesetRule {
	rule := cloudflare.ZoneRulesetRule{
		ID:         wafRule.Status.CloudflareID,
		Expression: wafRule.Spec.Expression,
		Action:     wafRule.Spec.Action,
		Enabled:    !wafRule.Spec.Paused,
	}

	// Set description if provided
	if wafRule.Spec.Description != "" {
		rule.Description = wafRule.Spec.Description
	}

	// Set action parameters if provided
	if wafRule.Spec.ActionParameters != nil {
		rule.ActionParameters = &cloudflare.ZoneRulesetRuleActionParameters{}

		if wafRule.Spec.ActionParameters.ID != "" {
			rule.ActionParameters.ID = wafRule.Spec.ActionParameters.ID
		}

		if wafRule.Spec.ActionParameters.Overrides != nil {
			// Convert overrides - this would need proper type mapping based on Cloudflare API
			rule.ActionParameters.Overrides = wafRule.Spec.ActionParameters.Overrides
		}

		if wafRule.Spec.ActionParameters.Response != nil {
			rule.ActionParameters.Response = &cloudflare.ZoneRulesetRuleActionParametersResponse{
				StatusCode:  wafRule.Spec.ActionParameters.Response.StatusCode,
				ContentType: wafRule.Spec.ActionParameters.Response.ContentType,
				Content:     wafRule.Spec.ActionParameters.Response.Content,
			}
		}
	}

	return rule
}

func (r *WAFRuleReconciler) createCloudflareWAFRule(ctx context.Context, zoneID string, rule cloudflare.ZoneRulesetRule) (string, error) {
	// First, get the zone's WAF managed ruleset
	rulesets, err := r.CF.ListZoneRulesets(ctx, cloudflare.ZoneIdentifier(zoneID))
	if err != nil {
		return "", fmt.Errorf("failed to list zone rulesets: %w", err)
	}

	var wafRulesetID string
	for _, rs := range rulesets {
		if rs.Phase == "http_request_firewall_custom" {
			wafRulesetID = rs.ID
			break
		}
	}

	if wafRulesetID == "" {
		return "", fmt.Errorf("WAF custom ruleset not found for zone %s", zoneID)
	}

	// Create the rule in the WAF ruleset
	createParams := cloudflare.CreateZoneRulesetRuleParams{
		ZoneID:    zoneID,
		RulesetID: wafRulesetID,
		Rule:      rule,
	}

	result, err := r.CF.CreateZoneRulesetRule(ctx, &createParams)
	if err != nil {
		return "", fmt.Errorf("failed to create WAF rule: %w", err)
	}

	return result.ID, nil
}

func (r *WAFRuleReconciler) updateCloudflareWAFRule(ctx context.Context, zoneID, ruleID string, rule cloudflare.ZoneRulesetRule) error {
	// First, get the zone's WAF managed ruleset
	rulesets, err := r.CF.ListZoneRulesets(ctx, cloudflare.ZoneIdentifier(zoneID))
	if err != nil {
		return fmt.Errorf("failed to list zone rulesets: %w", err)
	}

	var wafRulesetID string
	for _, rs := range rulesets {
		if rs.Phase == "http_request_firewall_custom" {
			wafRulesetID = rs.ID
			break
		}
	}

	if wafRulesetID == "" {
		return fmt.Errorf("WAF custom ruleset not found for zone %s", zoneID)
	}

	// Update the rule in the WAF ruleset
	updateParams := cloudflare.UpdateZoneRulesetRuleParams{
		ZoneID:    zoneID,
		RulesetID: wafRulesetID,
		RuleID:    ruleID,
		Rule:      rule,
	}

	_, err = r.CF.UpdateZoneRulesetRule(ctx, &updateParams)
	if err != nil {
		return fmt.Errorf("failed to update WAF rule: %w", err)
	}

	return nil
}

func (r *WAFRuleReconciler) deleteCloudflareWAFRule(ctx context.Context, zoneID, ruleID string) error {
	// First, get the zone's WAF managed ruleset
	rulesets, err := r.CF.ListZoneRulesets(ctx, cloudflare.ZoneIdentifier(zoneID))
	if err != nil {
		return fmt.Errorf("failed to list zone rulesets: %w", err)
	}

	var wafRulesetID string
	for _, rs := range rulesets {
		if rs.Phase == "http_request_firewall_custom" {
			wafRulesetID = rs.ID
			break
		}
	}

	if wafRulesetID == "" {
		return fmt.Errorf("WAF custom ruleset not found for zone %s", zoneID)
	}

	// Delete the rule from the WAF ruleset
	deleteParams := cloudflare.DeleteZoneRulesetRuleParams{
		ZoneID:    zoneID,
		RulesetID: wafRulesetID,
		RuleID:    ruleID,
	}

	err = r.CF.DeleteZoneRulesetRule(ctx, &deleteParams)
	if err != nil {
		return fmt.Errorf("failed to delete WAF rule: %w", err)
	}

	return nil
}

func (r *WAFRuleReconciler) updateWAFCondition(wafRule *cloudflareV1alpha1.WAFRule, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one
	for i, existingCondition := range wafRule.Status.Conditions {
		if existingCondition.Type == conditionType {
			wafRule.Status.Conditions[i] = condition
			return
		}
	}
	wafRule.Status.Conditions = append(wafRule.Status.Conditions, condition)
}
