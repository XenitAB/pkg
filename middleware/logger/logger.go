package logger

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"go.uber.org/multierr"
)

func Logger(logger logr.Logger) gin.HandlerFunc {
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

    // Info log if 2xx response
    if statusCode >= 200 && statusCode < 300 {
      logger.Info("", "path", path, "status", statusCode, "method", c.Request.Method, "latency", latency, "ip", c.ClientIP())
      return
    }

    // Error log if any other status and include error message
    var err error
    for _, e := range c.Errors {
      err = multierr.Append(err, e.Err)
    }
		logger.Error(err, "", "path", path, "status", statusCode, "method", c.Request.Method, "latency", latency, "ip", c.ClientIP())
	}
}
