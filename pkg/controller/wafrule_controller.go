package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
	cfclient "github.com/wzygadlo/kubeflare-light/pkg/cloudflare/client"
	"github.com/wzygadlo/kubeflare-light/pkg/utils"
)

const wafRuleFinalizer = "cloudflare.io/wafrule-finalizer"

// WAFRuleReconciler reconciles a WAFRule object
type WAFRuleReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	CF                cfclient.CloudflareClientInterface
	ZoneResolver      *utils.ZoneResolver
	ReconcileInterval time.Duration
	ErrorInterval     time.Duration
}

func SetupWAFRuleController(mgr ctrl.Manager) error {
	// Initialize Cloudflare client
	cfClient, err := cfclient.NewClient(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	// Parse configurable intervals from environment variables (shared with RateLimit controller)
	reconcileInterval := parseInterval("RECONCILE_INTERVAL", "10m")
	errorInterval := parseInterval("ERROR_RETRY_INTERVAL", "5m")

	log.Log.WithName("setup").Info("WAF rule controller configuration",
		"reconcileInterval", reconcileInterval,
		"errorInterval", errorInterval)

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudflareV1alpha1.WAFRule{}).
		Complete(&WAFRuleReconciler{
			Client:            mgr.GetClient(),
			Log:               log.Log.WithName("controllers").WithName("WAFRule"),
			Scheme:            mgr.GetScheme(),
			CF:                cfClient,
			ZoneResolver:      utils.NewZoneResolver(mgr.GetClient()),
			ReconcileInterval: reconcileInterval,
			ErrorInterval:     errorInterval,
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

	// Resolve zone ID from ConfigMap if needed
	zoneID, err := r.ZoneResolver.ResolveZoneID(ctx, wafRule.Spec.ZoneID)
	if err != nil {
		log.Error(err, "Failed to resolve zone ID")
		r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "ZoneResolutionFailed", err.Error())
		r.Status().Update(ctx, wafRule)
		return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
	}

	// Convert Kubernetes WAFRule to Cloudflare WAF Rule
	cfWAFRule := r.convertToCloudflareWAFRule(wafRule)

	// Cloudflare only honors a custom response for the "block" action; reject rather than
	// silently ignore a response configured on any other action.
	if cfWAFRule.Response != nil && wafRule.Spec.Action != "block" {
		return r.markWAFRuleInvalid(ctx, wafRule, "InvalidActionParameters", fmt.Errorf("actionParameters.response is only supported for action \"block\", got %q", wafRule.Spec.Action))
	}

	if wafRule.Status.CloudflareID == "" {
		// Create new WAF rule in Cloudflare
		createdRule, err := r.CF.CreateFirewallRule(ctx, zoneID, cfWAFRule)
		if err != nil {
			log.Error(err, "Failed to create WAF rule in Cloudflare")
			r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "CreateFailed", err.Error())
			r.Status().Update(ctx, wafRule)
			return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
		}

		// Update status with Cloudflare ID
		wafRule.Status.CloudflareID = createdRule.ID
		wafRule.Status.ObservedGeneration = wafRule.Generation
		r.updateWAFCondition(wafRule, "Ready", metav1.ConditionTrue, "Created", "WAF rule created successfully in Cloudflare")

		log.Info("Created WAF rule in Cloudflare", "cloudflareId", createdRule.ID)
	} else {
		// Always check current state from Cloudflare to detect external changes (drift detection)
		currentCFWAFRule, err := r.CF.GetFirewallRule(ctx, zoneID, wafRule.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to get current WAF rule from Cloudflare")
			// If rule was deleted in Cloudflare, recreate it
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
				log.Info("WAF rule not found in Cloudflare, recreating", "cloudflareId", wafRule.Status.CloudflareID)
				wafRule.Status.CloudflareID = "" // Reset ID to trigger recreation
				wafRule.Status.ObservedGeneration = 0
				return r.reconcileWAFRule(ctx, wafRule) // Retry reconciliation
			}
			r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "GetFailed", err.Error())
			r.Status().Update(ctx, wafRule)
			return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
		}

		// Check if update is needed by comparing desired vs current state
		needsUpdate := r.wafRuleNeedsUpdate(&cfWAFRule, currentCFWAFRule)
		specChanged := wafRule.Status.ObservedGeneration != wafRule.Generation

		if needsUpdate || specChanged {
			// Update existing WAF rule in Cloudflare
			_, err := r.CF.UpdateFirewallRule(ctx, zoneID, wafRule.Status.CloudflareID, cfWAFRule)
			if err != nil {
				log.Error(err, "Failed to update WAF rule in Cloudflare")
				r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "UpdateFailed", err.Error())
				r.Status().Update(ctx, wafRule)
				return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
			}

			wafRule.Status.ObservedGeneration = wafRule.Generation
			r.updateWAFCondition(wafRule, "Ready", metav1.ConditionTrue, "Updated", "WAF rule updated successfully in Cloudflare")

			if specChanged {
				log.Info("Updated WAF rule in Cloudflare due to spec change", "cloudflareId", wafRule.Status.CloudflareID)
			} else {
				log.Info("Updated WAF rule in Cloudflare due to drift", "cloudflareId", wafRule.Status.CloudflareID)
			}

			// Update status after changes
			err = r.Status().Update(ctx, wafRule)
			if err != nil {
				log.Error(err, "Failed to update WAFRule status")
				return ctrl.Result{}, err
			}

			// Requeue with longer delay to account for Cloudflare API consistency
			log.Info("Requeuing in 2m to verify update", "wafrule", wafRule.Name)
			return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
		} else {
			// No update needed, mark as observed if spec changed
			statusNeedsUpdate := false
			if specChanged {
				wafRule.Status.ObservedGeneration = wafRule.Generation
				statusNeedsUpdate = true
			}

			// Only update condition if it's not already "Synced"
			if len(wafRule.Status.Conditions) == 0 ||
				wafRule.Status.Conditions[len(wafRule.Status.Conditions)-1].Reason != "Synced" {
				r.updateWAFCondition(wafRule, "Ready", metav1.ConditionTrue, "Synced", "WAF rule is in sync with Cloudflare")
				statusNeedsUpdate = true
			}

			// Only update status if there are actual changes
			if statusNeedsUpdate {
				err = r.Status().Update(ctx, wafRule)
				if err != nil {
					log.Error(err, "Failed to update WAFRule status")
					return ctrl.Result{}, err
				}
			}

			log.V(1).Info("WAF rule is in sync with Cloudflare", "cloudflareId", wafRule.Status.CloudflareID)
			log.V(1).Info("WAF rule in sync, requeueing for drift check", "wafrule", wafRule.Name)
			return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
		}
	}

	// For creation case, always use the normal reconcile interval
	err = r.Status().Update(ctx, wafRule)
	if err != nil {
		log.Error(err, "Failed to update WAFRule status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
}

// markWAFRuleInvalid records why the spec can't be reconciled and requeues at the normal
// drift-check interval, so a WAFRule doesn't get stuck forever once its spec becomes invalid.
func (r *WAFRuleReconciler) markWAFRuleInvalid(ctx context.Context, wafRule *cloudflareV1alpha1.WAFRule, reason string, err error) (ctrl.Result, error) {
	if !r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, reason, err.Error()) {
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}
	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, r.Status().Update(ctx, wafRule)
}

