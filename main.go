package main

import (

	// "context"
	// "net/http"
	// "os/signal"
	// "syscall"

	"os"
	"proteng-conductor/api/routes"
	"proteng-conductor/config"
	"proteng-conductor/database"
	"proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	config.AutomaticLoadEnv()

	router := gin.Default()

	// database
	err := database.ConnectToDB()
	if err != nil {
		logrus.Fatalf("Failed to connect to database: %v", err)
	}
	jobRepository := repositories.NewJobRepository()

	// rabbitmq consumer
	// rabbitConsumer := rabbitmq.RabbitConsumer()
	// amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%s/", os.Getenv("RABBITMQ_USER"), os.Getenv("RABBITMQ_PASSWORD"), os.Getenv("RABBITMQ_HOST"), os.Getenv("RABBITMQ_PORT"))
	// go func() {
	// 	err := rabbitConsumer.RunConsumer(amqpURL, "test")
	// 	if err != nil {
	// 		logrus.Fatalf("Error in RabbitMQ Consumer: %v", err)
	// 	}
	// }()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	// routes
	routes.JobRoute(router, jobRepository)

	// start server
	httpPort := os.Getenv("HTTP_PORT")
	err = router.Run(":" + httpPort)
	if err != nil {
		logrus.Fatalf("Failed to start server: %v", err)
	}
}

// func gracefulShutdown(server *http.Server) {
// 	quit := make(chan os.Signal, 1)
// 	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

// 	<-quit

// 	logrus.Info("Shuttingdown Server ...")
// 	if err := server.Shutdown(context.Background()); err != nil {
// 		logrus.Fatalf("Server shutdowned with error: %v", err)
// 	} else {
// 		logrus.Info("Server shutdowned gracefully.")
// 	}
// }
