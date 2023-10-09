package config

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

// LoadConfig loads the config from the .env file if the ENV variable is set
func AutomaticLoadEnv() {
	if env, ok := os.LookupEnv("ENV"); ok {
		if err := LoadEnvFromPath(".env." + env); err != nil {
			logrus.Fatalf("Error loading .env file from  file: %v", err)
		} else {
			logrus.Infof("Running in environment: %s", env)
		}
	}
}

func LoadEnvFromPath(path string) error {
	return godotenv.Load(path)
}