func (r *WAFRuleReconciler) finalizeWAFRule(ctx context.Context, wafRule *cloudflareV1alpha1.WAFRule) error {
	log := r.Log.WithValues("wafrule", wafRule.Name)

	if wafRule.Status.CloudflareID != "" {
		// Resolve zone ID from ConfigMap if needed
		zoneID, err := r.ZoneResolver.ResolveZoneID(ctx, wafRule.Spec.ZoneID)
		if err != nil {
			log.Error(err, "Failed to resolve zone ID during cleanup")
			return err
		}

		err = r.CF.DeleteFirewallRule(ctx, zoneID, wafRule.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to delete WAF rule from Cloudflare")
			return err
		}
		log.Info("Deleted WAF rule from Cloudflare", "cloudflareId", wafRule.Status.CloudflareID)
	}

	return nil
}

func (r *WAFRuleReconciler) convertToCloudflareWAFRule(wafRule *cloudflareV1alpha1.WAFRule) cfclient.FirewallRule {
	description := wafRule.Spec.Description
	if description == "" {
		description = wafRule.Name
	}

	rule := cfclient.FirewallRule{
		ID:          wafRule.Status.CloudflareID,
		Name:        wafRule.Name,
		Description: description,
		Expression:  wafRule.Spec.Expression,
		Action:      wafRule.Spec.Action,
		Paused:      wafRule.Spec.Paused,
	}

	if wafRule.Spec.Priority != 0 {
		rule.Priority = wafRule.Spec.Priority
	}

	if wafRule.Spec.ActionParameters != nil && wafRule.Spec.ActionParameters.Response != nil {
		rule.Response = &cfclient.FirewallRuleResponse{
			StatusCode:  wafRule.Spec.ActionParameters.Response.StatusCode,
			ContentType: wafRule.Spec.ActionParameters.Response.ContentType,
			Content:     wafRule.Spec.ActionParameters.Response.Content,
		}
	}

	return rule
}

