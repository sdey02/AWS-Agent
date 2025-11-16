package handlers

import (
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/aws/actions"
	"github.com/aws-agent/backend/pkg/logger"
)

type ActionsHandler struct {
	executor *actions.Executor
}

func NewActionsHandler(executor *actions.Executor) *ActionsHandler {
	return &ActionsHandler{
		executor: executor,
	}
}

func (h *ActionsHandler) PlanActions(c *fiber.Ctx) error {
	var req struct {
		Issue   string `json:"issue"`
		Context string `json:"context"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error("Failed to parse request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	plan, err := h.executor.PlanActions(c.Context(), req.Issue, req.Context)
	if err != nil {
		logger.Error("Failed to plan actions", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to plan actions",
		})
	}

	return c.JSON(fiber.Map{
		"plan":              plan.Actions,
		"explanation":       plan.Explanation,
		"risk_level":        plan.RiskLevel,
		"requires_approval": plan.RequiresApproval,
	})
}

func (h *ActionsHandler) ExecuteActions(c *fiber.Ctx) error {
	var req struct {
		Plan     actions.ActionPlan `json:"plan"`
		Approved bool               `json:"approved"`
	}

	if err := c.BodyParser(&req); err != nil {
		logger.Error("Failed to parse request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	results, err := h.executor.ExecuteActions(c.Context(), &req.Plan, req.Approved)
	if err != nil {
		logger.Error("Failed to execute actions", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"results": results,
	})
}
