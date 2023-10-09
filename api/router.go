package api

import (
	"proteng-conductor/api/job"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, jobApi *job.JobApi) {
	jobRouter := r.Group("/api/job")
	jobRouter.POST("/start", jobApi.StartJob)

}
