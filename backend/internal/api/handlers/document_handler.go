package handlers

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/ingestion"
	"github.com/aws-agent/backend/pkg/logger"
)

type DocumentHandler struct {
	processor *ingestion.Processor
}

func NewDocumentHandler(processor *ingestion.Processor) *DocumentHandler {
	return &DocumentHandler{
		processor: processor,
	}
}

func (h *DocumentHandler) UploadDocument(c *fiber.Ctx) error {
	var req struct {
		URL         string `json:"url"`
		HTMLContent string `json:"html_content"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error("Failed to parse request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.URL == "" || req.HTMLContent == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "URL and HTML content are required",
		})
	}

	err := h.processor.ProcessDocument(c.Context(), req.URL, req.HTMLContent)
	if err != nil {
		logger.Error("Failed to process document", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process document",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Document processed successfully",
		"url":     req.URL,
	})
}
