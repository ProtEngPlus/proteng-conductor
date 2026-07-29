package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/storage"

	"github.com/gin-gonic/gin"
)

func UniProtRoute(router *gin.Engine, ss storage.StorageService) {
	up := controllers.NewUniProtController(ss)

	router.GET("/uniProt/:uniProtId", up.GetProteinSequenceFromId)
}
