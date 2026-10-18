package conductor

import (
	"github.com/protengplus/proteng-conductor/models"
)

type Data struct {
	JobID             string                 `json:"job_id"`
	MutationID        string                 `json:"mutation_id,omitempty"`
	QueryResultId     string                 `json:"query_result_id,omitempty"`
	StageID           int                    `json:"stage_id"`
	Status            string                 `json:"status"`
	Artifact          models.Artifact        `json:"artifact"`
	MutationResult    map[string]float32     `json:"mutation_result,omitempty"`
	QueryResult       []models.ResultFields  `json:"query_result,omitempty"`
	Error             string                 `json:"error"`
	EvaluationRunID   string                 `json:"evaluation_run_id,omitempty"`
	Plugin            string                 `json:"plugin,omitempty"`
	EvaluationResults []EvaluationResultData `json:"evaluation_results,omitempty"`
}

type EvaluationResultData struct {
	MutationResultID string                 `json:"mutation_result_id"`
	Values           map[string]interface{} `json:"values"`
}

type Payload struct {
	ServiceName string `json:"service_name"`
	Timestamp   string `json:"timestamp"`
	Data        Data   `json:"data"`
}

type PipelineRequest struct {
	JobId         string                `json:"job_id"`
	Input         string                `json:"input"`
	Config        interface{}           `json:"config"`
	Artifact      interface{}           `json:"artifact"`
	Meta          []string              `json:"meta"`
	LabResult     models.LabResult      `json:"lab_result,omitempty"`
	MutationId    string                `json:"mutation_id,omitempty"`
	QueryResultId string                `json:"query_result_id,omitempty"`
	QueryResult   []models.ResultFields `json:"query_result,omitempty"`
}

type PipelineResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
