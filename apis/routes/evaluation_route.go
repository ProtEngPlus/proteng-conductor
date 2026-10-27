package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func EvaluationRoute(router *gin.Engine, jr repositories.JobRepository, mr repositories.MutationRepository, mrr repositories.MutationResultRepository, evr repositories.EvaluationRunRepository, eres repositories.EvaluationResultRepository, con conductor.Conductor) {
	ec := controllers.NewEvaluationController(jr, mr, mrr, evr, eres, con)

	router.GET("/evaluation-plugins", ec.GetEvaluationPlugins)
	router.GET("/jobs/:id/evaluations", ec.GetJobEvaluations)
	router.GET("/evaluations/:id", ec.GetEvaluation)
	router.POST("/jobs/:id/evaluations", ec.RerunEvaluation)
}
