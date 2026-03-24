// Package middleware provides HTTP middleware for the Botka server.
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS returns Gin middleware that allows all origins.
// Botka is a local network tool with no authentication,
// so permissive CORS is appropriate.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Max-Age", "43200")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
