package config

import (
	"os"

	"github.com/protengplus/proteng-conductor/internal/logger"

	"github.com/spf13/viper"
)

var Config config

type config struct {
	Env      string `mapstructure:"ENV"`
	HttpPort string `mapstructure:"HTTP_PORT"`

	RabbitMqUser     string `mapstructure:"RABBITMQ_USER"`
	RabbitMqPassword string `mapstructure:"RABBITMQ_PASSWORD"`
	RabbitMqHost     string `mapstructure:"RABBITMQ_HOST"`
	RabbitMqPort     string `mapstructure:"RABBITMQ_PORT"`
	JobQueue         string `mapstructure:"JOB_QUEUE"`

	MongoUri string `mapstructure:"MONGO_URI"`
	MongoDb  string `mapstructure:"MONGO_DB"`

	SequencerUrl string `mapstructure:"SEQUENCER_URL"`
	EvotuneUrl   string `mapstructure:"EVOTUNE_URL"`
	FittopUrl    string `mapstructure:"FITTOP_URL"`
	MutationUrl  string `mapstructure:"MUTATION_URL"`
}

func AutomaticLoadEnv() {
	if env, ok := os.LookupEnv("ENV"); ok {
		if err := LoadEnvFromPath(".env." + env); err != nil {
			logger.Fatalf("Error loading .env file from file: %v", err)
		} else {
			logger.Infof("Running in environment: %s", env)
		}
	}

	viper.AutomaticEnv()
	err := viper.Unmarshal(&Config)
	if err != nil {
		logger.Fatalf("Unable to decode env into config struct, %v", err)
	}
}

func LoadEnvFromPath(path string) error {
	viper.SetConfigType("env")
	viper.SetConfigFile(path)
	return viper.ReadInConfig()
}
