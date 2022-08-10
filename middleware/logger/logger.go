package logger

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
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
		if c.Request.URL.RawQuery != "" {
			path = path + "?" + c.Request.URL.RawQuery
		}
		lastErr := c.Errors.ByType(gin.ErrorTypePrivate).Last()
		if lastErr != nil {
			logger.Error(lastErr.Err, "", "path", path, "status", c.Writer.Status(), "method", c.Request.Method, "latency", latency, "ip", c.ClientIP())
			return
		}
		logger.Info("", "path", path, "status", c.Writer.Status(), "method", c.Request.Method, "latency", latency, "ip", c.ClientIP())
	}
}
