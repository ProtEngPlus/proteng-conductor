package models

import (
	"time"

	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Mutation struct {
	Id           primitive.ObjectID     `bson:"_id" json:"id"`
	Name         string                 `bson:"name" json:"name"`
	JobId        primitive.ObjectID     `bson:"job_id" json:"job_id"`
	RunId        int                    `bson:"run_id" json:"run_id"`
	InputProtein string                 `bson:"input_protein" json:"input_protein"`
	Options      map[string]interface{} `bson:"options" json:"options"`
	State        enum.MutationState     `bson:"state" json:"state"`
	Result       map[string]float32     `bson:"result" json:"result"`
	IsBookmark   bool 					`bson:"is_bookmark" json:"is_bookmark"`
	CreatedAt    time.Time              `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time              `bson:"complete_at" json:"complete_at"`
}
