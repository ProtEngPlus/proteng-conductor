package repositories

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	// "go.mongodb.org/mongo-driver/mongo/options"
)

//go:generate mockgen -source=query_result_repository.go -destination=mock_repository/mock_query_result_repository.go -package=mock_repository

type QueryResultRepository interface {
	Create(query_result *models.QueryResult) error
	FindById(id string) (*models.QueryResult, error)
	Update(id string, query_result *models.QueryResult) error
	GetAll(query map[string]interface{}) ([]*models.QueryResult, error)
	DeleteByJobId(jobId string) error
}

type queryResultRepository struct {
	collection *mongo.Collection
}

func NewQueryResultRepository() QueryResultRepository {
	return &queryResultRepository{collection: database.GetCollection("query_results")}
}

func (qr *queryResultRepository) GetAll(query map[string]interface{}) ([]*models.QueryResult, error) {
	pipeline := mongo.Pipeline{}

	if jobID, ok := query["job_id"]; ok && jobID != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "job_id", Value: jobID}}}})
	}

	pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: "$result"}})

	if isSelected, ok := query["is_selected"]; ok && isSelected != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "result.is_selected", Value: isSelected}}}})
	}

	if organisms, ok := query["organisms"]; ok && organisms != nil {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.organisms", Value: bson.D{
				{Key: "$regex", Value: organisms},
				{Key: "$options", Value: "i"},
			}},
		}}})
	}

	if percentIdentityFrom, ok := query["percent_identity_from"]; ok && percentIdentityFrom != nil {
		pif, err := strconv.ParseFloat(percentIdentityFrom.(string), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid percent_identity_from value: %v", err)
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.percent_identity", Value: bson.D{{Key: "$gte", Value: pif}}},
		}}})
	}

	if percentIdentityTo, ok := query["percent_identity_to"]; ok && percentIdentityTo != nil {
		pit, err := strconv.ParseFloat(percentIdentityTo.(string), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid percent_identity_to value: %v", err)
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.percent_identity", Value: bson.D{{Key: "$lte", Value: pit}}},
		}}})
	}

	if eValuesFrom, ok := query["e_values_from"]; ok && eValuesFrom != nil {
		evf, err := strconv.ParseFloat(eValuesFrom.(string), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid e_values_from value: %v", err)
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.e_values", Value: bson.D{{Key: "$gte", Value: evf}}},
		}}})
	}

	if eValuesTo, ok := query["e_values_to"]; ok && eValuesTo != nil {
		evt, err := strconv.ParseFloat(eValuesTo.(string), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid e_values_to value: %v", err)
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.e_values", Value: bson.D{{Key: "$lte", Value: evt}}},
		}}})
	}

	if queryCoverFrom, ok := query["query_cover_from"]; ok && queryCoverFrom != nil {
		qcf, err := strconv.ParseFloat(queryCoverFrom.(string), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid query_cover_from value: %v", err)
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.query_cover", Value: bson.D{{Key: "$gte", Value: qcf}}},
		}}})
	}

	if queryCoverTo, ok := query["query_cover_to"]; ok && queryCoverTo != nil {
		qct, err := strconv.ParseFloat(queryCoverTo.(string), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid query_cover_to value: %v", err)
		}
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{
			{Key: "result.query_cover", Value: bson.D{{Key: "$lte", Value: qct}}},
		}}})
	}

	if sortField, ok := query["sort"]; ok && sortField != nil {
		var order int = 1

		if orderVal, ok := query["order"]; ok && orderVal != nil {
			order = orderVal.(int)
		}

		pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{
			{Key: "result." + sortField.(string), Value: order},
		}}})
	}

	pipeline = append(pipeline, bson.D{{Key: "$project", Value: bson.D{
		{Key: "job_id", Value: 1},
		{Key: "input_protein", Value: 1},
		{Key: "complete_at", Value: 1},
		{Key: "created_at", Value: 1},
		{Key: "result.id", Value: 1},
		{Key: "result.is_selected", Value: 1},
		{Key: "result.organisms", Value: 1},
		{Key: "result.query_cover", Value: 1},
		{Key: "result.e_values", Value: 1},
		{Key: "result.percent_identity", Value: 1},
		{Key: "result.acc_len", Value: 1},
		{Key: "result.accession", Value: 1},
		{Key: "result.description", Value: 1},
		{Key: "result.hsp_query_from", Value: 1},
		{Key: "result.hsp_query_to", Value: 1},
		{Key: "result.score", Value: 1},
		{Key: "result.max_score", Value: 1},
		{Key: "result.sequences", Value: 1},
	}}})

	pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.D{
		{Key: "_id", Value: "$_id"},
		{Key: "job_id", Value: bson.D{{Key: "$first", Value: "$job_id"}}},
		{Key: "input_protein", Value: bson.D{{Key: "$first", Value: "$input_protein"}}},
		{Key: "result", Value: bson.D{{Key: "$push", Value: "$result"}}},
		{Key: "created_at", Value: bson.D{{Key: "$first", Value: "$created_at"}}},
		{Key: "complete_at", Value: bson.D{{Key: "$first", Value: "$complete_at"}}},
	}}})

	var results []*models.QueryResult
	cursor, err := qr.collection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var queryResult models.QueryResult
		if err := cursor.Decode(&queryResult); err != nil {
			return nil, err
		}
		results = append(results, &queryResult)
	}

	return results, nil
}

func (qr *queryResultRepository) FindById(id string) (*models.QueryResult, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": objectId}

	var query_result models.QueryResult
	err = qr.collection.FindOne(context.Background(), filter).Decode(&query_result)
	if err != nil {
		return nil, err
	}

	return &query_result, nil
}

func (qr *queryResultRepository) Create(query_result *models.QueryResult) error {
	query_result.Id = primitive.NewObjectID()
	query_result.CreatedAt = time.Now()

	if query_result.State == "" {
		query_result.State = enum.QueryResultStatePending
	}

	var sortFilter = bson.M{
		"job_id": query_result.JobId.Hex(),
		"sort":   "run_id",
		"order":  -1,
	}
	thisJobQueryResults, err := qr.GetAll(sortFilter)
	if err != nil {
		return err
	}

	if len(thisJobQueryResults) == 0 {
		query_result.RunId = 1
	} else {
		query_result.RunId = thisJobQueryResults[0].RunId + 1
	}

	_, err = qr.collection.InsertOne(context.Background(), query_result)
	if err != nil {
		return err
	}

	return nil
}

func (qr *queryResultRepository) Update(id string, query_result *models.QueryResult) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objectId}

	update := bson.M{
		"$set": bson.M{
			"state":         string(query_result.State),
			"input_protein": query_result.InputProtein,
			"result":        query_result.Result,
		},
	}

	_, err = qr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (qr *queryResultRepository) DeleteByJobId(jobId string) error {
	objectId, err := primitive.ObjectIDFromHex(jobId)
	if err != nil {
		return err
	}

	filter := bson.M{"job_id": objectId}

	_, err = qr.collection.DeleteMany(context.Background(), filter)
	if err != nil {
		return err
	}

	return nil
}
