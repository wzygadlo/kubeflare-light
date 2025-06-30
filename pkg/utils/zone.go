package utils

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ZoneResolver handles zone ID resolution from ConfigMaps
type ZoneResolver struct {
	client    client.Client
	configMap string
	namespace string
}

// NewZoneResolver creates a new zone resolver
func NewZoneResolver(client client.Client) *ZoneResolver {
	configMapName := os.Getenv("ZONES_CONFIGMAP_NAME")
	if configMapName == "" {
		configMapName = "kubeflare-zones"
	}

	namespace := os.Getenv("ZONES_CONFIGMAP_NAMESPACE")
	if namespace == "" {
		namespace = "kubeflare-light-system"
	}

	return &ZoneResolver{
		client:    client,
		configMap: configMapName,
		namespace: namespace,
	}
}

// ResolveZoneID resolves a zone ID from the ConfigMap
// First tries to find a mapping for the given domain/key
// Falls back to "default" key if not found
// Finally falls back to DEFAULT_ZONE_ID environment variable
func (z *ZoneResolver) ResolveZoneID(ctx context.Context, domain string) (string, error) {
	// If domain is already a zone ID (starts with hex chars), return as-is
	if isZoneID(domain) {
		return domain, nil
	}

	// Try to get from ConfigMap
	configMap := &corev1.ConfigMap{}
	err := z.client.Get(ctx, types.NamespacedName{
		Name:      z.configMap,
		Namespace: z.namespace,
	}, configMap)

	if err != nil {
		// ConfigMap not found, try default zone ID
		defaultZoneID := os.Getenv("DEFAULT_ZONE_ID")
		if defaultZoneID != "" {
			return defaultZoneID, nil
		}
		return "", fmt.Errorf("failed to get zones ConfigMap and no default zone ID set: %w", err)
	}

	// Try to find specific domain mapping
	if zoneID, exists := configMap.Data[domain]; exists && zoneID != "" {
		return zoneID, nil
	}

	// Try default mapping
	if zoneID, exists := configMap.Data["default"]; exists && zoneID != "" {
		return zoneID, nil
	}

	// Fall back to environment variable
	defaultZoneID := os.Getenv("DEFAULT_ZONE_ID")
	if defaultZoneID != "" {
		return defaultZoneID, nil
	}

	return "", fmt.Errorf("no zone ID found for domain %s in ConfigMap %s/%s and no default zone ID set",
		domain, z.namespace, z.configMap)
}

// isZoneID checks if a string looks like a Cloudflare zone ID
// Zone IDs are 32-character hexadecimal strings
func isZoneID(s string) bool {
	if len(s) != 32 {
		return false
	}

	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}

	return true
}
