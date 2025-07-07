package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
	cfclient "github.com/wzygadlo/kubeflare-light/pkg/cloudflare/client"
)

const wafRuleFinalizer = "cloudflare.io/wafrule-finalizer"

// WAFRuleReconciler reconciles a WAFRule object
type WAFRuleReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	CF                *cfclient.CloudflareClient
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

	// Convert Kubernetes WAFRule to Cloudflare WAF Rule
	cfWAFRule := r.convertToCloudflareWAFRule(wafRule)

	if wafRule.Status.CloudflareID == "" {
		// Create new WAF rule in Cloudflare
		createdRule, err := r.CF.CreateFirewallRule(ctx, wafRule.Spec.ZoneID, cfWAFRule)
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
		// Update existing WAF rule in Cloudflare
		_, err := r.CF.UpdateFirewallRule(ctx, wafRule.Spec.ZoneID, wafRule.Status.CloudflareID, cfWAFRule)
		if err != nil {
			log.Error(err, "Failed to update WAF rule in Cloudflare")
			r.updateWAFCondition(wafRule, "Ready", metav1.ConditionFalse, "UpdateFailed", err.Error())
			r.Status().Update(ctx, wafRule)
			return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
		}

		wafRule.Status.ObservedGeneration = wafRule.Generation
		r.updateWAFCondition(wafRule, "Ready", metav1.ConditionTrue, "Updated", "WAF rule updated successfully in Cloudflare")

		log.Info("Updated WAF rule in Cloudflare", "cloudflareId", wafRule.Status.CloudflareID)
	}

	// Update status
	err := r.Status().Update(ctx, wafRule)
	if err != nil {
		log.Error(err, "Failed to update WAFRule status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
}

func (r *WAFRuleReconciler) finalizeWAFRule(ctx context.Context, wafRule *cloudflareV1alpha1.WAFRule) error {
	log := r.Log.WithValues("wafrule", wafRule.Name)

	if wafRule.Status.CloudflareID != "" {
		err := r.CF.DeleteFirewallRule(ctx, wafRule.Spec.ZoneID, wafRule.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to delete WAF rule from Cloudflare")
			return err
		}
		log.Info("Deleted WAF rule from Cloudflare", "cloudflareId", wafRule.Status.CloudflareID)
	}

	return nil
}

func (r *WAFRuleReconciler) convertToCloudflareWAFRule(wafRule *cloudflareV1alpha1.WAFRule) cfclient.FirewallRule {
	rule := cfclient.FirewallRule{
		ID:          wafRule.Status.CloudflareID,
		Name:        wafRule.Name,
		Description: wafRule.Spec.Description,
		Expression:  wafRule.Spec.Expression,
		Action:      wafRule.Spec.Action,
		Paused:      wafRule.Spec.Paused,
	}

	// Set priority if provided
	if wafRule.Spec.Priority != 0 {
		rule.Priority = wafRule.Spec.Priority
	}

	return rule
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
