package gin

import (
	"regexp"

	gogin "github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	metricsmiddleware "github.com/slok/go-http-metrics/middleware"
	ginmetricsmiddleware "github.com/slok/go-http-metrics/middleware/gin"
)

type Config struct {
	LogConfig     LogConfig
	MetricsConfig MetricsConfig
}

type LogConfig struct {
	// Logger instance to output request logs.
	Logger logr.Logger
	// Regex used to filter out request logs.
	PathFilter *regexp.Regexp
	// Should request logs should include latency.
	IncludeLatency bool
	// Should request logs should include client IP.
	IncludeClientIP bool
}

type MetricsConfig struct {
	// Handler ID to use when using dynamic path parameters.
	HandlerID string
}

func DefaultConfig() Config {
	return Config{
		LogConfig: LogConfig{
			Logger:          logr.Discard(),
			PathFilter:      nil,
			IncludeLatency:  true,
			IncludeClientIP: false,
		},
		MetricsConfig: MetricsConfig{
			HandlerID: "",
		},
	}
}

func NewEngine(cfg Config) *gogin.Engine {
	gogin.SetMode(gogin.ReleaseMode)
	mdlw := metricsmiddleware.New(metricsmiddleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
	})
	engine := gogin.New()
	engine.Use(Logger(cfg.LogConfig))
	engine.Use(ginmetricsmiddleware.Handler(cfg.MetricsConfig.HandlerID, mdlw))
	engine.Use(gogin.Recovery())
	return engine
}
