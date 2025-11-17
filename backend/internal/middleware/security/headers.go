package security

import (
	"github.com/gofiber/fiber/v2"
)

type HeadersConfig struct {
	AllowedOrigins []string
	IsDevelopment  bool
}

func HeadersMiddleware(cfg HeadersConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		if !cfg.IsDevelopment {
			c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' 'unsafe-eval'; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: https:; " +
			"font-src 'self' data:; " +
			"connect-src 'self' " + buildConnectSrc(cfg.AllowedOrigins) + "; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
		c.Set("Content-Security-Policy", csp)

		return c.Next()
	}
}

func buildConnectSrc(origins []string) string {
	if len(origins) == 0 {
		return ""
	}

	connectSrc := ""
	for _, origin := range origins {
		connectSrc += origin + " "
	}
	return connectSrc
}
