package routes

import (
	"github.com/protengplus/proteng-conductor/apis/controllers"
	"github.com/protengplus/proteng-conductor/repositories"

	"github.com/gin-gonic/gin"
)

func AssayTypeRoute(router *gin.Engine, atr repositories.AssayTypeRepository, exr repositories.ExperimentalResultRepository, mrr repositories.MutationResultRepository, jr repositories.JobRepository) {
	ac := controllers.NewAssayTypeController(atr, exr, mrr, jr)

	router.GET("/assay-types", ac.GetAllAssayTypes)
	router.POST("/assay-types", ac.CreateAssayType)
	router.GET("/assay-types/:id", ac.GetAssayType)
	router.PUT("/assay-types/:id", ac.UpdateAssayType)
	router.DELETE("/assay-types/:id", ac.DeleteAssayType)
	router.GET("/assay-types/:id/library/jobs", ac.GetLibraryJobs)
	router.GET("/assay-types/:id/library", ac.GetLibraryResults)
	router.GET("/assay-types/:id/library/export", ac.ExportLibrary)
}
