package routes

import (
	"proteng-conductor/api/controllers"
	"proteng-conductor/internal/conductor"
	"proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func JobRoute(router *gin.Engine, jr repositories.JobRepository, con *conductor.Conductor) {
	jc := controllers.NewJobController(jr, con)

	router.GET("/jobs", jc.GetAllJobs)
	router.GET("/jobs/:id", jc.GetJob)
	router.POST("/jobs", jc.CreateJob)
	router.PUT("/jobs/:id", jc.UpdateJob)
	router.DELETE("/jobs/:id", jc.DeleteJob)
	router.POST("/jobs/:id/run", jc.RunJob)
}
