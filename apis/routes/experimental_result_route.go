package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func ExperimentalResultRoute(router *gin.Engine, mrr repositories.MutationResultRepository, exr repositories.ExperimentalResultRepository) {
	erc := controllers.NewExperimentalResultController(mrr, exr)

	router.GET("/mutations/experimental-results", erc.GetAllExperimentalResults)
	router.PUT("/mutations/experimental-results/by-mutation-result/:mutationResultId", erc.UpsertExperimentalResult)
	router.DELETE("/mutations/experimental-results/by-mutation-result/:mutationResultId", erc.DeleteExperimentalResult)
}
