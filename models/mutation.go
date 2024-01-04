package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Mutation struct {
	Id           primitive.ObjectID     `bson:"_id" json:"id"`
	JobId        primitive.ObjectID     `bson:"job_id" json:"job_id"`
	InputProtein string                 `bson:"input_protein" json:"input_protein"`
	Options      map[string]interface{} `bson:"options" json:"options"`
	State        string                 `bson:"state" json:"state"`
	Result       map[string]int         `bson:"lab_result" json:"lab_result"`
	CreatedAt    time.Time              `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time              `bson:"complete_at" json:"complete_at"`
}
