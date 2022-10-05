package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"go.uber.org/multierr"
)

func Logger(logger logr.Logger, includeLatency bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Process request
		c.Next()

		// Stop timer
		latency := time.Now().Sub(start)

		// Log request
    path := c.Request.URL.Path
    statusCode := c.Writer.Status()
    kvs := []interface{}{"path", path, "status", statusCode, "method", c.Request.Method, "ip", c.ClientIP()}
    if includeLatency {
      kvs = append(kvs, "latency", latency)
    }

    // Info log if 2xx response
    if statusCode >= 200 && statusCode < 300 {
      logger.Info("", kvs...)
      return
    }

    // Error log if any other status and include error message
    var err error
    for _, e := range c.Errors {
      err = multierr.Append(err, e.Err)
    }
		logger.Error(err, "", kvs...)
	}
}
