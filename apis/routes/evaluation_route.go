package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func EvaluationRoute(router *gin.Engine, jr repositories.JobRepository, mrr repositories.MutationResultRepository, evr repositories.EvaluationRunRepository, eres repositories.EvaluationResultRepository) {
	ec := controllers.NewEvaluationController(jr, mrr, evr, eres)

	router.GET("/evaluation-plugins", ec.GetEvaluationPlugins)
	router.GET("/jobs/:id/evaluations", ec.GetJobEvaluations)
	router.GET("/evaluations/:id", ec.GetEvaluation)
}
