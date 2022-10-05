package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tonglil/buflogr"
)
  
func TestLogOk(t *testing.T) {
  var buf bytes.Buffer
  log := buflogr.NewWithBuffer(&buf)
  mdlw := Logger(log, false)
  gin.SetMode(gin.TestMode)
  c, _ := gin.CreateTestContext(httptest.NewRecorder())
  c.Request = httptest.NewRequest("GET", "/foo", nil)
  mdlw(c)
  require.Equal(t, "INFO path /foo status 200 method GET ip 192.0.2.1\n", string(buf.Bytes()))
}

func TestLogInternalServerError(t *testing.T) {
  var buf bytes.Buffer
  log := buflogr.NewWithBuffer(&buf)
  mdlw := Logger(log, false)
  gin.SetMode(gin.TestMode)
  c, _ := gin.CreateTestContext(httptest.NewRecorder())
  c.Request = httptest.NewRequest("POST", "/bar", nil)
  c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("hello world"))
  mdlw(c)
  require.Equal(t, "ERROR hello world path /bar status 500 method POST ip 192.0.2.1\n", string(buf.Bytes()))
}
