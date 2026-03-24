package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

const maxAudioSize = 25 << 20 // 25 MB (Whisper API limit)

// TranscribeHandler proxies audio transcription to OpenClaw's whisper endpoint.
type TranscribeHandler struct {
	openClawURL   string
	openClawToken string
	enabled       bool
}

// NewTranscribeHandler creates a new TranscribeHandler.
func NewTranscribeHandler(openClawURL, openClawToken string, enabled bool) *TranscribeHandler {
	return &TranscribeHandler{
		openClawURL:   openClawURL,
		openClawToken: openClawToken,
		enabled:       enabled,
	}
}

// RegisterTranscribeRoutes attaches transcription endpoints to the given router group.
func RegisterTranscribeRoutes(rg *gin.RouterGroup, h *TranscribeHandler) {
	rg.GET("/transcribe/status", h.Status)
	rg.POST("/transcribe", h.Transcribe)
}

// Status returns whether whisper transcription is enabled.
func (h *TranscribeHandler) Status(c *gin.Context) {
	respondOK(c, gin.H{"enabled": h.enabled})
}

// Transcribe proxies an audio file to OpenClaw's whisper endpoint and returns the transcription.
func (h *TranscribeHandler) Transcribe(c *gin.Context) {
	if !h.enabled {
		respondError(c, http.StatusServiceUnavailable, "whisper transcription is not enabled")
		return
	}

	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		respondError(c, http.StatusBadRequest, "missing audio file")
		return
	}
	defer func() { _ = file.Close() }()

	if header.Size > maxAudioSize {
		respondError(c, http.StatusRequestEntityTooLarge, "audio file too large (max 25 MB)")
		return
	}

	// Build multipart request for OpenAI-compatible transcription API
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", header.Filename)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to build request")
		return
	}
	if _, err := io.Copy(fw, file); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to read audio")
		return
	}

	if err := mw.WriteField("model", "whisper-1"); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to build request")
		return
	}

	if lang := c.Request.FormValue("lang"); lang != "" {
		if err := mw.WriteField("language", lang); err != nil {
			respondError(c, http.StatusInternalServerError, "failed to build request")
			return
		}
	}

	if err := mw.Close(); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to build request")
		return
	}

	url := h.openClawURL + "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, &buf)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create request")
		return
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if h.openClawToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.openClawToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("whisper API error", "error", err)
		respondError(c, http.StatusBadGateway, "transcription service unavailable")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("whisper API error", "status", resp.StatusCode, "body", string(body))
		respondError(c, http.StatusBadGateway, fmt.Sprintf("transcription failed (status %d)", resp.StatusCode))
		return
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to parse transcription response")
		return
	}

	respondOK(c, gin.H{"text": result.Text})
}
