package middleware

import (
	"net/http"
	"wappiz/svc/api/openapi"

	"github.com/gin-gonic/gin"
)

// WithErrorHandling returns middleware that translates errors into appropriate
func WithErrorHandling() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			// If there are errors, we can handle them here. For now, we'll just return a generic error response.
			err := c.Errors.Last().Err
			if err.Error() == "resource limit reached" {
				c.AbortWithStatusJSON(http.StatusForbidden, openapi.ForbiddenErrorResponse{
					Meta: openapi.Meta{
						RequestId: c.GetString("request_id"),
					},
					Error: openapi.BaseError{
						Title:  "Forbidden Access",
						Type:   "forbidden",
						Detail: "Se ha alcanzado el límite de recursos de tu plan. Actualiza tu plan para añadir más recursos.",
						Status: http.StatusForbidden,
					},
				})
			}
		}
	}
}
