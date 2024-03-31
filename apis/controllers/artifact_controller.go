package controllers

import (
	"github.com/gin-gonic/gin"
	"github.com/protengplus/proteng-conductor/models"
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
	bucketName := c.Param("bucketName")
	objectName := c.Param("objectName")

	// Retrieve artifact content from storage service
	content, err := ac.storageService.GetObjectContent(bucketName, objectName)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
		return
	}

	// Create an Artifact object with the retrieved content
	artifact := models.Artifact{
		BucketName: bucketName,
		Path:       objectName,
		Content:    content,
	}

	// Serve the artifact as the response
	apiutil.ApiResponseOk(c, artifact)
}
