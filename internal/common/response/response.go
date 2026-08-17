// Package response defines the single JSON envelope every endpoint returns.
//
//	success: {"success": true,  "message": "...", "data": {...}}
//	failure: {"success": false, "message": "...", "error": "...", "errors": {...}, "data": null}
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
)

type Body struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Data    any               `json:"data"`
	Error   string            `json:"error,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// Meta is embedded in list payloads for pagination.
type Meta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type ListData struct {
	Items any  `json:"items"`
	Meta  Meta `json:"meta"`
}

func OK(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Body{Success: true, Message: message, Data: data})
}

func Created(c *gin.Context, message string, data any) {
	c.JSON(http.StatusCreated, Body{Success: true, Message: message, Data: data})
}

func List(c *gin.Context, message string, items any, meta Meta) {
	OK(c, message, ListData{Items: items, Meta: meta})
}

// Error renders any error through the AppError mapping — the one exit path
// for failures, so the envelope never drifts.
func Error(c *gin.Context, err error) {
	app := apperrors.From(err)
	c.AbortWithStatusJSON(app.HTTPStatus(), Body{
		Success: false,
		Message: app.Message,
		Error:   string(app.Kind),
		Errors:  app.Fields,
		Data:    nil,
	})
}
