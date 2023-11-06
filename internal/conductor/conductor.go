package conductor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"proteng-conductor/models"
	"proteng-conductor/repositories"
)

type Conductor struct {
	jobRepository repositories.JobRepository
}

func NewConductor(jobRepository repositories.JobRepository) *Conductor {
	return &Conductor{jobRepository: jobRepository}
}

type Data struct {
	JobID    string `json:"job_id"`
	Stage    string `json:"stage_name"`
	Status   string `json:"status"`
	Artifact string `json:"artifact"`
	Error    string `json:"error"`
}

type Payload struct {
	ServiceName string `json:"service_name"`
	Timestamp   string `json:"timestamp"`
	Data        Data   `json:"data"`
}

func (con *Conductor) Orchestrate(m string) {
	// Decode the incoming message
	var payload Payload
	if err := json.Unmarshal([]byte(m), &payload); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Update job data
	job := con.updateJobData(payload.Data)
	if job == nil || job.State != "ONGOING" {
		return
	}

	// Get next task
	reqBody, err := json.Marshal(map[string]interface{}{
		"job_id":   job.Id.Hex(),
		"input":    job.InputProtein,
		"config":   job.Options[job.CurrentStage],
		"artifact": job.Artifacts,
	})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	req, err := http.NewRequest("POST", os.Getenv(job.CurrentStage+"_URL"), bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()
}

func (con *Conductor) updateJobData(data Data) *models.Job {
	job, err := con.jobRepository.FindById(data.JobID)
	if err != nil {
		fmt.Println("Failed to find job:", err)
		return nil
	}

	if data.Stage != job.CurrentStage {
		fmt.Println("Error: stage mismatch")
		return nil
	}

	if data.Status == "FAIL" {
		job.State = "FAILED"
		fmt.Println("Job failed:", data.Error)
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			fmt.Println("Failed to update job:", err)
			return nil
		}
		return nil
	}

	state, stage := getNextStage(*job, data)
	job.State = state

	if state == "FAILED" {
		fmt.Println("Error: Invalid stage")
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			fmt.Println("Failed to update job:", err)
			return nil
		}
		return nil
	}

	if job.Artifacts == nil {
		job.Artifacts = make(map[string]interface{})
	}
	job.Artifacts[job.CurrentStage] = data.Artifact
	job.CurrentStage = stage
	if err := con.jobRepository.Update(data.JobID, job); err != nil {
		fmt.Println("Failed to update job:", err)
		return nil
	}

	return job
}

func getNextStage(job models.Job, data Data) (state string, stage string) {
	stages := job.Stages
	for i, stage := range stages {
		if stage == data.Stage {
			if i == len(stages)-1 {
				return "COMPLETED", stage
			}
			if stage == "EVOTUNE" && job.LabResult == nil {
				return "PENDING", stages[i+1]
			}
			return "ONGOING", stages[i+1]
		}
	}
	return "FAILED", data.Stage
}
