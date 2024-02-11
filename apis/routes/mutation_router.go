package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func MutationRoute(router *gin.Engine, jr repositories.JobRepository, mr repositories.MutationRepository, con conductor.Conductor) {
	mc := controllers.NewMutationController(jr, mr, con)

	router.GET("/mutations", mc.GetAllMutations)
	router.GET("/mutations/:id", mc.GetMutation)
	router.POST("/mutations", mc.CreateMutation)
	router.PUT("/mutations/:id", mc.UpdateMutation)
	router.DELETE("/mutations/:id", mc.DeleteMutation)
}
