package repositories

import (
	"context"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"
	"github.com/protengplus/proteng-conductor/models/enum"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type MutationRepository interface {
	Create(mutation *models.Mutation) error
	FindById(id string) (*models.Mutation, error)
	Update(id string, mutation *models.Mutation) error
	Delete(id string) error
	GetAll(query map[string]interface{}) ([]*models.Mutation, error)
}

type mutationRepository struct {
	collection *mongo.Collection
}

func NewMutationRepository() MutationRepository {
	return &mutationRepository{collection: database.GetCollection("mutations")}
}

func (mr *mutationRepository) GetAll(query map[string]interface{}) ([]*models.Mutation, error) {
	var mutations []*models.Mutation
	filter := bson.M{}

	if len(query) > 0 {
		if jobID, ok := query["job_id"]; ok {
			filter["job_id"] = jobID
		}
	}

	cursor, err := mr.collection.Find(context.Background(), filter)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var mutation models.Mutation
		if err := cursor.Decode(&mutation); err != nil {
			return nil, err
		}
		mutations = append(mutations, &mutation)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return mutations, nil
}

func (mr *mutationRepository) FindById(id string) (*models.Mutation, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": objectId}

	var mutation models.Mutation
	err = mr.collection.FindOne(context.Background(), filter).Decode(&mutation)
	if err != nil {
		return nil, err
	}

	return &mutation, nil
}

func (mr *mutationRepository) Create(mutation *models.Mutation) error {
	mutation.Id = primitive.NewObjectID()
	mutation.State = enum.MutationStatePending
	mutation.CreatedAt = time.Now()

	_, err := mr.collection.InsertOne(context.Background(), mutation)
	if err != nil {
		return err
	}

	return nil
}

func (mr *mutationRepository) Update(id string, mutation *models.Mutation) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	filter := bson.M{"_id": objectId}

	update := bson.M{
		"$set": bson.M{
			"state":         string(mutation.State),
			"options":       mutation.Options,
			"input_protein": mutation.InputProtein,
			"result":        mutation.Result,
		},
	}

	_, err = mr.collection.UpdateOne(context.Background(), filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (mr *mutationRepository) Delete(id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objectId}

	_, err = mr.collection.DeleteOne(context.Background(), filter)
	if err != nil {
		return err
	}

	return nil
}