// updateWAFCondition updates the condition status of the WAFRule resource, returning whether
// anything changed. Delegates to apimachinery so persistent errors don't rewrite
// LastTransitionTime (and trigger a status write) on every identical retry.
func (r *WAFRuleReconciler) updateWAFCondition(wafRule *cloudflareV1alpha1.WAFRule, conditionType string, status metav1.ConditionStatus, reason, message string) bool {
	return apimeta.SetStatusCondition(&wafRule.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// wafRuleNeedsUpdate compares desired state with current Cloudflare state
func (r *WAFRuleReconciler) wafRuleNeedsUpdate(desired, current *cfclient.FirewallRule) bool {
	if current == nil {
		r.Log.Info("WAF rule update needed: current state is nil")
		return true // If we can't get current state, assume update is needed
	}

	// DEBUG: Log all field values for comparison
	r.Log.V(1).Info("Comparing WAF rule states",
		"desired.Description", desired.Description, "current.Description", current.Description,
		"desired.Expression", desired.Expression, "current.Expression", current.Expression,
		"desired.Action", desired.Action, "current.Action", current.Action,
		"desired.Paused", desired.Paused, "current.Paused", current.Paused)

	// Compare key fields that would indicate a change is needed
	if desired.Description != current.Description {
		r.Log.Info("WAF rule update needed: description mismatch", "desired", desired.Description, "current", current.Description)
		return true
	}
	if desired.Expression != current.Expression {
		r.Log.Info("WAF rule update needed: expression mismatch", "desired", desired.Expression, "current", current.Expression)
		return true
	}
	if desired.Action != current.Action {
		r.Log.Info("WAF rule update needed: action mismatch", "desired", desired.Action, "current", current.Action)
		return true
	}
	if desired.Paused != current.Paused {
		r.Log.Info("WAF rule update needed: paused mismatch", "desired", desired.Paused, "current", current.Paused)
		return true
	}
	if !firewallRuleResponsesEqual(desired.Response, current.Response) {
		r.Log.Info("WAF rule update needed: response mismatch", "desired", desired.Response, "current", current.Response)
		return true
	}

	r.Log.V(1).Info("WAF rule no update needed: all fields match")
	return false // No differences found
}

// firewallRuleResponsesEqual compares two possibly-nil custom responses.
func firewallRuleResponsesEqual(a, b *cfclient.FirewallRuleResponse) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
