package handlers

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/query"
	"github.com/aws-agent/backend/pkg/logger"
)

type QueryHandler struct {
	queryEngine *query.Engine
}

func NewQueryHandler(queryEngine *query.Engine) *QueryHandler {
	return &QueryHandler{
		queryEngine: queryEngine,
	}
}

func (h *QueryHandler) HandleQuery(c *fiber.Ctx) error {
	var req struct {
		Query  string `json:"query"`
		UserID string `json:"user_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error("Failed to parse request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Query is required",
		})
	}

	queryReq := query.QueryRequest{
		Query:  req.Query,
		UserID: req.UserID,
	}

	response, err := h.queryEngine.ProcessQuery(c.Context(), queryReq)
	if err != nil {
		logger.Error("Failed to process query", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to process query",
		})
	}

	return c.JSON(fiber.Map{
		"id":          response.ID,
		"query":       response.Query,
		"response":    response.Response,
		"sources":     response.Sources,
		"confidence":  response.Confidence,
		"latency_ms":  response.LatencyMS,
	})
}

func (h *QueryHandler) GetQueryHistory(c *fiber.Ctx) error {
	userID := c.Query("user_id")
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "user_id is required",
		})
	}

	return c.JSON(fiber.Map{
		"history": []interface{}{},
	})
}
