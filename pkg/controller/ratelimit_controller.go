package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
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
	Log               logr.Logger
	Scheme            *runtime.Scheme
	CF                cfclient.CloudflareClientInterface
	ZoneResolver      *utils.ZoneResolver
	ReconcileInterval time.Duration
	ErrorInterval     time.Duration
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

	// Parse configurable intervals from environment variables
	reconcileInterval := parseInterval("RECONCILE_INTERVAL", "10m")
	errorInterval := parseInterval("ERROR_RETRY_INTERVAL", "5m")

	log.Log.WithName("setup").Info("Rate limit controller configuration",
		"reconcileInterval", reconcileInterval,
		"errorInterval", errorInterval)

	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudflareV1alpha1.RateLimit{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(&RateLimitReconciler{
			Client:            mgr.GetClient(),
			Log:               log.Log.WithName("controllers").WithName("RateLimit"),
			Scheme:            mgr.GetScheme(),
			CF:                cfClient,
			ZoneResolver:      utils.NewZoneResolver(mgr.GetClient()),
			ReconcileInterval: reconcileInterval,
			ErrorInterval:     errorInterval,
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
		return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
	}

	// Convert Kubernetes RateLimit to Cloudflare RateLimit using official 2025 API format
	cfRateLimit := cfclient.RateLimit{
		ID:          rateLimit.Status.CloudflareID,
		Name:        rateLimit.Name,
		Description: rateLimit.Name, // Use Kubernetes name as Cloudflare description for alignment
		Disabled:    rateLimit.Spec.Disabled,
		Threshold:   rateLimit.Spec.Threshold, // maps to requests_per_period
		Period:      rateLimit.Spec.Period,    // official period parameter
		Expression:  "true",                   // Default - will be built below
		Action:      "block",                  // Default action
	}

	// Set mitigation timeout from action spec
	// - timeout = 0: Throttle mode (block only excess requests) - Enterprise only for block action
	// - timeout > 0: Ban mode (block all requests from client for duration)
	if rateLimit.Spec.Action != nil {
		cfRateLimit.MitigationTimeout = rateLimit.Spec.Action.Timeout
	}

	// Set characteristics (how requests are grouped for rate limiting)
	if len(rateLimit.Spec.Characteristics) > 0 {
		characteristics := make([]string, len(rateLimit.Spec.Characteristics))
		copy(characteristics, rateLimit.Spec.Characteristics)
		hasColo := false
		for _, char := range characteristics {
			if char == "cf.colo.id" {
				hasColo = true
				break
			}
		}
		if !hasColo {
			characteristics = append(characteristics, "cf.colo.id")
		}
		cfRateLimit.Characteristics = characteristics
	} else {
		cfRateLimit.Characteristics = []string{"ip.src", "cf.colo.id"}
	}

	// Set action using official 2025 API actions
	if rateLimit.Spec.Action != nil {
		cfRateLimit.Action = rateLimit.Spec.Action.Mode // block, challenge, js_challenge, log
	}

	// Build expression using available operators. Never write a match-all rule
	// when the user supplied a match we cannot render.
	if rateLimit.Spec.Match != nil {
		if rateLimit.Spec.Match.Response != nil {
			return r.markRateLimitInvalid(ctx, rateLimit, "InvalidMatch", fmt.Errorf("match.response is not supported"))
		}
		if rateLimit.Spec.Match.Request == nil {
			return r.markRateLimitInvalid(ctx, rateLimit, "InvalidMatch", fmt.Errorf("match.request is required when match is set"))
		}
		expression, err := r.buildCloudflareExpression(rateLimit.Spec.Match.Request)
		if err != nil {
			return r.markRateLimitInvalid(ctx, rateLimit, "InvalidMatch", err)
		}
		cfRateLimit.Expression = expression
	}

	if rateLimit.Status.CloudflareID == "" {
		// Create new rate limit in Cloudflare
		resp, err := r.CF.CreateRateLimit(ctx, zoneID, cfRateLimit)
		if err != nil {
			log.Error(err, "Failed to create rate limit in Cloudflare")
			r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "CreateFailed", err.Error())
			r.Status().Update(ctx, rateLimit)
			return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
		}

		// Update status with Cloudflare ID
		rateLimit.Status.CloudflareID = resp.ID
		rateLimit.Status.ObservedGeneration = rateLimit.Generation
		r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Created", "Rate limit created successfully in Cloudflare")

		log.Info("Created rate limit in Cloudflare", "cloudflareId", resp.ID)
	} else {
		// Always check current state from Cloudflare to detect external changes (drift detection)
		currentCFRateLimit, err := r.CF.GetRateLimit(ctx, zoneID, rateLimit.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to get current rate limit from Cloudflare")
			// If rule was deleted in Cloudflare, recreate it
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404") {
				log.Info("Rate limit not found in Cloudflare, recreating", "cloudflareId", rateLimit.Status.CloudflareID)
				rateLimit.Status.CloudflareID = "" // Reset ID to trigger recreation
				rateLimit.Status.ObservedGeneration = 0
				return r.reconcileRateLimit(ctx, rateLimit) // Retry reconciliation
			}
			r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "GetFailed", err.Error())
			r.Status().Update(ctx, rateLimit)
			return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
		}

		// Check if update is needed by comparing desired vs current state
		needsUpdate := r.rateLimitNeedsUpdate(&cfRateLimit, currentCFRateLimit)
		specChanged := rateLimit.Status.ObservedGeneration != rateLimit.Generation

		if needsUpdate || specChanged {
			// Update existing rate limit in Cloudflare
			_, err := r.CF.UpdateRateLimit(ctx, zoneID, rateLimit.Status.CloudflareID, cfRateLimit)
			if err != nil {
				log.Error(err, "Failed to update rate limit in Cloudflare")
				r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, "UpdateFailed", err.Error())
				r.Status().Update(ctx, rateLimit)
				return ctrl.Result{RequeueAfter: r.ErrorInterval}, err
			}

			rateLimit.Status.ObservedGeneration = rateLimit.Generation
			r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Updated", "Rate limit updated successfully in Cloudflare")

			if specChanged {
				log.Info("Updated rate limit in Cloudflare due to spec change", "cloudflareId", rateLimit.Status.CloudflareID)
			} else {
				log.Info("Updated rate limit in Cloudflare due to drift", "cloudflareId", rateLimit.Status.CloudflareID)
			}

			// Update status after changes
			err = r.Status().Update(ctx, rateLimit)
			if err != nil {
				log.Error(err, "Failed to update RateLimit status")
				return ctrl.Result{}, err
			}

			// Requeue with longer delay to account for Cloudflare API consistency
			log.Info("Requeuing in 2m to verify update", "ratelimit", rateLimit.Name)
			return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
		} else {
			// No update needed, mark as observed if spec changed
			statusNeedsUpdate := false
			if specChanged {
				rateLimit.Status.ObservedGeneration = rateLimit.Generation
				statusNeedsUpdate = true
			}

			// Only update condition if it's not already "Synced"
			if len(rateLimit.Status.Conditions) == 0 ||
				rateLimit.Status.Conditions[len(rateLimit.Status.Conditions)-1].Reason != "Synced" {
				r.updateCondition(rateLimit, "Ready", metav1.ConditionTrue, "Synced", "Rate limit is in sync with Cloudflare")
				statusNeedsUpdate = true
			}

			// Only update status if there are actual changes
			if statusNeedsUpdate {
				err = r.Status().Update(ctx, rateLimit)
				if err != nil {
					log.Error(err, "Failed to update RateLimit status")
					return ctrl.Result{}, err
				}
			}

			log.V(1).Info("Rate limit is in sync with Cloudflare", "cloudflareId", rateLimit.Status.CloudflareID)
			log.V(1).Info("Rate limit in sync, requeueing for drift check", "ratelimit", rateLimit.Name)
			return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
		}
	}

	// For creation case, always use the normal reconcile interval
	err = r.Status().Update(ctx, rateLimit)
	if err != nil {
		log.Error(err, "Failed to update RateLimit status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
}

func (r *RateLimitReconciler) markRateLimitInvalid(ctx context.Context, rateLimit *cloudflareV1alpha1.RateLimit, reason string, err error) (ctrl.Result, error) {
	if !r.updateCondition(rateLimit, "Ready", metav1.ConditionFalse, reason, err.Error()) {
		return ctrl.Result{}, nil
	}
	if updateErr := r.Status().Update(ctx, rateLimit); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{}, nil
}

func (r *RateLimitReconciler) finalizeRateLimit(ctx context.Context, rateLimit *cloudflareV1alpha1.RateLimit) error {
	log := r.Log.WithValues("ratelimit", rateLimit.Name)

	if rateLimit.Status.CloudflareID != "" {
		// Resolve zone ID from ConfigMap if needed (same as in reconcile)
		zoneID, err := r.ZoneResolver.ResolveZoneID(ctx, rateLimit.Spec.ZoneID)
		if err != nil {
			log.Error(err, "Failed to resolve zone ID during finalization")
			return err
		}

		err = r.CF.DeleteRateLimit(ctx, zoneID, rateLimit.Status.CloudflareID)
		if err != nil {
			log.Error(err, "Failed to delete rate limit from Cloudflare")
			return err
		}
		log.Info("Deleted rate limit from Cloudflare", "cloudflareId", rateLimit.Status.CloudflareID, "zoneId", zoneID)
	}

	return nil
}

// buildCloudflareExpression builds a Cloudflare expression from CRD fields
// Uses official 2025 Cloudflare operators and syntax with proper input sanitization
func (r *RateLimitReconciler) buildCloudflareExpression(req *cloudflareV1alpha1.RateLimitMatchRequest) (string, error) {
	var expressions []string

	// === URL Operators (All Plans) ===

	// Legacy URLPattern (for backward compatibility)
	if req.URLPattern != "" {
		expressions = append(expressions, fmt.Sprintf("(http.request.uri wildcard \"%s\")",
			r.sanitizeString(req.URLPattern)))
	}

	// Single URL operators
	if req.URLWildcard != "" {
		expressions = append(expressions, fmt.Sprintf("(http.request.uri wildcard \"%s\")",
			r.sanitizeString(req.URLWildcard)))
	}
	if req.URLStartsWith != "" {
		expressions = append(expressions, fmt.Sprintf("(starts_with(http.request.uri, \"%s\"))",
			r.sanitizeString(req.URLStartsWith)))
	}
	if req.URLEndsWith != "" {
		expressions = append(expressions, fmt.Sprintf("(ends_with(http.request.uri, \"%s\"))",
			r.sanitizeString(req.URLEndsWith)))
	}
	if req.URLContains != "" {
		expressions = append(expressions, fmt.Sprintf("(contains(http.request.uri, \"%s\"))",
			r.sanitizeString(req.URLContains)))
	}
	if req.URLEquals != "" {
		expressions = append(expressions, fmt.Sprintf("(http.request.uri eq \"%s\")",
			r.sanitizeString(req.URLEquals)))
	}
	if req.URLMatches != "" {
		expressions = append(expressions, fmt.Sprintf("(http.request.uri matches \"%s\")",
			r.sanitizeString(req.URLMatches)))
	}

	// Multiple URL operators (OR conditions)
	if len(req.URLWildcardAny) > 0 {
		urlExprs := make([]string, len(req.URLWildcardAny))
		for i, pattern := range req.URLWildcardAny {
			urlExprs[i] = fmt.Sprintf("(http.request.uri wildcard \"%s\")", r.sanitizeString(pattern))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(urlExprs, " or ")))
	}
	if len(req.URLStartsWithAny) > 0 {
		urlExprs := make([]string, len(req.URLStartsWithAny))
		for i, prefix := range req.URLStartsWithAny {
			urlExprs[i] = fmt.Sprintf("(starts_with(http.request.uri, \"%s\"))", r.sanitizeString(prefix))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(urlExprs, " or ")))
	}
	if len(req.URLEndsWithAny) > 0 {
		urlExprs := make([]string, len(req.URLEndsWithAny))
		for i, suffix := range req.URLEndsWithAny {
			urlExprs[i] = fmt.Sprintf("(ends_with(http.request.uri, \"%s\"))", r.sanitizeString(suffix))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(urlExprs, " or ")))
	}
	if len(req.URLContainsAny) > 0 {
		urlExprs := make([]string, len(req.URLContainsAny))
		for i, substring := range req.URLContainsAny {
			urlExprs[i] = fmt.Sprintf("(contains(http.request.uri, \"%s\"))", r.sanitizeString(substring))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(urlExprs, " or ")))
	}
	if len(req.URLEqualsAny) > 0 {
		urlExprs := make([]string, len(req.URLEqualsAny))
		for i, path := range req.URLEqualsAny {
			urlExprs[i] = fmt.Sprintf("(http.request.uri eq \"%s\")", r.sanitizeString(path))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(urlExprs, " or ")))
	}
	if len(req.URLMatchesAny) > 0 {
		urlExprs := make([]string, len(req.URLMatchesAny))
		for i, regex := range req.URLMatchesAny {
			urlExprs[i] = fmt.Sprintf("(http.request.uri matches \"%s\")", r.sanitizeString(regex))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(urlExprs, " or ")))
	}

	// === HTTP Methods ===
	if len(req.Methods) > 0 {
		methodExprs := make([]string, len(req.Methods))
		for i, method := range req.Methods {
			methodExprs[i] = fmt.Sprintf("(http.request.method eq \"%s\")", r.sanitizeString(method))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(methodExprs, " or ")))
	}

	// === HTTP Schemes ===
	if len(req.Schemes) > 0 {
		schemeExprs := make([]string, len(req.Schemes))
		for i, scheme := range req.Schemes {
			schemeExprs[i] = fmt.Sprintf("(http.request.uri.scheme eq \"%s\")", r.sanitizeString(scheme))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(schemeExprs, " or ")))
	}

	// === Header Matching ===
	if len(req.Headers) > 0 {
		for headerName, rule := range req.Headers {
			expr := r.buildHeaderExpression(headerName, rule)
			if expr != "" {
				expressions = append(expressions, expr)
			}
		}
	}
	if len(req.HeadersAny) > 0 {
		var headerExprs []string
		for headerName, rule := range req.HeadersAny {
			expr := r.buildHeaderExpression(headerName, rule)
			if expr != "" {
				headerExprs = append(headerExprs, expr)
			}
		}
		if len(headerExprs) > 0 {
			expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(headerExprs, " or ")))
		}
	}

	// Special headers
	if req.Host != "" {
		expressions = append(expressions, fmt.Sprintf("(http.host eq \"%s\")", r.sanitizeString(req.Host)))
	}
	if len(req.HostAny) > 0 {
		hostExprs := make([]string, len(req.HostAny))
		for i, host := range req.HostAny {
			hostExprs[i] = fmt.Sprintf("(http.host eq \"%s\")", r.sanitizeString(host))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(hostExprs, " or ")))
	}
	if req.UserAgent != "" {
		expressions = append(expressions, fmt.Sprintf("(contains(lower(http.user_agent), \"%s\"))",
			r.sanitizeString(strings.ToLower(req.UserAgent))))
	}
	if req.Referer != "" {
		expressions = append(expressions, fmt.Sprintf("(http.referer eq \"%s\")", r.sanitizeString(req.Referer)))
	}

	// === Cookie Matching ===
	if len(req.Cookies) > 0 {
		for cookieName, rule := range req.Cookies {
			expr := r.buildCookieExpression(cookieName, rule)
			if expr != "" {
				expressions = append(expressions, expr)
			}
		}
	}

	// === Query Parameters ===
	if len(req.QueryParams) > 0 {
		for paramName, rule := range req.QueryParams {
			expr := r.buildQueryParamExpression(paramName, rule)
			if expr != "" {
				expressions = append(expressions, expr)
			}
		}
	}

	// === IP and Geography ===
	if len(req.IPSources) > 0 {
		if len(req.IPSources) == 1 {
			expressions = append(expressions, fmt.Sprintf("(ip.src eq %s)", req.IPSources[0]))
		} else {
			ipExprs := make([]string, len(req.IPSources))
			for i, ip := range req.IPSources {
				ipExprs[i] = fmt.Sprintf("(ip.src eq %s)", ip)
			}
			expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(ipExprs, " or ")))
		}
	}
	if len(req.IPSourcesExclude) > 0 {
		if len(req.IPSourcesExclude) == 1 {
			expressions = append(expressions, fmt.Sprintf("(ip.src ne %s)", req.IPSourcesExclude[0]))
		} else {
			ipExprs := make([]string, len(req.IPSourcesExclude))
			for i, ip := range req.IPSourcesExclude {
				ipExprs[i] = fmt.Sprintf("(ip.src ne %s)", ip)
			}
			expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(ipExprs, " and ")))
		}
	}
	if len(req.Countries) > 0 {
		countryExprs := make([]string, len(req.Countries))
		for i, country := range req.Countries {
			countryExprs[i] = fmt.Sprintf("(ip.geoip.country eq \"%s\")", r.sanitizeString(country))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(countryExprs, " or ")))
	}
	if len(req.CountriesExclude) > 0 {
		countryExprs := make([]string, len(req.CountriesExclude))
		for i, country := range req.CountriesExclude {
			countryExprs[i] = fmt.Sprintf("(ip.geoip.country ne \"%s\")", r.sanitizeString(country))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(countryExprs, " and ")))
	}
	if len(req.ASNs) > 0 {
		asnExprs := make([]string, len(req.ASNs))
		for i, asn := range req.ASNs {
			asnExprs[i] = fmt.Sprintf("(ip.geoip.asnum eq %d)", asn)
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(asnExprs, " or ")))
	}

	// === Request Body (Enterprise) ===
	if req.BodySize != "" {
		expr := r.buildComparisonExpression("http.request.body.size", req.BodySize)
		if expr != "" {
			expressions = append(expressions, expr)
		}
	}
	if req.BodyContains != "" {
		expressions = append(expressions, fmt.Sprintf("(contains(http.request.body.raw, \"%s\"))",
			r.sanitizeString(req.BodyContains)))
	}
	if len(req.BodyFormFields) > 0 {
		for fieldName, rule := range req.BodyFormFields {
			expr := r.buildFormFieldExpression(fieldName, rule)
			if expr != "" {
				expressions = append(expressions, expr)
			}
		}
	}
	if len(req.BodyJSONFields) > 0 {
		for jsonPath, rule := range req.BodyJSONFields {
			expr := r.buildJSONFieldExpression(jsonPath, rule)
			if expr != "" {
				expressions = append(expressions, expr)
			}
		}
	}

	// === Bot Management (Enterprise) ===
	if req.BotScore != "" {
		expr := r.buildComparisonExpression("cf.bot_management.score", req.BotScore)
		if expr != "" {
			expressions = append(expressions, expr)
		}
	}
	if req.VerifiedBot != nil {
		if *req.VerifiedBot {
			expressions = append(expressions, "(cf.bot_management.verified_bot)")
		} else {
			expressions = append(expressions, "(not cf.bot_management.verified_bot)")
		}
	}
	if len(req.JA3Fingerprint) > 0 {
		ja3Exprs := make([]string, len(req.JA3Fingerprint))
		for i, fingerprint := range req.JA3Fingerprint {
			ja3Exprs[i] = fmt.Sprintf("(cf.bot_management.ja3_hash eq \"%s\")", r.sanitizeString(fingerprint))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(ja3Exprs, " or ")))
	}
	if len(req.JA4Fingerprint) > 0 {
		ja4Exprs := make([]string, len(req.JA4Fingerprint))
		for i, fingerprint := range req.JA4Fingerprint {
			ja4Exprs[i] = fmt.Sprintf("(cf.bot_management.ja4 eq \"%s\")", r.sanitizeString(fingerprint))
		}
		expressions = append(expressions, fmt.Sprintf("(%s)", strings.Join(ja4Exprs, " or ")))
	}

	// === Custom Expression ===
	if req.CustomExpression != "" {
		// Validate and wrap custom expression
		customExpr := strings.TrimSpace(req.CustomExpression)
		if customExpr != "" {
			if !strings.HasPrefix(customExpr, "(") {
				customExpr = "(" + customExpr + ")"
			}
			expressions = append(expressions, customExpr)
		}
	}

	// Combine all expressions with AND
	if len(expressions) == 0 {
		return "", fmt.Errorf("match.request must contain at least one supported predicate")
	}
	if len(expressions) == 1 {
		return expressions[0], nil
	}
	return strings.Join(expressions, " and "), nil
}

