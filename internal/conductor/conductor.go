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
		"config":   job.Options[job.Meta[job.StageId]],
		"artifact": job.Artifacts,
		"meta":     job.Meta,
	})
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	steps := []string{"SEQUENCER", "EVOTUNE", "FIT_TOP", "MUTATION"}

	req, err := http.NewRequest("POST", os.Getenv(steps[job.StageId]+"_URL"), bytes.NewBuffer(reqBody))
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

	if data.StageID != job.StageId {
		fmt.Println("Error: stage mismatch")
		return nil
	}

	if data.Status == "FAILED" {
		job.State = "FAILED"
		fmt.Println("Job failed:", data.Error)
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			fmt.Println("Failed to update job:", err)
			return nil
		}
		return nil
	}

	state, stage_id := getNextStage(*job, data)
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
	if job.StageId < 3 {
		job.Artifacts[job.Meta[job.StageId]] = data.Artifact
	}
	job.StageId = stage_id
	if err := con.jobRepository.Update(data.JobID, job); err != nil {
		fmt.Println("Failed to update job:", err)
		return nil
	}

	return job
}

func getNextStage(job models.Job, data Data) (state string, stage_id int) {
	// stages := job.Stages
	// for i, stage := range stages {
	// 	if stage == data.Stage {
	// 		if i == len(stages)-1 {
	// 			return "COMPLETED", stage
	// 		}
	// 		if stage == "EVOTUNE" && job.LabResult == nil {
	// 			return "PENDING", stages[i+1]
	// 		}
	// 		return "ONGOING", stages[i+1]
	// 	}
	// }
	// return "FAILED", data.Stage
	switch data.StageID {
	case 0:
		return "ONGOING", 1
	case 1:
		if job.LabResult == nil {
			return "PENDING", 2
		}
		return "ONGOING", 2
	case 2:
		return "ONGOING", 3
	case 3:
		return "COMPLETED", 3
	default:
		return "FAILED", data.StageID
	}
}
