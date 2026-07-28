package models

import (
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/internal/validator"
	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type LabResult struct {
	Total     int       `bson:"total" json:"total" validate:"gte=0"`
	Names     []string  `bson:"names" json:"names" validate:"required"`
	Sequences []string  `bson:"sequences" json:"sequences" validate:"required"`
	Scores    []float32 `bson:"scores" json:"scores" validate:"required"`
}

func (lr LabResult) Validate() error {
	if lr.Total != len(lr.Sequences) || lr.Total != len(lr.Scores) || lr.Total != len(lr.Names) {
		return fmt.Errorf("data length mismatch")
	}
	return nil
}

type UniProtSequence struct {
	Sequence string `json:"sequence" validate:"required"`
}

type Artifact struct {
	BucketName string `bson:"bucket_name" json:"bucket_name"`
	Path       string `bson:"path" json:"path"`
	Url        string `bson:"url,omitempty" json:"url,omitempty"`
	Content    []byte `json:"content,omitempty"`
}

type StageRunTime struct {
	StartTime 	time.Time 		  `bson:"start_time" json:"start_time"`
	EndTime   	time.Time 		  `bson:"end_time" json:"end_time"`
}

type ErrLog struct {
	Content   string    `bson:"content" json:"content"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
}

type Job struct {
	Id               primitive.ObjectID     `bson:"_id" json:"id"`
	Name             string                 `bson:"name" json:"name" validate:"required"`
	State            enum.JobState          `bson:"state" json:"state"`
	StageId          int                    `bson:"stage_id" json:"stage_id" validate:"gte=0,lte=3"`
	UserId           string                 `bson:"user_id" json:"user_id" validate:"required"`
	RefJobId         primitive.ObjectID     `bson:"ref_job_id" json:"ref_job_id"`
	LabResult        LabResult              `bson:"lab_result" json:"lab_result" validate:"omitempty"`
	Options          map[string]interface{} `bson:"options" json:"options" validate:"required"`
	Artifacts        map[string]Artifact    `bson:"artifact" json:"artifact"`
	Meta             []string               `bson:"meta" json:"meta"`
	RunTime			 map[string]StageRunTime`bson:"run_time" json:"run_time"`
	InputProtein     string                 `bson:"input_protein" json:"input_protein" validate:"required"`
	RunType          string                 `bson:"run_type" json:"run_type" validate:"required"`
	Description      string                 `bson:"description" json:"description"`
	IsNotificationOn bool                   `bson:"is_notification_on" json:"is_notification_on"`
	CreatedAt        time.Time              `bson:"created_at" json:"created_at"`
	CompleteAt       time.Time              `bson:"complete_at" json:"complete_at"`
	UpdatedAt 		 time.Time 				`bson:"updated_at" json:"updated_at"`
	ErrorLogs        []ErrLog               `bson:"error_logs" json:"error_logs"`
}

type Configuration struct {
	Id               primitive.ObjectID     `bson:"_id" json:"id"`
	RefJobId         primitive.ObjectID     `bson:"ref_job_id" json:"ref_job_id" validate:"required"`
	State            enum.JobState          `bson:"state" json:"state"`
	StageId          int                    `bson:"stage_id" json:"stage_id" validate:"gte=0,lte=3"`
	Name             string                 `bson:"name" json:"name" validate:"required"`
	UserId           string                 `bson:"user_id" json:"user_id" validate:"required"`
	LabResult        LabResult              `bson:"lab_result" json:"lab_result"`
	Options          map[string]interface{} `bson:"options" json:"options" validate:"required"`
	Artifacts        map[string]Artifact    `bson:"artifact" json:"artifact"`
	Meta             []string               `bson:"meta" json:"meta"`
	InputProtein     string                 `bson:"input_protein" json:"input_protein" validate:"required"`
	RunType          string                 `bson:"run_type" json:"run_type" validate:"required"`
	Description      string                 `bson:"description" json:"description"`
	IsNotificationOn bool                   `bson:"is_notification_on" json:"is_notification_on"`
}
type NumberOfJobs struct {
	Created   int `json:"created"`
	Pending   int `json:"pending"`
	Ongoing   int `json:"ongoing"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type RecentJob struct {
	Id          primitive.ObjectID `bson:"_id" json:"id"`
	Name        string             `bson:"name" json:"name"`
	Description string             `bson:"description" json:"description"`
}

type DashboardResponseData struct {
	NumberOfJobs   NumberOfJobs    `json:"number_of_jobs"`
	BestAssayScore *BestAssayScore `json:"best_assay_score"`
	RecentJob      *RecentJob      `json:"recent_job"`
}

func (job Job) Validate(labresult bool) error {
	if labresult {
		err := validator.Validate.Struct(job.LabResult)
		if err != nil {
			return err
		}
		err = job.LabResult.Validate()
		if err != nil {
			return err
		}
	}

	return validator.Validate.Struct(job)
}
