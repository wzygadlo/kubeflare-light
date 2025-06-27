package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudflare/cloudflare-go"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudflareV1alpha1 "github.com/wzygadlo/kubeflare-light/pkg/apis/cloudflare/v1alpha1"
	"github.com/wzygadlo/kubeflare-light/pkg/cloudflare/client"
)

// RateLimitReconciler reconciles a RateLimit object
type RateLimitReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	CloudflareAPI client.CloudflareClient
}

// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=ratelimits,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudflare.kubeflare.io,resources=ratelimits/status,verbs=get;update;patch

// Reconcile handles RateLimit resources
func (r *RateLimitReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling RateLimit", "namespacedName", req.NamespacedName)

	// Fetch the RateLimit instance
	rateLimit := &cloudflareV1alpha1.RateLimit{}
	err := r.Get(ctx, req.NamespacedName, rateLimit)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Check if the resource is being deleted
	if !rateLimit.ObjectMeta.DeletionTimestamp.IsZero() {
		// Resource is being deleted, clean up Cloudflare resources
		if rateLimit.Status.ID != "" {
			err := r.CloudflareAPI.DeleteRateLimit(ctx, rateLimit.Spec.ZoneID, rateLimit.Status.ID)
			if err != nil {
				logger.Error(err, "Failed to delete rate limit from Cloudflare")
				return ctrl.Result{}, err
			}
			logger.Info("Successfully deleted rate limit from Cloudflare")
		}
		return ctrl.Result{}, nil
	}

	// Convert our CRD to Cloudflare API format
	cfRateLimit, err := convertToCloudflareRateLimit(rateLimit)
	if err != nil {
		logger.Error(err, "Failed to convert RateLimit to Cloudflare format")
		return ctrl.Result{}, err
	}

	// Check if this is a new resource or an update
	if rateLimit.Status.ID == "" {
		// Create new rate limit in Cloudflare
		resp, err := r.CloudflareAPI.CreateRateLimit(ctx, rateLimit.Spec.ZoneID, cfRateLimit)
		if err != nil {
			logger.Error(err, "Failed to create rate limit in Cloudflare")
			setRateLimitCondition(rateLimit, metav1.ConditionFalse, "CreateFailed", err.Error())
			if err := r.Status().Update(ctx, rateLimit); err != nil {
				logger.Error(err, "Failed to update RateLimit status")
			}
			return ctrl.Result{}, err
		}

		// Update status with the Cloudflare ID
		rateLimit.Status.ID = resp.ID
		setRateLimitCondition(rateLimit, metav1.ConditionTrue, "Created", "Successfully created in Cloudflare")
		if err := r.Status().Update(ctx, rateLimit); err != nil {
			logger.Error(err, "Failed to update RateLimit status")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully created rate limit in Cloudflare", "ID", resp.ID)
	} else {
		// Update existing rate limit in Cloudflare
		_, err := r.CloudflareAPI.UpdateRateLimit(ctx, rateLimit.Spec.ZoneID, rateLimit.Status.ID, cfRateLimit)
		if err != nil {
			logger.Error(err, "Failed to update rate limit in Cloudflare")
			setRateLimitCondition(rateLimit, metav1.ConditionFalse, "UpdateFailed", err.Error())
			if err := r.Status().Update(ctx, rateLimit); err != nil {
				logger.Error(err, "Failed to update RateLimit status")
			}
			return ctrl.Result{}, err
		}

		setRateLimitCondition(rateLimit, metav1.ConditionTrue, "Updated", "Successfully updated in Cloudflare")
		if err := r.Status().Update(ctx, rateLimit); err != nil {
			logger.Error(err, "Failed to update RateLimit status")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully updated rate limit in Cloudflare", "ID", rateLimit.Status.ID)
	}

	return ctrl.Result{RequeueAfter: time.Minute * 30}, nil
}

// SetupRateLimitController sets up the controller with the Manager
func SetupRateLimitController(mgr ctrl.Manager) error {
	cfClient, err := client.NewCloudflareClient()
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudflareV1alpha1.RateLimit{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(&RateLimitReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			CloudflareAPI: cfClient,
		})
}
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

const rateLimitFinalizer = "cloudflare.io/ratelimit-finalizer"

// RateLimitReconciler reconciles a RateLimit object
type RateLimitReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	CF     *cloudflare.API
}

func SetupRateLimitController(mgr ctrl.Manager) error {
	// Initialize Cloudflare API client
	cfAPI, err := cloudflare.NewWithAPIToken(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare API client: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudflareV1alpha1.RateLimit{}).
		Complete(&RateLimitReconciler{
			Client: mgr.GetClient(),
			Log:    log.Log.WithName("controllers").WithName("RateLimit"),
			Scheme: mgr.GetScheme(),
			CF:     cfAPI,
		})
}

// +kubebuilder:rbac:groups=cloudflare.io,resources=ratelimits,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudflare.io,resources=ratelimits/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cloudflare.io,resources=ratelimits/finalizers,verbs=update

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

	// Convert Kubernetes RateLimit to Cloudflare RateLimit
	cfRateLimit := r.convertToCloudflareLegacyRateLimit(rateLimit)

	var cfID string
	var err error

	if rateLimit.Status.CloudflareID == "" {
		// Create new rate limit in Cloudflare
		cfID, err = r.createCloudflareLegacyRateLimit(ctx, rateLimit.Spec.ZoneID, cfRateLimit)
		if err != nil {
			log.Error(err, "Failed to create rate limit in Cloudflare")
			r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "CreateFailed", err.Error())
			r.Status().Update(ctx, rateLimit)
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}

		// Update status with Cloudflare ID
		rateLimit.Status.CloudflareID = cfID
		rateLimit.Status.ObservedGeneration = rateLimit.Generation
		r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Created", "Rate limit created successfully in Cloudflare")

		log.Info("Created rate limit in Cloudflare", "cloudflareId", cfID)
	} else {
		// Update existing rate limit in Cloudflare
		err = r.updateCloudflareLegacyRateLimit(ctx, rateLimit.Spec.ZoneID, rateLimit.Status.CloudflareID, cfRateLimit)
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
	err = r.Status().Update(ctx, rateLimit)
	if err != nil {
		log.Error(err, "Failed to update RateLimit status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}

func (r *RateLimitReconciler) finalizeRateLimit(ctx context.Context, rateLimit *cloudflareV1alpha1.RateLimit) error {
	log := r.Log.WithValues("ratelimit", rateLimit.Name)

	if rateLimit.Status.CloudflareID != "" {
		err := r.deleteCloudflareLegacyRateLimit(ctx, rateLimit.Spec.ZoneID, rateLimit.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to delete rate limit from Cloudflare")
			return err
		}
		log.Info("Deleted rate limit from Cloudflare", "cloudflareId", rateLimit.Status.CloudflareID)
	}

	return nil
}

func (r *RateLimitReconciler) convertToCloudflareLegacyRateLimit(rateLimit *cloudflareV1alpha1.RateLimit) cloudflare.RateLimit {
	cfRateLimit := cloudflare.RateLimit{
		ID:          rateLimit.Status.CloudflareID,
		Disabled:    rateLimit.Spec.Disabled,
		Description: rateLimit.Spec.Description,
		Match: cloudflare.RateLimitTrafficMatcher{
			Request: cloudflare.RateLimitRequestMatcher{
				Methods:    rateLimit.Spec.Match.Request.Methods,
				Schemes:    rateLimit.Spec.Match.Request.Schemes,
				URLPattern: rateLimit.Spec.Match.Request.URLPattern,
			},
			Response: cloudflare.RateLimitResponseMatcher{
				Statuses: rateLimit.Spec.Match.Response.StatusCodes,
				Headers:  make([]cloudflare.RateLimitResponseMatcherHeaders, 0),
			},
		},
		Threshold: rateLimit.Spec.Threshold,
		Period:    rateLimit.Spec.Period,
		Action: cloudflare.RateLimitAction{
			Mode:    rateLimit.Spec.Action.Mode,
			Timeout: rateLimit.Spec.Action.Timeout,
		},
	}

	// Convert headers
	for name, value := range rateLimit.Spec.Match.Response.Headers {
		cfRateLimit.Match.Response.Headers = append(cfRateLimit.Match.Response.Headers,
			cloudflare.RateLimitResponseMatcherHeaders{
				Name:  name,
				Value: value,
			})
	}

	// Convert action response if present
	if rateLimit.Spec.Action.Response != nil {
		cfRateLimit.Action.Response = &cloudflare.RateLimitActionResponse{
			ContentType: rateLimit.Spec.Action.Response.ContentType,
			Body:        rateLimit.Spec.Action.Response.Body,
		}
	}

	return cfRateLimit
}

func (r *RateLimitReconciler) createCloudflareLegacyRateLimit(ctx context.Context, zoneID string, rateLimit cloudflare.RateLimit) (string, error) {
	result, err := r.CF.CreateRateLimit(ctx, cloudflare.ZoneIdentifier(zoneID), rateLimit)
	if err != nil {
		return "", err
	}
	return result.ID, nil
}

func (r *RateLimitReconciler) updateCloudflareLegacyRateLimit(ctx context.Context, zoneID, rateLimitID string, rateLimit cloudflare.RateLimit) error {
	_, err := r.CF.UpdateRateLimit(ctx, cloudflare.ZoneIdentifier(zoneID), rateLimitID, rateLimit)
	return err
}

func (r *RateLimitReconciler) deleteCloudflareLegacyRateLimit(ctx context.Context, zoneID, rateLimitID string) error {
	return r.CF.DeleteRateLimit(ctx, cloudflare.ZoneIdentifier(zoneID), rateLimitID)
}

func (r *RateLimitReconciler) updateCondition(rateLimit *cloudflareV1alpha1.RateLimit, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one
	for i, existingCondition := range rateLimit.Status.Conditions {
		if existingCondition.Type == conditionType {
			rateLimit.Status.Conditions[i] = condition
			return
		}
	}
	rateLimit.Status.Conditions = append(rateLimit.Status.Conditions, condition)
}
// Helper functions

func convertToCloudflareRateLimit(rl *cloudflareV1alpha1.RateLimit) (cloudflare.RateLimit, error) {
	cfRL := cloudflare.RateLimit{
		Description: rl.Spec.Description,
		Threshold:   rl.Spec.Threshold,
		Period:      rl.Spec.Period,
		Action: cloudflare.RateLimitAction{
			Mode: rl.Spec.Action.Mode,
		},
		Disabled: rl.Spec.Disabled,
	}

	// Set timeout if provided
	if rl.Spec.Action.Timeout > 0 {
		cfRL.Action.Timeout = rl.Spec.Action.Timeout
	}

	// Set custom response if provided
	if rl.Spec.Action.Response != nil {
		cfRL.Action.Response = &cloudflare.RateLimitActionResponse{
			ContentType: rl.Spec.Action.Response.ContentType,
			Body:        rl.Spec.Action.Response.Body,
		}
	}

	// Set match criteria if provided
	if rl.Spec.Match.Request != nil {
		cfRL.Match = cloudflare.RateLimitMatch{
			Request: &cloudflare.RateLimitMatchRequest{},
		}

		if len(rl.Spec.Match.Request.Methods) > 0 {
			cfRL.Match.Request.Methods = rl.Spec.Match.Request.Methods
		}

		if len(rl.Spec.Match.Request.Schemes) > 0 {
			cfRL.Match.Request.Schemes = rl.Spec.Match.Request.Schemes
		}

		if rl.Spec.Match.Request.URLPattern != "" {
			cfRL.Match.Request.URL = rl.Spec.Match.Request.URLPattern
		}
	}

	return cfRL, nil
}

func setRateLimitCondition(rl *cloudflareV1alpha1.RateLimit, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: rl.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one
	for i, cond := range rl.Status.Conditions {
		if cond.Type == "Ready" {
			if cond.Status != status || cond.Reason != reason || cond.Message != message {
				rl.Status.Conditions[i] = condition
			}
			return
		}
	}

	// Condition not found, append new one
	rl.Status.Conditions = append(rl.Status.Conditions, condition)
}