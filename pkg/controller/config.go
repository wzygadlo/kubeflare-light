package controller

import (
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// parseInterval parses a duration string with default fallback
func parseInterval(envVar, defaultValue string) time.Duration {
	value := os.Getenv(envVar)
	if value == "" {
		value = defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Log.WithName("setup").Error(err, "Failed to parse interval, using default", "envVar", envVar, "value", value, "default", defaultValue)
		duration, _ = time.ParseDuration(defaultValue)
	}

	return duration
}
