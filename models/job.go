package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type LabResult struct {
	Total     int       `bson:"total" json:"total"`
	Sequences []string  `bson:"sequences" json:"sequences"`
	Scores    []float32 `bson:"scores" json:"scores"`
}

type Artifact struct {
	BucketName string `bson:"bucket_name" json:"bucket_name"`
	Path       string `bson:"path" json:"path"`
	Url        string `bson:"url" json:"url"`
}

type ErrLog struct {
	Content   string    `bson:"content" json:"content"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
}

type Job struct {
	Id           primitive.ObjectID     `bson:"_id" json:"id"`
	Name         string                 `bson:"name" json:"name"`
	State        string                 `bson:"state" json:"state"`
	StageId      int                    `bson:"stage_id" json:"stage_id"`
	UserId       string                 `bson:"user_id" json:"user_id"`
	LabResult    LabResult              `bson:"lab_result" json:"lab_result"`
	Options      map[string]interface{} `bson:"options" json:"options"`
	Artifacts    map[string]Artifact    `bson:"artifact" json:"artifact"`
	Meta         []string               `bson:"meta" json:"meta"`
	InputProtein string                 `bson:"input_protein" json:"input_protein"`
	RefJobId     primitive.ObjectID     `bson:"ref_job_id" json:"ref_job_id"`
	CreatedAt    time.Time              `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time              `bson:"complete_at" json:"complete_at"`
	ErrorLogs    []ErrLog               `bson:"error_logs" json:"error_logs"`
}
