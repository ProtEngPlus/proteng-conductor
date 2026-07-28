package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/protengplus/proteng-conductor/config"
	"google.golang.org/api/option"
)

type StorageService interface {
	GetObjectContent(bucketName, objectName string) ([]byte, error)
}

type storageService struct {
	client *storage.Client
}

func NewStorageService() StorageService {
	ctx := context.Background()

	// Replace escaped newline characters with actual newline
	privateKey := strings.ReplaceAll(config.Config.PrivateKey, "\\n", "\n")

	// Build credentials JSON
	credentials := map[string]string{
		"type":           "service_account",
		"project_id":     config.Config.ProjectID,
		"private_key_id": config.Config.PrivateKeyID,
		"private_key":    privateKey,
		"client_email":   config.Config.ClientEmail,
		"client_id":      config.Config.ClientID,
		"token_uri":      config.Config.TokenURI,
	}

	credsJSON, err := json.Marshal(credentials)
	if err != nil {
		panic(fmt.Errorf("error marshalling credentials to JSON: %v", err))
	}

	client, err := storage.NewClient(ctx, option.WithCredentialsJSON(credsJSON))
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
