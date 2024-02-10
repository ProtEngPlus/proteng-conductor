package database

import (
	"context"

	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/internal/logger"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client

func ConnectToDB() error {
	uri := config.Config.MongoUri
	logger.Infof("Connecting to MongoDB")
	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return err
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		return err
	}

	Client = client
	logger.Zap.Info("Connected to MongoDB")
	return nil
}

func GetCollection(collectionName string) *mongo.Collection {
	return Client.Database(config.Config.MongoDb).Collection(collectionName)
}