// Helper functions for expression building
func (r *RateLimitReconciler) sanitizeString(input string) string {
	// Escape quotes and prevent injection
	return strings.ReplaceAll(input, "\"", "\\\"")
}

func (r *RateLimitReconciler) buildHeaderExpression(headerName, rule string) string {
	parts := strings.SplitN(rule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	operator, value := parts[0], parts[1]
	headerName = r.sanitizeString(strings.ToLower(headerName))
	value = r.sanitizeString(value)

	switch operator {
	case "eq":
		return fmt.Sprintf("(http.request.headers[\"%s\"][0] eq \"%s\")", headerName, value)
	case "contains":
		return fmt.Sprintf("(contains(http.request.headers[\"%s\"][0], \"%s\"))", headerName, value)
	case "exists":
		return fmt.Sprintf("(http.request.headers[\"%s\"][0] ne \"\")", headerName)
	default:
		return ""
	}
}

func (r *RateLimitReconciler) buildCookieExpression(cookieName, rule string) string {
	parts := strings.SplitN(rule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	operator, value := parts[0], parts[1]
	cookieName = r.sanitizeString(cookieName)
	value = r.sanitizeString(value)

	switch operator {
	case "eq":
		return fmt.Sprintf("(http.cookie[\"%s\"] eq \"%s\")", cookieName, value)
	case "contains":
		return fmt.Sprintf("(contains(http.cookie[\"%s\"], \"%s\"))", cookieName, value)
	case "exists":
		return fmt.Sprintf("(http.cookie[\"%s\"] ne \"\")", cookieName)
	default:
		return ""
	}
}

func (r *RateLimitReconciler) buildQueryParamExpression(paramName, rule string) string {
	parts := strings.SplitN(rule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	operator, value := parts[0], parts[1]
	paramName = r.sanitizeString(paramName)
	value = r.sanitizeString(value)

	switch operator {
	case "eq":
		return fmt.Sprintf("(http.request.uri.args[\"%s\"][0] eq \"%s\")", paramName, value)
	case "contains":
		return fmt.Sprintf("(contains(http.request.uri.args[\"%s\"][0], \"%s\"))", paramName, value)
	case "exists":
		return fmt.Sprintf("(http.request.uri.args[\"%s\"][0] ne \"\")", paramName)
	default:
		return ""
	}
}

func (r *RateLimitReconciler) buildComparisonExpression(field, rule string) string {
	parts := strings.SplitN(rule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	operator, value := parts[0], parts[1]

	switch operator {
	case "eq":
		return fmt.Sprintf("(%s eq %s)", field, value)
	case "ne":
		return fmt.Sprintf("(%s ne %s)", field, value)
	case "gt":
		return fmt.Sprintf("(%s gt %s)", field, value)
	case "ge":
		return fmt.Sprintf("(%s ge %s)", field, value)
	case "lt":
		return fmt.Sprintf("(%s lt %s)", field, value)
	case "le":
		return fmt.Sprintf("(%s le %s)", field, value)
	default:
		return ""
	}
}

func (r *RateLimitReconciler) buildFormFieldExpression(fieldName, rule string) string {
	parts := strings.SplitN(rule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	operator, value := parts[0], parts[1]
	fieldName = r.sanitizeString(fieldName)
	value = r.sanitizeString(value)

	switch operator {
	case "eq":
		return fmt.Sprintf("(http.request.body.form[\"%s\"][0] eq \"%s\")", fieldName, value)
	case "contains":
		return fmt.Sprintf("(contains(http.request.body.form[\"%s\"][0], \"%s\"))", fieldName, value)
	case "exists":
		return fmt.Sprintf("(http.request.body.form[\"%s\"][0] ne \"\")", fieldName)
	default:
		return ""
	}
}

func (r *RateLimitReconciler) buildJSONFieldExpression(jsonPath, rule string) string {
	parts := strings.SplitN(rule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	operator, value := parts[0], parts[1]
	jsonPath = r.sanitizeString(jsonPath)
	value = r.sanitizeString(value)

	switch operator {
	case "eq":
		return fmt.Sprintf("(lookup_json_string(http.request.body.raw, \"%s\") eq \"%s\")", jsonPath, value)
	case "contains":
		return fmt.Sprintf("(contains(lookup_json_string(http.request.body.raw, \"%s\"), \"%s\"))", jsonPath, value)
	case "exists":
		return fmt.Sprintf("(lookup_json_string(http.request.body.raw, \"%s\") ne \"\")", jsonPath)
	default:
		return ""
	}
}

// rateLimitNeedsUpdate compares desired state with current Cloudflare state
func (r *RateLimitReconciler) rateLimitNeedsUpdate(desired, current *cfclient.RateLimit) bool {
	if current == nil {
		r.Log.Info("Rate limit update needed: current state is nil")
		return true // If we can't get current state, assume update is needed
	}

	// DEBUG: Log all field values for comparison
	r.Log.Info("Comparing rate limit states",
		"desired.Description", desired.Description, "current.Description", current.Description,
		"desired.Expression", desired.Expression, "current.Expression", current.Expression,
		"desired.Action", desired.Action, "current.Action", current.Action,
		"desired.Threshold", desired.Threshold, "current.Threshold", current.Threshold,
		"desired.Period", desired.Period, "current.Period", current.Period,
		"desired.MitigationTimeout", desired.MitigationTimeout, "current.MitigationTimeout", current.MitigationTimeout,
		"desired.Disabled", desired.Disabled, "current.Disabled", current.Disabled)

	// Compare key fields that would indicate a change is needed
	if desired.Description != current.Description {
		r.Log.Info("Rate limit update needed: description mismatch", "desired", desired.Description, "current", current.Description)
		return true
	}
	if desired.Expression != current.Expression {
		r.Log.Info("Rate limit update needed: expression mismatch", "desired", desired.Expression, "current", current.Expression)
		return true
	}
	if desired.Action != current.Action {
		r.Log.Info("Rate limit update needed: action mismatch", "desired", desired.Action, "current", current.Action)
		return true
	}
	if desired.Threshold != current.Threshold {
		r.Log.Info("Rate limit update needed: threshold mismatch", "desired", desired.Threshold, "current", current.Threshold)
		return true
	}
	if desired.Period != current.Period {
		r.Log.Info("Rate limit update needed: period mismatch", "desired", desired.Period, "current", current.Period)
		return true
	}
	// Skip MitigationTimeout comparison for log actions - Cloudflare ignores it and returns default
	if desired.Action != "log" && desired.MitigationTimeout != current.MitigationTimeout {
		r.Log.Info("Rate limit update needed: mitigation timeout mismatch", "desired", desired.MitigationTimeout, "current", current.MitigationTimeout)
		return true
	}
	if desired.Disabled != current.Disabled {
		r.Log.Info("Rate limit update needed: disabled mismatch", "desired", desired.Disabled, "current", current.Disabled)
		return true
	}

	// Compare characteristics (how requests are grouped for rate limiting)
	if len(desired.Characteristics) != len(current.Characteristics) {
		r.Log.Info("Rate limit update needed: characteristics length mismatch",
			"desired", desired.Characteristics, "current", current.Characteristics)
		return true
	}
	desiredChars := make(map[string]bool)
	for _, char := range desired.Characteristics {
		desiredChars[char] = true
	}
	for _, char := range current.Characteristics {
		if !desiredChars[char] {
			r.Log.Info("Rate limit update needed: characteristics mismatch",
				"desired", desired.Characteristics, "current", current.Characteristics)
			return true
		}
	}

	r.Log.Info("Rate limit no update needed: all fields match")
	return false // No differences found
}

// updateCondition updates the condition status of the RateLimit resource.
func (r *RateLimitReconciler) updateCondition(rateLimit *cloudflareV1alpha1.RateLimit, conditionType string, status metav1.ConditionStatus, reason, message string) bool {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	// Find existing condition or append new one
	for i, existingCondition := range rateLimit.Status.Conditions {
		if existingCondition.Type == conditionType {
			if existingCondition.Status == status && existingCondition.Reason == reason && existingCondition.Message == message {
				return false
			}
			if existingCondition.Status == status {
				condition.LastTransitionTime = existingCondition.LastTransitionTime
			}
			rateLimit.Status.Conditions[i] = condition
			return true
		}
	}

	rateLimit.Status.Conditions = append(rateLimit.Status.Conditions, condition)
	return true
}
