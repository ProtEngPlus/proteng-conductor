package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/storage"

	"github.com/gin-gonic/gin"
)

func ArtifactRoute(router *gin.Engine, ss storage.StorageService) {
	ac := controllers.NewArtifactController(ss)

	router.GET("/artifact/:bucketName/:objectName", ac.DownloadArtifact)
}
