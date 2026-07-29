package models

import (
	"time"

	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type QueryResult struct {
	Id           primitive.ObjectID    `bson:"_id" json:"id"`
	JobId        primitive.ObjectID    `bson:"job_id" json:"job_id"`
	RunId        int                   `bson:"run_id" json:"run_id"`
	InputProtein string                `bson:"input_protein" json:"input_protein"`
	State        enum.QueryResultState `bson:"state" json:"state"`
	Result       []ResultFields        `bson:"result" json:"result"`
	CreatedAt    time.Time             `bson:"created_at" json:"created_at"`
	CompleteAt   time.Time             `bson:"complete_at" json:"complete_at"`
}

type ResultFields struct {
	Id              primitive.ObjectID `bson:"id" json:"id"`
	IsSelected      bool               `bson:"is_selected" json:"is_selected"`
	Sequences       string             `bson:"sequences" json:"sequences"`
	Score           float64            `bson:"score" json:"score"`
	MaxScore        float64            `bson:"max_score" json:"max_score"`
	HspQueryFrom    float64            `bson:"hsp_query_from" json:"hsp_query_from"`
	HspQueryTo      float64            `bson:"hsp_query_to" json:"hsp_query_to"`
	QueryCover      float64            `bson:"query_cover" json:"query_cover"`
	EValues         float64            `bson:"e_values" json:"e_values"`
	Accession       string             `bson:"accession" json:"accession"`
	PercentIdentity float64            `bson:"percent_identity" json:"percent_identity"`
	AccLen          int                `bson:"acc_len" json:"acc_len"`
	Description     string             `bson:"description" json:"description"`
	Organisms       string             `bson:"organisms" json:"organisms"`
}
