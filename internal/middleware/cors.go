package middleware

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	wwwOrigin := "https://www." + strings.TrimPrefix(frontendURL, "https://")
	allowedOrigins := []string{
		frontendURL,
		wwwOrigin,
		"https://ikasman-ticketing-concert-fe.vercel.app",
	}

	return func(c *gin.Context) {
		requestOrigin := c.GetHeader("Origin")
		for _, allowedOrigin := range allowedOrigins {
			if requestOrigin != "" && requestOrigin == allowedOrigin {
				c.Header("Access-Control-Allow-Origin", requestOrigin)
				break
			}
		}

		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
