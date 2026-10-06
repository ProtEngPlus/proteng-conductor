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
// Phase 3.2 registers one plugin; sequencing multiple plugins is a later phase.
type EvaluationRun struct {
	Id                 primitive.ObjectID   `bson:"_id" json:"id"`
	JobId              primitive.ObjectID   `bson:"job_id" json:"job_id"`
	MutationId         primitive.ObjectID   `bson:"mutation_id" json:"mutation_id"`
	MutationResultIds  []primitive.ObjectID `bson:"mutation_result_ids" json:"mutation_result_ids"`
	Plugins            []string             `bson:"plugins" json:"plugins"`
	CurrentPluginIndex int                  `bson:"current_plugin_index" json:"current_plugin_index"`
	State              EvaluationState      `bson:"state" json:"state"`
	Error              string               `bson:"error" json:"error"`
	CreatedAt          time.Time            `bson:"created_at" json:"created_at"`
	CompleteAt         time.Time            `bson:"complete_at" json:"complete_at"`
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
	if len(run.Plugins) != 1 || strings.TrimSpace(run.Plugins[0]) == "" || run.CurrentPluginIndex != 0 {
		return fmt.Errorf("phase 3.2 requires exactly one plugin at index zero")
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
