package models

import (
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EvaluationState string

const (
	EvaluationOngoing   EvaluationState = "ONGOING"
	EvaluationCompleted EvaluationState = "COMPLETED"
	EvaluationFailed    EvaluationState = "FAILED"
)

// EvaluationRun snapshots the candidates so callbacks cannot add unrelated mutants.
// Its plugins run one at a time in order; CurrentPluginIndex is the running one.
type EvaluationRun struct {
	Id                 primitive.ObjectID   `bson:"_id" json:"id"`
	JobId              primitive.ObjectID   `bson:"job_id" json:"job_id"`
	MutationId         primitive.ObjectID   `bson:"mutation_id" json:"mutation_id"`
	MutationResultIds  []primitive.ObjectID `bson:"mutation_result_ids" json:"mutation_result_ids"`
	Plugins            []string             `bson:"plugins" json:"plugins"`
	CurrentPluginIndex int                  `bson:"current_plugin_index" json:"current_plugin_index"`
	// PluginStartedAt is when the current plugin was dispatched; the timeout counts from it.
	PluginStartedAt time.Time `bson:"plugin_started_at" json:"plugin_started_at"`
	// PluginErrors lists "plugin: reason" for every plugin that failed so far.
	PluginErrors []string        `bson:"plugin_errors,omitempty" json:"plugin_errors,omitempty"`
	State        EvaluationState `bson:"state" json:"state"`
	Error        string          `bson:"error" json:"error"`
	CreatedAt    time.Time       `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time       `bson:"complete_at" json:"complete_at"`
}

type EvaluationResult struct {
	Id               primitive.ObjectID     `bson:"_id" json:"id"`
	EvaluationRunId  primitive.ObjectID     `bson:"evaluation_run_id" json:"evaluation_run_id"`
	MutationResultId primitive.ObjectID     `bson:"mutation_result_id" json:"mutation_result_id"`
	Plugin           string                 `bson:"plugin" json:"plugin"`
	State            EvaluationState        `bson:"state" json:"state"`
	Values           map[string]interface{} `bson:"values" json:"values"`
	Error            string                 `bson:"error" json:"error"`
	CreatedAt        time.Time              `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time              `bson:"updated_at" json:"updated_at"`
}

func (run *EvaluationRun) Validate() error {
	if run == nil || run.JobId.IsZero() || run.MutationId.IsZero() {
		return fmt.Errorf("evaluation run requires job and mutation IDs")
	}
	if len(run.Plugins) == 0 {
		return fmt.Errorf("evaluation run requires at least one plugin")
	}
	plugins := make(map[string]bool, len(run.Plugins))
	for _, plugin := range run.Plugins {
		if strings.TrimSpace(plugin) == "" || plugins[plugin] {
			return fmt.Errorf("evaluation plugins must be nonempty and unique")
		}
		plugins[plugin] = true
	}
	if run.CurrentPluginIndex < 0 || run.CurrentPluginIndex >= len(run.Plugins) {
		return fmt.Errorf("evaluation current plugin index is out of range")
	}
	if len(run.MutationResultIds) == 0 {
		return fmt.Errorf("evaluation run requires candidates")
	}
	seen := make(map[primitive.ObjectID]bool, len(run.MutationResultIds))
	for _, id := range run.MutationResultIds {
		if id.IsZero() || seen[id] {
			return fmt.Errorf("evaluation candidates must be nonzero and unique")
		}
		seen[id] = true
	}
	return nil
}

// PluginIndex returns the position of a plugin in the run, or -1.
func (run *EvaluationRun) PluginIndex(plugin string) int {
	for i, p := range run.Plugins {
		if p == plugin {
			return i
		}
	}
	return -1
}

// CurrentPlugin is the plugin that is running now.
func (run *EvaluationRun) CurrentPlugin() string {
	if run.CurrentPluginIndex < 0 || run.CurrentPluginIndex >= len(run.Plugins) {
		return ""
	}
	return run.Plugins[run.CurrentPluginIndex]
}

// IsLastPlugin reports whether the current plugin is the run's last one.
func (run *EvaluationRun) IsLastPlugin() bool {
	return run.CurrentPluginIndex >= len(run.Plugins)-1
}
