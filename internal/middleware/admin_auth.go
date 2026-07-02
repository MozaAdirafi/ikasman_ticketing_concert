package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		pin := c.GetHeader("X-Admin-PIN")
		if pin == "" {
			pin = c.Query("pin")
		}

		expectedPIN := os.Getenv("ADMIN_PIN")
		if expectedPIN == "" {
			expectedPIN = "1234"
		}

		if pin != expectedPIN {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Next()
	}
}
