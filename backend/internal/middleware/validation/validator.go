package validation

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

var (
	sqlInjectionPattern = regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|script|javascript|onerror|onload)`)
	xssPattern          = regexp.MustCompile(`(?i)(<script|<iframe|javascript:|onerror=|onload=|onclick=)`)
)

type Config struct {
	MaxQueryLength      int
	MaxDocumentSize     int
	AllowedContentTypes []string
	Logger              *zap.Logger
}

func Middleware(cfg Config) fiber.Handler {
	if cfg.MaxQueryLength == 0 {
		cfg.MaxQueryLength = 5000
	}
	if cfg.MaxDocumentSize == 0 {
		cfg.MaxDocumentSize = 10 * 1024 * 1024
	}
	if len(cfg.AllowedContentTypes) == 0 {
		cfg.AllowedContentTypes = []string{"application/json", "multipart/form-data"}
	}

	return func(c *fiber.Ctx) error {
		if c.Method() == "POST" || c.Method() == "PUT" {
			contentType := c.Get("Content-Type")
			if contentType != "" {
				allowed := false
				for _, allowedType := range cfg.AllowedContentTypes {
					if strings.Contains(contentType, allowedType) {
						allowed = true
						break
					}
				}
				if !allowed {
					return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
						"error": "Unsupported content type",
					})
				}
			}
		}

		path := c.Path()

		if strings.Contains(path, "/api/v1/query") {
			var req map[string]interface{}
			if err := c.BodyParser(&req); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid JSON format",
				})
			}

			query, ok := req["query"].(string)
			if !ok || query == "" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Query is required and must be a string",
				})
			}

			if len(query) > cfg.MaxQueryLength {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Query exceeds maximum length",
				})
			}

			if containsSQLInjection(query) {
				cfg.Logger.Warn("Potential SQL injection attempt",
					zap.String("ip", c.IP()),
					zap.String("query", query),
				)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid query content",
				})
			}

			if containsXSS(query) {
				cfg.Logger.Warn("Potential XSS attempt",
					zap.String("ip", c.IP()),
					zap.String("query", query),
				)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid query content",
				})
			}

			sanitized := sanitizeString(query)
			req["query"] = sanitized
			c.Locals("sanitized_body", req)
		}

		if strings.Contains(path, "/api/v1/documents") {
			var req map[string]interface{}
			if err := c.BodyParser(&req); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid JSON format",
				})
			}

			urlStr, ok := req["url"].(string)
			if !ok || urlStr == "" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "URL is required and must be a string",
				})
			}

			if !isValidURL(urlStr) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": "Invalid URL format",
				})
			}

			content, ok := req["content"].(string)
			if ok && len(content) > cfg.MaxDocumentSize {
				return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
					"error": "Document content exceeds maximum size",
				})
			}
		}

		return c.Next()
	}
}

func containsSQLInjection(input string) bool {
	return sqlInjectionPattern.MatchString(input)
}

func containsXSS(input string) bool {
	return xssPattern.MatchString(input)
}

func sanitizeString(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "\x00", "")
	return input
}

func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	if u.Host == "" {
		return false
	}

	return true
}
