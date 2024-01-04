package routes

import (
	"proteng-conductor/apis/controllers"
	"proteng-conductor/repositories"
	"proteng-conductor/services/conductor"

	"github.com/gin-gonic/gin"
)

func MutationRoute(router *gin.Engine, jr repositories.JobRepository, mr repositories.MutationRepository, con *conductor.Conductor) {
	mc := controllers.NewMutationController(jr, mr, con)

	router.GET("/mutations", mc.GetAllMutations)
	router.GET("/mutations/:id", mc.GetMutation)
	router.POST("/mutations", mc.CreateMutation)
	router.PUT("/mutations/:id", mc.UpdateMutation)
	router.DELETE("/mutations/:id", mc.DeleteMutation)
}
