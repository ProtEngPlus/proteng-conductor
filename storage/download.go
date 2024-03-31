package storage

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

type StorageService interface {
	GetObjectContent(bucketName, objectName string) ([]byte, error)
}

type storageService struct {
	client *storage.Client
}

func NewStorageService(credentialsFilePath string) StorageService {
	ctx := context.Background()

	client, err := storage.NewClient(ctx, option.WithCredentialsFile(credentialsFilePath))
	if err != nil {
		panic(fmt.Sprintf("Error creating GCS client: %v", err))
	}

	return &storageService{client: client}
}

func (s *storageService) GetObjectContent(bucketName, objectName string) ([]byte, error) {
	ctx := context.Background()

	rc, err := s.client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("error opening object: %v", err)
	}
	defer rc.Close()

	slurp, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("error reading object: %v", err)
	}
	return slurp, nil
}
