package middleware

import (
	"log/slog"
	"net/http"

	"ila-engine/pkg/httpx"

	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Unhandled panic recovered", "panic", r)
				httpx.Error(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Internal server error occurred", nil)
				c.Abort()
			}
		}()
		c.Next()
	}
}
