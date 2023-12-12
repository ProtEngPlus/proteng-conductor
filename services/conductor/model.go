package conductor

import (
	"proteng-conductor/models"
)

type Data struct {
	JobID    string `json:"job_id"`
	StageID  int    `json:"stage_id"`
	Status   string `json:"status"`
	Artifact string `json:"artifact"`
	Error    string `json:"error"`
}

type Payload struct {
	ServiceName string `json:"service_name"`
	Timestamp   string `json:"timestamp"`
	Data        Data   `json:"data"`
}

type PipelineRequest struct {
	JobId     string           `json:"job_id"`
	Input     string           `json:"input"`
	Config    interface{}      `json:"config"`
	Artifact  interface{}      `json:"artifact"`
	Meta      []string         `json:"meta"`
	LabResult models.LabResult `json:"lab_result"`
}

type PipelineResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
