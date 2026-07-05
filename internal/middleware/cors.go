package middleware

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	wwwOrigin := ""
	if parsed, err := url.Parse(frontendURL); err == nil && parsed.Host != "" {
		wwwOrigin = fmt.Sprintf("%s://www.%s", parsed.Scheme, strings.TrimPrefix(parsed.Host, "www."))
	}

	allowedOrigins := []string{
		frontendURL,
		"https://ikasman-ticketing-concert-fe.vercel.app",
		"http://localhost:3000",
		"http://localhost:3001",
	}
	if wwwOrigin != "" {
		allowedOrigins = append(allowedOrigins, wwwOrigin)
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

		requestHeaders := c.GetHeader("Access-Control-Request-Headers")
		if requestHeaders != "" {
			c.Header("Access-Control-Allow-Headers", requestHeaders)
		} else {
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Admin-PIN")
		}
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
