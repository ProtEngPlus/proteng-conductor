package repositories

import (
	"context"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

//go:generate mockgen -source=configuration_repository.go -destination=mock_repository/mock_configuration_repository.go -package=mock_repository

type ConfigurationRepository interface {
	Create(configuration *models.Configuration) error
	GetAll(query map[string]interface{}) ([]*models.Configuration, error)
	DeleteByJobId(jobId string) error
}

type configurationRepository struct {
	collection *mongo.Collection
}

func NewConfigurationRepository() ConfigurationRepository {
	return &configurationRepository{collection: database.GetCollection("configurations")}
}

func (cr *configurationRepository) GetAll(query map[string]interface{}) ([]*models.Configuration, error) {
	var configurations []*models.Configuration
	filter := bson.M{}

	if len(query) > 0 {
		if userID, ok := query["user_id"]; ok {
			filter["user_id"] = userID
		}
		if states, ok := query["state"]; ok {
			filter["state"] = bson.M{"$in": states}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	cursor, err := cr.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}

	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var configuration models.Configuration
		if err := cursor.Decode(&configuration); err != nil {
			return nil, err
		}
		configurations = append(configurations, &configuration)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return configurations, nil
}

func (cr *configurationRepository) Create(configuration *models.Configuration) error {
	configuration.Id = primitive.NewObjectID()

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err := cr.collection.InsertOne(ctx, configuration)
	if err != nil {
		return err
	}

	return nil
}

func (cr *configurationRepository) DeleteByJobId(jobId string) error {
	objectId, err := primitive.ObjectIDFromHex(jobId)
	if err != nil {
		return err
	}

	filter := bson.M{"ref_job_id": objectId}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = cr.collection.DeleteMany(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}
