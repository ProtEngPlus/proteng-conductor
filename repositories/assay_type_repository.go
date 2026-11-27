package repositories

import (
	"context"
	"time"

	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

//go:generate mockgen -source=assay_type_repository.go -destination=mock_repository/mock_assay_type_repository.go -package=mock_repository

type AssayTypeRepository interface {
	Create(assayType *models.AssayType) error
	FindById(id string) (*models.AssayType, error)
	GetAllByUser(userId string) ([]*models.AssayType, error)
	Update(id string, assayType *models.AssayType) error
	Delete(id string) error
}

type assayTypeRepository struct {
	collection *mongo.Collection
}

func NewAssayTypeRepository() AssayTypeRepository {
	return &assayTypeRepository{collection: database.GetCollection("assay_types")}
}

func (ar *assayTypeRepository) Create(assayType *models.AssayType) error {
	assayType.Id = primitive.NewObjectID()
	assayType.CreatedAt = time.Now()
	assayType.UpdatedAt = assayType.CreatedAt

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err := ar.collection.InsertOne(ctx, assayType)
	return err
}

func (ar *assayTypeRepository) FindById(id string) (*models.AssayType, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	var assayType models.AssayType
	if err := ar.collection.FindOne(ctx, bson.M{"_id": objectId}).Decode(&assayType); err != nil {
		return nil, err
	}

	return &assayType, nil
}

func (ar *assayTypeRepository) GetAllByUser(userId string) ([]*models.AssayType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	cursor, err := ar.collection.Find(ctx, bson.M{"user_id": userId})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	assayTypes := []*models.AssayType{}
	if err := cursor.All(ctx, &assayTypes); err != nil {
		return nil, err
	}

	return assayTypes, nil
}

func (ar *assayTypeRepository) Update(id string, assayType *models.AssayType) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	assayType.UpdatedAt = time.Now()
	update := bson.M{"$set": bson.M{
		"name":        assayType.Name,
		"description": assayType.Description,
		"updated_at":  assayType.UpdatedAt,
	}}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = ar.collection.UpdateOne(ctx, bson.M{"_id": objectId}, update)
	return err
}

func (ar *assayTypeRepository) Delete(id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), database.QueryTimeout)
	defer cancel()

	_, err = ar.collection.DeleteOne(ctx, bson.M{"_id": objectId})
	return err
}
