package handlers

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/query"
	"github.com/aws-agent/backend/pkg/logger"
)

type WebSocketHandler struct {
	queryEngine *query.Engine
}

func NewWebSocketHandler(queryEngine *query.Engine) *WebSocketHandler {
	return &WebSocketHandler{
		queryEngine: queryEngine,
	}
}

func (h *WebSocketHandler) HandleConnection(c *websocket.Conn) {
	logger.Info("WebSocket connection established")

	defer func() {
		c.Close()
		logger.Info("WebSocket connection closed")
	}()

	for {
		var msg struct {
			Type    string `json:"type"`
			Content string `json:"content"`
			UserID  string `json:"user_id"`
		}

		err := c.ReadJSON(&msg)
		if err != nil {
			logger.Error("Failed to read WebSocket message", zap.Error(err))
			break
		}

		if msg.Type != "query" {
			continue
		}

		logger.Info("Processing WebSocket query", zap.String("query", msg.Content))

		err = h.streamResponse(c, msg.Content, msg.UserID)
		if err != nil {
			logger.Error("Failed to stream response", zap.Error(err))
			h.sendError(c, "Failed to process query")
		}
	}
}

func (h *WebSocketHandler) streamResponse(c *websocket.Conn, queryText, userID string) error {
	ctx := context.Background()

	req := query.QueryRequest{
		Query:  queryText,
		UserID: userID,
	}

	h.sendChunk(c, "status", "Processing query...")

	response, err := h.queryEngine.ProcessQuery(ctx, req)
	if err != nil {
		return err
	}

	words := splitIntoWords(response.Response)
	for i, word := range words {
		chunk := word
		if i < len(words)-1 {
			chunk += " "
		}

		err := h.sendChunk(c, "chunk", chunk)
		if err != nil {
			return err
		}
	}

	err = h.sendComplete(c, response)
	if err != nil {
		return err
	}

	return nil
}

func (h *WebSocketHandler) sendChunk(c *websocket.Conn, msgType, content string) error {
	msg := map[string]interface{}{
		"type":    msgType,
		"content": content,
	}

	return c.WriteJSON(msg)
}

func (h *WebSocketHandler) sendComplete(c *websocket.Conn, response *query.QueryResponse) error {
	msg := map[string]interface{}{
		"type":       "complete",
		"message_id": response.ID,
		"sources":    response.Sources,
		"confidence": response.Confidence,
		"latency_ms": response.LatencyMS,
	}

	return c.WriteJSON(msg)
}

func (h *WebSocketHandler) sendError(c *websocket.Conn, errorMsg string) {
	msg := map[string]interface{}{
		"type":  "error",
		"error": errorMsg,
	}

	c.WriteJSON(msg)
}

func splitIntoWords(text string) []string {
	words := []string{}
	currentWord := ""

	for _, char := range text {
		if char == ' ' || char == '\n' {
			if currentWord != "" {
				words = append(words, currentWord)
				currentWord = ""
			}
			if char == '\n' {
				words = append(words, "\n")
			}
		} else {
			currentWord += string(char)
		}
	}

	if currentWord != "" {
		words = append(words, currentWord)
	}

	return words
}
