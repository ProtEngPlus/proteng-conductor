package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func QueryResultRoute(router *gin.Engine, jr repositories.JobRepository, qr repositories.QueryResultRepository, con conductor.Conductor) {
	qrs := controllers.NewQueryResultController(jr, qr, con)

	router.GET("/query_results", qrs.GetAllQueryResults)
	router.GET("/query_results/:id", qrs.GetQueryResult)
	router.PUT("/query_results/:id", qrs.UpdateQueryResult)
	router.GET("/query_results/:id/download", qrs.DownloadQueryResult)
}
