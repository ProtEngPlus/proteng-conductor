package main

import (
	"fmt"
	"time"

	"github.com/protengplus/proteng-conductor/apis/routes"
	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/internal/rabbitmq"
	"github.com/protengplus/proteng-conductor/internal/validator"
	"github.com/protengplus/proteng-conductor/repositories"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitZap()

	config.AutomaticLoadEnv()

	validator.Init()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// database
	err := database.ConnectToDB()
	if err != nil {
		logger.Fatalf("Failed to connect to database: %v", err)
	}
	jobRepository := repositories.NewJobRepository()
	mutationRepository := repositories.NewMutationRepository()

	// conductor
	conductor := conductor.NewConductor(jobRepository, mutationRepository)
	rabbitConsumer := rabbitmq.NewConsumer(conductor)

	rabbitMqUser := config.Config.RabbitMqUser
	rabbitMqPassword := config.Config.RabbitMqPassword
	rabbitMqHost := config.Config.RabbitMqHost
	rabbitMqPort := config.Config.RabbitMqPort
	amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%s/", rabbitMqUser, rabbitMqPassword, rabbitMqHost, rabbitMqPort)
	go func() {
		err := rabbitConsumer.RunConsumer(amqpURL, config.Config.JobQueue)
		if err != nil {
			logger.Fatalf("Error in RabbitMQ Consumer: %v", err)
		}
	}()

	// logging middleware
	router.Use(ginzap.GinzapWithConfig(logger.Zap, &ginzap.Config{
		TimeFormat: time.RFC3339,
		UTC:        true,
		SkipPaths:  []string{"/metrics", "/health"},
	}))

	// health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	// routes
	routes.JobRoute(router, jobRepository, conductor)
	routes.MutationRoute(router, jobRepository, mutationRepository, conductor)

	// panic recovery
	router.Use(ginzap.RecoveryWithZap(logger.Zap, true))

	// start server
	httpPort := config.Config.HttpPort
	err = router.Run(":" + httpPort)
	if err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}
}

// TODO: Graceful shutdown
