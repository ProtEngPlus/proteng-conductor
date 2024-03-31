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

	RabbitMqUser     string `envconfig:"RABBITMQ_USER"`
	RabbitMqPassword string `envconfig:"RABBITMQ_PASSWORD"`
	RabbitMqHost     string `envconfig:"RABBITMQ_HOST"`
	RabbitMqPort     string `envconfig:"RABBITMQ_PORT"`
	JobQueue         string `envconfig:"JOB_QUEUE"`

	MongoUri string `envconfig:"MONGO_URI"`
	MongoDb  string `envconfig:"MONGO_DB"`

	SequencerUrl string `envconfig:"SEQUENCER_URL"`
	EvotuneUrl   string `envconfig:"EVOTUNE_URL"`
	FittopUrl    string `envconfig:"FIT_TOP_URL"`
	MutationUrl  string `envconfig:"MUTATION_URL"`

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
			logger.Fatalf("Error loading .env file from file: %v", err)
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
