package apiutil

import (
	"fmt"
	"net/http"

	"github.com/protengplus/proteng-conductor/models"

	"github.com/gin-gonic/gin"
)

func ApiResponseOk(c *gin.Context, data interface{}, messages ...string) {
	if len(messages) == 0 {
		messages = append(messages, "")
	}
	c.JSON(http.StatusOK, models.HttpResponseOK{
		Code:    http.StatusOK,
		Data:    data,
		Message: messages[0],
	})
}

func ApiResponseErrorBadRequest(c *gin.Context, err error, messages ...string) {
	if len(messages) == 0 {
		messages = append(messages, "")
	}
	if err == nil {
		err = fmt.Errorf("bad request")
	}
	c.JSON(http.StatusBadRequest, models.HttpResponseError{
		Code:    http.StatusBadRequest,
		Error:   err.Error(),
		Message: messages[0],
	})
}

func ApiResponseInternalServerError(c *gin.Context, err error, messages ...string) {
	if len(messages) == 0 {
		messages = append(messages, "")
	}
	if err == nil {
		err = fmt.Errorf("internal server error")
	}
	c.JSON(http.StatusInternalServerError, models.HttpResponseError{
		Code:    http.StatusInternalServerError,
		Error:   err.Error(),
		Message: messages[0],
	})
}

func ApiResponseNotFound(c *gin.Context, err error, messages ...string) {
	if len(messages) == 0 {
		messages = append(messages, "")
	}
	if err == nil {
		err = fmt.Errorf("not found")
	}
	c.JSON(http.StatusNotFound, models.HttpResponseError{
		Code:    http.StatusNotFound,
		Error:   err.Error(),
		Message: messages[0],
	})
}
