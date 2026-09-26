package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/protengplus/proteng-conductor/apis/routes"
	"github.com/protengplus/proteng-conductor/config"
	"github.com/protengplus/proteng-conductor/database"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/internal/logger"
	"github.com/protengplus/proteng-conductor/storage"

	rmqConsumer "github.com/protengplus/proteng-conductor/internal/rabbitmq/consumer"
	rmqPublisher "github.com/protengplus/proteng-conductor/internal/rabbitmq/publisher"
	"github.com/protengplus/proteng-conductor/internal/validator"
	"github.com/protengplus/proteng-conductor/repositories"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
)

func main() {
	logger.InitZap()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config.AutomaticLoadEnv()

	validator.Init()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// database
	err := database.ConnectWithRetry(database.ConnectToDB, 5, 10*time.Second)
	if err != nil {
		logger.Fatalf("Failed to connect to database: %v", err)
	}
	jobRepository := repositories.NewJobRepository()
	mutationRepository := repositories.NewMutationRepository()
	queryResultRepository := repositories.NewQueryResultRepository()
	mutationResultRepository := repositories.NewMutationResultRepository()
	configurationRepository := repositories.NewConfigurationRepository()

	// conductor
	rabbitPublisher := rmqPublisher.NewPublisher()
	conductor := conductor.NewConductor(jobRepository, mutationRepository, queryResultRepository, mutationResultRepository, rabbitPublisher)
	rabbitConsumer := rmqConsumer.NewConsumer(conductor)

	amqpURL := config.Config.RabbitMqUrl
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		err := rabbitConsumer.RunConsumer(ctx, amqpURL, config.Config.JobQueue)
		if err != nil {
			logger.Fatalf("Error in RabbitMQ Consumer: %v", err)
		}
	}()

	// storage service
	storageService := storage.NewStorageService()

	// logging middleware
	router.Use(ginzap.GinzapWithConfig(logger.Zap, &ginzap.Config{
		TimeFormat: time.RFC3339,
		UTC:        true,
		SkipPaths:  []string{"/metrics", "/health"},
	}))

	// health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok", "environment": config.Config.Env})
	})

	// routes
	routes.JobRoute(router, jobRepository, mutationRepository, mutationResultRepository, configurationRepository, queryResultRepository, conductor)
	routes.MutationRoute(router, jobRepository, mutationRepository, mutationResultRepository, conductor)
	routes.ArtifactRoute(router, storageService)
	routes.UniProtRoute(router, storageService)
	routes.QueryResultRoute(router, jobRepository, queryResultRepository, conductor)

	// panic recovery
	router.Use(ginzap.RecoveryWithZap(logger.Zap, true))

	// start server
	httpPort := config.Config.HttpPort
	logger.Zap.Info("proteng-conductor is running on :" + httpPort)

	srv := &http.Server{Addr: ":" + httpPort, Handler: router}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Zap.Info("Shutdown signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("HTTP server shutdown: %v", err)
	}

	select {
	case <-consumerDone:
	case <-shutdownCtx.Done():
		logger.Errorf("Consumer did not stop before shutdown timeout")
	}

	if err := database.Client.Disconnect(shutdownCtx); err != nil {
		logger.Errorf("MongoDB disconnect: %v", err)
	}
	logger.Zap.Info("proteng-conductor stopped")
}
