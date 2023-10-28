package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Job struct {
	Id           primitive.ObjectID     `bson:"_id" json:"id"`
	State        string                 `bson:"state" json:"state"`
	CurrentStage string                 `bson:"current_stage" json:"current_stage"`
	UserId       string                 `bson:"user_id" json:"user_id"`
	LabResult    map[string]interface{} `bson:"lab_result" json:"lab_result"`
	Options      map[string]interface{} `bson:"options" json:"options"`
	Artifacts    map[string]interface{} `bson:"artifact" json:"artifact"`
	Stages       []string               `bson:"stages" json:"stages"`
	InputProtein string                 `bson:"input_protein" json:"input_protein"`
	RefJobId     string                 `bson:"ref_job_id" json:"ref_job_id"`
	CreatedAt    time.Time              `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time              `bson:"complete_at" json:"complete_at"`
}
