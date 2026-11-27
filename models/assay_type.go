package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AssayType struct {
	Id          primitive.ObjectID `bson:"_id" json:"id"`
	UserId      string             `bson:"user_id" json:"user_id"`
	Name        string             `bson:"name" json:"name"`
	Description string             `bson:"description" json:"description"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

type AssayTypeSummary struct {
	AssayType
	JobCount    int `json:"job_count"`
	ResultCount int `json:"result_count"`
}

type LibraryJob struct {
	JobId       primitive.ObjectID `json:"job_id"`
	JobName     string             `json:"job_name"`
	CompletedAt time.Time          `json:"completed_at"`
	ResultCount int                `json:"result_count"`
}

type LibraryJobList struct {
	Jobs                []LibraryJob `json:"jobs"`
	DuplicatesCollapsed int          `json:"duplicates_collapsed"`
}

type LibraryResult struct {
	Id                primitive.ObjectID `json:"id"`
	ProteinSequence   string             `json:"protein_sequence"`
	MutationPositions []string           `json:"mutation_positions"`
	ActualAssayScore  float32            `json:"actual_assay_score"`
	MeasuredAt        time.Time          `json:"measured_at"`
	Note              string             `json:"note"`
}
