package job

import (
	"net/http"

	"proteng-conductor/model"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type JobApi struct {
}

func (api *JobApi) StartJob(gctx *gin.Context) {
	logrus.Info("[JobApi] StartJob")

	gctx.JSON(http.StatusOK, model.SomeResponse{
		Message: "Hello World",
	})
}
