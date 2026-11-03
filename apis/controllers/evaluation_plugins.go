package controllers

import (
	"fmt"

	"github.com/protengplus/proteng-conductor/config"
)

// validateEvaluationPlugins checks the optional BLX01 plugin selection of a job
// or configuration. No plugins keeps the inherited behaviour (the job ends after
// mutation). Only registered names are accepted, because each name becomes the
// RabbitMQ routing key evaluation.<plugin>.
func validateEvaluationPlugins(plugins []string) error {
	seen := make(map[string]bool, len(plugins))
	for _, plugin := range plugins {
		if plugin == "" {
			return fmt.Errorf("error: evaluation plugin name is empty")
		}
		if _, ok := config.EvaluationPluginSchemas[plugin]; !ok {
			return fmt.Errorf("error: unknown evaluation plugin %q", plugin)
		}
		if seen[plugin] {
			return fmt.Errorf("error: evaluation plugin %q is selected more than once", plugin)
		}
		seen[plugin] = true
	}
	return nil
}
