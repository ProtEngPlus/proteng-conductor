package apiutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestApiResponseForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ApiResponseForbidden(c, fmt.Errorf("not yours"))

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.JSONEq(t, `{"code":403,"error":"not yours","message":""}`, w.Body.String())
}
