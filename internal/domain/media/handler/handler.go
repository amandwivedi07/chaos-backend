// Package handler exposes media upload on top of the storage port.
package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
	"github.com/chaosapp/backend/internal/storage"
)

const maxUploadBytes = 25 << 20 // 25 MB

// Content types a card may carry. Anything else is refused.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
	"image/heic": ".heic", "image/gif": ".gif",
	"video/mp4": ".mp4", "video/quicktime": ".mov", "video/webm": ".webm",
	"audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/aac": ".aac",
	"audio/webm": ".weba", "audio/x-m4a": ".m4a",
	"audio/wav": ".wav", "audio/x-wav": ".wav",
	"application/pdf": ".pdf",
}

type Handler struct {
	store storage.Storage
}

func New(store storage.Storage) *Handler { return &Handler{store: store} }

// Upload godoc
// @Summary  Upload a media file (image/video/audio/pdf, max 25 MB)
// @Tags     media
// @Accept   multipart/form-data
// @Produce  json
// @Param    file formData file true "the file"
// @Success  201 {object} response.Body
// @Security BearerAuth
// @Router   /media [post]
func (h *Handler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, apperrors.BadRequest("Attach a file under the 'file' field (max 25 MB)"))
		return
	}
	if file.Size > maxUploadBytes {
		response.Error(c, apperrors.BadRequest("File is larger than 25 MB"))
		return
	}

	declared := strings.ToLower(file.Header.Get("Content-Type"))
	ext, ok := allowedTypes[declared]
	if !ok {
		// Fall back to the filename extension for pickers that omit the type.
		ext = strings.ToLower(filepath.Ext(file.Filename))
		if !extAllowed(ext) {
			response.Error(c, apperrors.BadRequest(
				fmt.Sprintf("Unsupported file type %q", declared)))
			return
		}
	}
	// NEVER store the client's declared type: a spoofed text/html on an
	// allowed extension would be served as HTML from the media origin.
	contentType := canonicalType(ext)

	src, err := file.Open()
	if err != nil {
		response.Error(c, apperrors.Internal(err))
		return
	}
	defer src.Close()

	key := "media/" + uuid.NewString() + ext
	url, err := h.store.Put(c.Request.Context(), key, src, contentType)
	if err != nil {
		response.Error(c, apperrors.Internal(err))
		return
	}
	response.Created(c, "Uploaded", gin.H{
		"url":          url,
		"content_type": contentType,
		"size":         file.Size,
	})
}

func extAllowed(ext string) bool {
	for _, allowed := range allowedTypes {
		if allowed == ext {
			return true
		}
	}
	return false
}

// canonicalType maps a whitelisted extension back to the content type we are
// willing to serve. Unknown extensions never reach here.
func canonicalType(ext string) string {
	for mime, allowed := range allowedTypes {
		if allowed == ext {
			return mime
		}
	}
	return "application/octet-stream"
}
