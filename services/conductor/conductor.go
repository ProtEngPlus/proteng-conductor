package conductor

import (
	"encoding/json"
	"fmt"

	"proteng-conductor/models"
	"proteng-conductor/repositories"
)

type Conductor struct {
	jobRepository      repositories.JobRepository
	mutationRepository repositories.MutationRepository
}

func NewConductor(jobRepository repositories.JobRepository, mutationRepository repositories.MutationRepository) *Conductor {
	return &Conductor{jobRepository: jobRepository, mutationRepository: mutationRepository}
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

	// Orchestrate next task
	err := con.OrchestrateJob(job)
	if err != nil {
		fmt.Println("Error:", err)
	}
}

// RunJob runs/retries a job
func (con *Conductor) RunJob(job *models.Job) error {
	if job.State == "ONGOING" {
		return fmt.Errorf("error: job is already ongoing")
	}
	if job.State == "COMPLETED" {
		return fmt.Errorf("error: job is already completed")
	}
	if job.StageId == 2 && job.LabResult.Total == 0 {
		return fmt.Errorf("error: lab result is missing")
	}
	job.State = "ONGOING"
	if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
		fmt.Println("Failed to update job:", err)
		return err
	}
	return con.OrchestrateJob(job)
}

// RunMutation runs/retries a mutation
func (con *Conductor) RunMutation(mutation *models.Mutation) error {
	if mutation.State == "ONGOING" {
		return fmt.Errorf("error: mutation is currently running")
	}
	if mutation.State == "COMPLETED" {
		return fmt.Errorf("error: mutation is already completed")
	}
	job, err := con.jobRepository.FindById(mutation.JobId.Hex())
	if err != nil {
		fmt.Println("Failed to find job:", err)
		return err
	}
	if job.StageId != 3 || job.State != "COMPLETED" {
		return fmt.Errorf("error: job is currently running")
	}
	mutation.State = "ONGOING"
	if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
		fmt.Println("Failed to update mutation:", err)
		return err
	}
	reqBodyMap := PipelineRequest{
		JobId:      mutation.JobId.Hex(),
		MutationId: mutation.Id.Hex(),
		Input:      mutation.InputProtein,
		Config:     mutation.Options,
		Artifact:   job.Artifacts,
		Meta:       job.Meta,
	}

	err = con.StartPipelineComponent(job.StageId, reqBodyMap)

	if err != nil {
		mutation.State = "FAILED"
		if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
			return err
		}
		return err
	}
	return nil
}

// OrchestrateJob sends a request to the next task
func (con *Conductor) OrchestrateJob(job *models.Job) error {
	// Get next task
	reqBodyMap := PipelineRequest{
		JobId:    job.Id.Hex(),
		Input:    job.InputProtein,
		Config:   job.Options[job.Meta[job.StageId]],
		Artifact: job.Artifacts,
		Meta:     job.Meta,
	}
	if job.StageId == 2 {
		reqBodyMap.LabResult = job.LabResult
	}
	if job.StageId == 3 {
		mutation, err := con.getFirstMutation(job)
		if err != nil {
			return err
		}
		reqBodyMap.MutationId = mutation.Id.Hex()
		reqBodyMap.Config = mutation.Options
	}

	err := con.StartPipelineComponent(job.StageId, reqBodyMap)

	if err != nil {
		job.State = "FAILED"
		if err := con.jobRepository.Update(job.Id.Hex(), job); err != nil {
			return err
		}
		return err
	}
	return nil
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

	if data.StageID == 3 {
		job.State = "COMPLETED"
		if err := con.jobRepository.Update(data.JobID, job); err != nil {
			fmt.Println("Failed to update job:", err)
			return nil
		}
		con.updateMutationData(data)
		return job
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
		job.Artifacts = make(map[string]models.Artifact)
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

func (con *Conductor) getFirstMutation(job *models.Job) (*models.Mutation, error) {
	query := map[string]interface{}{
		"job_id": job.Id.Hex(),
	}
	mutations, err := con.mutationRepository.GetAll(query)
	if err != nil {
		return nil, err
	}
	mutation := &models.Mutation{}
	if len(mutations) == 0 {
		mutation = &models.Mutation{
			JobId:        job.Id,
			InputProtein: job.InputProtein,
			Options:      job.Options[job.Meta[3]].(map[string]interface{}),
		}
		if err = con.mutationRepository.Create(mutation); err != nil {
			return nil, err
		}
	} else {
		mutation = mutations[0]
	}
	mutation.State = "ONGOING"
	if err := con.mutationRepository.Update(mutation.Id.Hex(), mutation); err != nil {
		return nil, err
	}
	return mutation, nil
}

func (con *Conductor) updateMutationData(data Data) {
	mutation, err := con.mutationRepository.FindById(data.MutationID)
	if err != nil {
		fmt.Println("Failed to find mutation:", err)
		return
	}

	if data.Status == "FAILED" {
		mutation.State = "FAILED"
		fmt.Println("Mutation failed:", data.Error)
		if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
			fmt.Println("Failed to update mutation:", err)
			return
		}
		return
	}

	mutation.State = "COMPLETED"
	mutation.Result = data.MutationResult
	if err := con.mutationRepository.Update(data.MutationID, mutation); err != nil {
		fmt.Println("Failed to update mutation:", err)
		return
	}
}

func getNextStage(job models.Job, data Data) (state string, stage_id int) {
	switch data.StageID {
	case 0:
		return "ONGOING", 1
	case 1:
		if job.LabResult.Total == 0 {
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
