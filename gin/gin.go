package gin

import (
	gogin "github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	metricsmiddleware "github.com/slok/go-http-metrics/middleware"
	ginmetricsmiddleware "github.com/slok/go-http-metrics/middleware/gin"

	"github.com/xenitab/pkg/gin/middleware"
)

func Default(logger logr.Logger, handlerID string) *gogin.Engine {
	gogin.SetMode(gogin.ReleaseMode)
	mdlw := metricsmiddleware.New(metricsmiddleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{}),
	})
	engine := gogin.New()
	engine.Use(middleware.Logger(logger, false))
	engine.Use(ginmetricsmiddleware.Handler(handlerID, mdlw))
	engine.Use(gogin.Recovery())
	return engine
}
