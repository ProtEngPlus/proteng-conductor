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
	IsBookmark   bool                   `bson:"is_bookmark" json:"is_bookmark"`
	UserId       string                 `bson:"user_id" json:"user_id"`
	CreatedAt    time.Time              `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time              `bson:"complete_at" json:"complete_at"`
}

type MutationResult struct {
	Id              primitive.ObjectID `bson:"_id" json:"id"`
	MutationId      primitive.ObjectID `bson:"mutation_id" json:"mutation_id"`
	JobId           primitive.ObjectID `bson:"job_id" json:"job_id"`
	UserId          string             `bson:"user_id" json:"user_id"`
	ProteinSequence string             `bson:"protein_sequence" json:"protein_sequence"`
	AssayScore      float32            `bson:"assay_score" json:"assay_score"`
	IsBookmark      bool               `bson:"is_bookmark" json:"is_bookmark"`
}

type BestAssayScore struct {
	Id         primitive.ObjectID `bson:"id" json:"id"` // job id
	AssayScore float32            `bson:"assay_score" json:"assay_score"`
}
