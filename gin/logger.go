package gin

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
)

const loggerKey = "logr.logger"

func Logger(cfg LogConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Inject loggin in gin context
		c.Set(loggerKey, cfg.Logger)

		// Do not log if path matches filter.
		if cfg.PathFilter != nil && cfg.PathFilter.MatchString(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Start timer
		start := time.Now()

		// Process request
		c.Next()

		// Stop timer
		latency := time.Now().Sub(start)

		// Log request
		path := c.Request.URL.Path
		statusCode := c.Writer.Status()
		kvs := []interface{}{"path", path, "status", statusCode, "method", c.Request.Method}
		if cfg.IncludeLatency {
			kvs = append(kvs, "latency", latency)
		}
		if cfg.IncludeClientIP {
			kvs = append(kvs, "ip", c.ClientIP())
		}
		for _, key := range cfg.IncludeKeys {
			v, ok := c.Keys[key]
			if !ok {
				continue
			}
			kvs = append(kvs, key, v)
		}

		// Info log if 2xx response
		if statusCode >= 200 && statusCode < 300 {
			cfg.Logger.Info("", kvs...)
			return
		}

		// Error log if any other status and include error message
		errs := []error{}
		for _, e := range c.Errors {
			errs = append(errs, e.Err)
		}
		cfg.Logger.Error(errors.Join(errs...), "", kvs...)
	}
}

func FromContextOrDiscard(c *gin.Context) logr.Logger {
	logVal, ok := c.Get(loggerKey)
	if !ok {
		return logr.Discard()
	}
	log, ok := logVal.(logr.Logger)
	if !ok {
		return logr.Discard()
	}
	return log
}
