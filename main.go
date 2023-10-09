package main

import (
	"os"
	// "context"
	// "net/http"
	// "os/signal"
	// "syscall"

	"proteng-conductor/api"
	"proteng-conductor/api/job"
	"proteng-conductor/config"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	config.AutomaticLoadEnv()

	// dependency injection
	jobApi := &job.JobApi{}

	// serve gin server
	r := gin.Default()
	api.RegisterRoutes(r, jobApi)
	httpPort := os.Getenv("HTTP_PORT")
	err := r.Run(":" + httpPort)
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
