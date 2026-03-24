// Package handlers provides HTTP request handlers for the Botka API.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondOK sends a 200 JSON response wrapping data in {"data": ...}.
func respondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// respondList sends a 200 JSON response wrapping data and total count in {"data": ..., "total": N}.
func respondList(c *gin.Context, data interface{}, total int64) {
	c.JSON(http.StatusOK, gin.H{"data": data, "total": total})
}

// respondError sends a JSON error response with the given HTTP status and message.
func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
