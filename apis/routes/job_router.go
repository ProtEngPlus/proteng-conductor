package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func JobRoute(router *gin.Engine, jr repositories.JobRepository, mr repositories.MutationRepository, cr repositories.ConfigurationRepository, con conductor.Conductor) {
	jc := controllers.NewJobController(jr, mr, cr, con)

	router.GET("/jobs", jc.GetAllJobs)
	router.GET("/jobs/dashboard", jc.GetJobDashboard)
	router.GET("/jobs/:id", jc.GetJob)
	router.POST("/jobs", jc.CreateJob)
	router.PUT("/jobs/:id", jc.UpdateJob)
	router.DELETE("/jobs/:id", jc.DeleteJob)
	router.POST("/jobs/:id/run", jc.RunJob)
	router.POST("/jobs/configurations", jc.CreateConfigurations)
	router.GET("/jobs/configurations", jc.GetAllConfigurations)
}
