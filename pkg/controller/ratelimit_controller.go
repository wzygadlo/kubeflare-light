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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
	cfclient "github.com/wzygadlo/kubeflare-light/pkg/cloudflare/client"
	"github.com/wzygadlo/kubeflare-light/pkg/utils"
)

const rateLimitFinalizer = "cloudflare.io/ratelimit-finalizer"

// RateLimitReconciler reconciles a RateLimit object
type RateLimitReconciler struct {
	client.Client
	Log          logr.Logger
	Scheme       *runtime.Scheme
	CF           *cfclient.CloudflareClient
	ZoneResolver *utils.ZoneResolver
}

// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=ratelimits,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=ratelimits/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=ratelimits/finalizers,verbs=update

// SetupRateLimitController sets up the controller with the Manager
func SetupRateLimitController(mgr ctrl.Manager) error {
	// Initialize Cloudflare client
	cfClient, err := cfclient.NewClient(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudflareV1alpha1.RateLimit{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(&RateLimitReconciler{
			Client:       mgr.GetClient(),
			Log:          log.Log.WithName("controllers").WithName("RateLimit"),
			Scheme:       mgr.GetScheme(),
			CF:           cfClient,
			ZoneResolver: utils.NewZoneResolver(mgr.GetClient()),
		})
}

// Reconcile handles RateLimit resource reconciliation
func (r *RateLimitReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("ratelimit", req.NamespacedName)

	// Fetch the RateLimit instance
	rateLimit := &cloudflareV1alpha1.RateLimit{}
	err := r.Get(ctx, req.NamespacedName, rateLimit)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("RateLimit resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get RateLimit")
		return ctrl.Result{}, err
	}

	// Check if the RateLimit instance is marked to be deleted
	if rateLimit.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, add the finalizer if it doesn't exist
		if !controllerutil.ContainsFinalizer(rateLimit, rateLimitFinalizer) {
			controllerutil.AddFinalizer(rateLimit, rateLimitFinalizer)
			err := r.Update(ctx, rateLimit)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(rateLimit, rateLimitFinalizer) {
			// Handle external dependency cleanup
			if err := r.finalizeRateLimit(ctx, rateLimit); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer to allow deletion
			controllerutil.RemoveFinalizer(rateLimit, rateLimitFinalizer)
			err := r.Update(ctx, rateLimit)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Handle creation or update
	return r.reconcileRateLimit(ctx, rateLimit)
}

func (r *RateLimitReconciler) reconcileRateLimit(ctx context.Context, rateLimit *cloudflareV1alpha1.RateLimit) (ctrl.Result, error) {
	log := r.Log.WithValues("ratelimit", rateLimit.Name)

	// Resolve zone ID from ConfigMap if needed
	zoneID, err := r.ZoneResolver.ResolveZoneID(ctx, rateLimit.Spec.ZoneID)
	if err != nil {
		log.Error(err, "Failed to resolve zone ID")
		r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "ZoneResolutionFailed", err.Error())
		r.Status().Update(ctx, rateLimit)
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	// Convert Kubernetes RateLimit to Cloudflare RateLimit
	cfRateLimit := r.convertToCloudflareRateLimit(rateLimit)

	if rateLimit.Status.CloudflareID == "" {
		// Create new rate limit in Cloudflare
		resp, err := r.CF.CreateRateLimit(ctx, zoneID, cfRateLimit)
		if err != nil {
			log.Error(err, "Failed to create rate limit in Cloudflare")
			r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "CreateFailed", err.Error())
			r.Status().Update(ctx, rateLimit)
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}

		// Update status with Cloudflare ID
		rateLimit.Status.CloudflareID = resp.ID
		rateLimit.Status.ObservedGeneration = rateLimit.Generation
		r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Created", "Rate limit created successfully in Cloudflare")

		log.Info("Created rate limit in Cloudflare", "cloudflareId", resp.ID)
	} else {
		// Update existing rate limit in Cloudflare
		_, err := r.CF.UpdateRateLimit(ctx, zoneID, rateLimit.Status.CloudflareID, cfRateLimit)
		if err != nil {
			log.Error(err, "Failed to update rate limit in Cloudflare")
			r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "UpdateFailed", err.Error())
			r.Status().Update(ctx, rateLimit)
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}

		rateLimit.Status.ObservedGeneration = rateLimit.Generation
		r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Updated", "Rate limit updated successfully in Cloudflare")

		log.Info("Updated rate limit in Cloudflare", "cloudflareId", rateLimit.Status.CloudflareID)
	}

	// Update status
	err := r.Status().Update(ctx, rateLimit)
	if err != nil {
		log.Error(err, "Failed to update RateLimit status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *RateLimitReconciler) finalizeRateLimit(ctx context.Context, rateLimit *cloudflareV1alpha1.RateLimit) error {
	log := r.Log.WithValues("ratelimit", rateLimit.Name)

	if rateLimit.Status.CloudflareID != "" {
		err := r.CF.DeleteRateLimit(ctx, rateLimit.Spec.ZoneID, rateLimit.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to delete rate limit from Cloudflare")
			return err
		}
		log.Info("Deleted rate limit from Cloudflare", "cloudflareId", rateLimit.Status.CloudflareID)
	}

	return nil
}

// convertToCloudflareRateLimit converts our CRD to Cloudflare API format
func (r *RateLimitReconciler) convertToCloudflareRateLimit(rateLimit *cloudflareV1alpha1.RateLimit) cfclient.RateLimit {
	cfRateLimit := cfclient.RateLimit{
		ID:          rateLimit.Status.CloudflareID,
		Name:        rateLimit.Name,
		Description: rateLimit.Spec.Description,
		Disabled:    rateLimit.Spec.Disabled,
		Threshold:   rateLimit.Spec.Threshold,
		Period:      rateLimit.Spec.Period,
	}

	// Generate expression based on match criteria
	if rateLimit.Spec.Match != nil {
		// Convert match conditions to Cloudflare expression
		expression := ""

		// Add URL pattern if defined
		if rateLimit.Spec.Match.Request != nil && rateLimit.Spec.Match.Request.URLPattern != "" {
			expression = fmt.Sprintf("(http.request.uri.path wildcard \"%s\")", rateLimit.Spec.Match.Request.URLPattern)
		}

		// Add methods if defined
		if rateLimit.Spec.Match.Request != nil && len(rateLimit.Spec.Match.Request.Methods) > 0 {
			methodExpr := "("
			for i, method := range rateLimit.Spec.Match.Request.Methods {
				if i > 0 {
					methodExpr += " or "
				}
				methodExpr += fmt.Sprintf("http.request.method eq \"%s\"", method)
			}
			methodExpr += ")"

			if expression != "" {
				expression = fmt.Sprintf("(%s) and %s", expression, methodExpr)
			} else {
				expression = methodExpr
			}
		}

		// Add schemes if defined
		if rateLimit.Spec.Match.Request != nil && len(rateLimit.Spec.Match.Request.Schemes) > 0 {
			schemeExpr := "("
			for i, scheme := range rateLimit.Spec.Match.Request.Schemes {
				if i > 0 {
					schemeExpr += " or "
				}
				schemeExpr += fmt.Sprintf("http.request.scheme eq \"%s\"", scheme)
			}
			schemeExpr += ")"

			if expression != "" {
				expression = fmt.Sprintf("(%s) and %s", expression, schemeExpr)
			} else {
				expression = schemeExpr
			}
		}

		// Add response statuses if defined
		if rateLimit.Spec.Match.Response != nil && len(rateLimit.Spec.Match.Response.Statuses) > 0 {
			statusExpr := "("
			for i, status := range rateLimit.Spec.Match.Response.Statuses {
				if i > 0 {
					statusExpr += " or "
				}
				statusExpr += fmt.Sprintf("http.response.status eq %d", status)
			}
			statusExpr += ")"

			if expression != "" {
				expression = fmt.Sprintf("(%s) and %s", expression, statusExpr)
			} else {
				expression = statusExpr
			}
		}

		// Use the generated expression if not empty
		if expression != "" {
			cfRateLimit.Expression = expression
		} else {
			// Default expression matches all requests
			cfRateLimit.Expression = "true"
		}
	} else {
		// Default expression matches all requests
		cfRateLimit.Expression = "true"
	}

	// Set action based on action configuration
	if rateLimit.Spec.Action != nil {
		cfRateLimit.Action = rateLimit.Spec.Action.Mode
	} else {
		// Default action is block
		cfRateLimit.Action = "block"
	}
	return cfRateLimit
}

// updateCondition updates the condition status of the RateLimit resource
func (r *RateLimitReconciler) updateCondition(rateLimit *cloudflareV1alpha1.RateLimit, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Find existing condition or append new one
	found := false
	for i, existingCondition := range rateLimit.Status.Conditions {
		if existingCondition.Type == conditionType {
			rateLimit.Status.Conditions[i] = condition
			found = true
			break
		}
	}

	if !found {
		rateLimit.Status.Conditions = append(rateLimit.Status.Conditions, condition)
	}
}
