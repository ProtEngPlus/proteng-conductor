package config

import (
	"os"

	"github.com/protengplus/proteng-conductor/internal/logger"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

var Config config

type config struct {
	Env      string `envconfig:"ENV"`
	HttpPort string `envconfig:"HTTP_PORT"`

	RabbitMqUrl string `envconfig:"RABBITMQ_URL"`
	JobQueue    string `envconfig:"JOB_QUEUE"`

	MongoUri string `envconfig:"MONGO_URI"`
	MongoDb  string `envconfig:"MONGO_DB"`

	ProjectID    string `envconfig:"PROJECT_ID"`
	PrivateKeyID string `envconfig:"PRIVATE_KEY_ID"`
	PrivateKey   string `envconfig:"PRIVATE_KEY"`
	ClientEmail  string `envconfig:"CLIENT_EMAIL"`
	ClientID     string `envconfig:"CLIENT_ID"`
	TokenURI     string `envconfig:"TOKEN_URI"`
}

func AutomaticLoadEnv() {
	if env, ok := os.LookupEnv("ENV"); ok {
		if err := LoadEnvFromPath(".env." + env); err != nil {
			logger.Infof("No .env.%s file found, using environment variables as-is: %v", env, err)
		} else {
			logger.Infof("Running in environment: %s", env)
		}
	}

	err := envconfig.Process("", &Config)
	if err != nil {
		logger.Fatalf("Error unmarshalling env vars: %v", err)
	}
}

func LoadEnvFromPath(path string) error {
	return godotenv.Load(path)
}
