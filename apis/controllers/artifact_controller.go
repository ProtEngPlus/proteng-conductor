package controllers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/protengplus/proteng-conductor/storage"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
)

type ArtifactController struct {
	storageService storage.StorageService
}

func NewArtifactController(storageService storage.StorageService) *ArtifactController {
	return &ArtifactController{storageService: storageService}
}

// DownloadArtifact downloads the artifact by bucket and object name
func (ac *ArtifactController) DownloadArtifact(c *gin.Context) {
	bucketName := c.Param("bucket")
	objectName := c.Param("object")

	// Retrieve artifact content from storage service
	content, err := ac.storageService.GetObjectContent(bucketName, objectName)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}
	fmt.Println(content)

	// Serve the artifact
	apiutil.ApiResponseOk(c, content)
}
