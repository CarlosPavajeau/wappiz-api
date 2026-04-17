package middleware

import (
	"wappiz/pkg/otel/tracing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

func ObservabilityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, span := tracing.Start(c, c.HandlerName())
		span.SetAttributes(attribute.String("request_id", c.Request.Header.Get("X-Request-Id")))
		defer span.End()

		c.Next()

		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				tracing.RecordError(span, err)
			}
		}
	}
}
