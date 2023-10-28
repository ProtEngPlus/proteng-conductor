package database

import (
	"context"
	"log"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client

func ConnectToDB() error {
	uri := os.Getenv("MONGO_URI")
	log.Println("Connecting to MongoDB:", uri)
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
	return nil
}

func GetCollection(collectionName string) *mongo.Collection {
	return Client.Database(os.Getenv("MONGO_DB")).Collection(collectionName)
}
