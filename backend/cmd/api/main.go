package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/api/handlers"
	"github.com/aws-agent/backend/internal/aws/actions"
	"github.com/aws-agent/backend/internal/cache/redis"
	"github.com/aws-agent/backend/internal/evaluation"
	"github.com/aws-agent/backend/internal/ingestion"
	"github.com/aws-agent/backend/internal/kg/builder"
	"github.com/aws-agent/backend/internal/kg/neo4j"
	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/internal/metrics"
	"github.com/aws-agent/backend/internal/middleware/ratelimit"
	"github.com/aws-agent/backend/internal/middleware/security"
	"github.com/aws-agent/backend/internal/middleware/validation"
	"github.com/aws-agent/backend/internal/query"
	"github.com/aws-agent/backend/internal/search/web"
	"github.com/aws-agent/backend/internal/storage/sqlite"
	"github.com/aws-agent/backend/internal/vector/zilliz"
	"github.com/aws-agent/backend/pkg/config"
	appLogger "github.com/aws-agent/backend/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	err = appLogger.Init(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.OutputPath)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer appLogger.Sync()

	appLogger.Info("Starting AWS RAG Agent API Server with Enhanced Features")

	metrics.Init()

	sqliteClient, err := sqlite.NewClient(cfg.SQLite.Path)
	if err != nil {
		appLogger.Fatal("Failed to create SQLite client", zap.Error(err))
	}
	defer sqliteClient.Close()

	sqliteClient.DB.SetMaxOpenConns(25)
	sqliteClient.DB.SetMaxIdleConns(5)
	sqliteClient.DB.SetConnMaxLifetime(5 * time.Minute)

	err = sqliteClient.InitSchema()
	if err != nil {
		appLogger.Fatal("Failed to initialize schema", zap.Error(err))
	}

	neo4jClient, err := neo4j.NewClient(
		cfg.Neo4j.URI,
		cfg.Neo4j.Username,
		cfg.Neo4j.Password,
		cfg.Neo4j.Database,
	)
	if err != nil {
		appLogger.Fatal("Failed to create Neo4j client", zap.Error(err))
	}
	defer neo4jClient.Close(context.Background())

	zillizClient, err := zilliz.NewClient(
		cfg.Zilliz.Endpoint,
		cfg.Zilliz.APIKey,
		cfg.Zilliz.CollectionName,
		cfg.Zilliz.VectorDim,
	)
	if err != nil {
		appLogger.Fatal("Failed to create Zilliz client", zap.Error(err))
	}
	defer zillizClient.Close()

	err = zillizClient.CreateCollection(context.Background())
	if err != nil {
		appLogger.Fatal("Failed to create collection", zap.Error(err))
	}

	redisClient, err := redis.NewClient(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)
	if err != nil {
		appLogger.Warn("Failed to create Redis client, continuing without cache", zap.Error(err))
	} else {
		defer redisClient.Close()
	}

	llmClient := llm.NewClient(
		cfg.LLM.APIKey,
		cfg.LLM.Model,
		cfg.LLM.EmbeddingModel,
		cfg.LLM.Temperature,
		cfg.LLM.MaxTokens,
	)

	kgBuilder := builder.NewBuilder(sqliteClient, neo4jClient, llmClient)
	err = kgBuilder.InitializeSeedConcepts()
	if err != nil {
		appLogger.Warn("Failed to initialize seed concepts", zap.Error(err))
	}

	webSearchClient := web.NewClient(cfg.Search.SerpAPIKey, llmClient)
	processor := ingestion.NewProcessor(sqliteClient, zillizClient, llmClient)
	queryEngine := query.NewEngine(sqliteClient, neo4jClient, zillizClient, llmClient)
	evaluator := evaluation.NewEvaluator(sqliteClient, llmClient)
	actionsExecutor := actions.NewExecutor(llmClient, true)

	app := fiber.New(fiber.Config{
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		BodyLimit:    cfg.Server.BodyLimit,
		ServerHeader: "AWS-RAG-Agent",
		AppName:      "AWS RAG Agent v2.0",
	})

	app.Use(recover.New())
	app.Use(logger.New())

	allowedOrigins := "http://localhost:3000"
	if cfg.Server.AllowedOrigins != "" {
		allowedOrigins = cfg.Server.AllowedOrigins
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-User-ID",
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
		AllowCredentials: true,
		MaxAge:           3600,
	}))

	app.Use(security.HeadersMiddleware(security.HeadersConfig{
		AllowedOrigins: []string{allowedOrigins},
		IsDevelopment:  cfg.Server.Environment == "development",
	}))

	app.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	rateLimiter := ratelimit.New(ratelimit.Config{
		MaxRequestsPerMinute: 60,
		WindowDuration:       time.Minute,
		Logger:               appLogger.GetLogger(),
	})
	app.Use(rateLimiter.Middleware())

	app.Use(validation.Middleware(validation.Config{
		MaxQueryLength:      5000,
		MaxDocumentSize:     10 * 1024 * 1024,
		AllowedContentTypes: []string{"application/json", "multipart/form-data"},
		Logger:              appLogger.GetLogger(),
	}))

	queryHandler := handlers.NewQueryHandler(queryEngine)
	documentHandler := handlers.NewDocumentHandler(processor)
	wsHandler := handlers.NewWebSocketHandler(queryEngine)
	actionsHandler := handlers.NewActionsHandler(actionsExecutor)

	api := app.Group("/api/v1")

	api.Post("/query", queryHandler.HandleQuery)
	api.Get("/query/history", queryHandler.GetQueryHistory)

	api.Get("/ws", websocket.New(wsHandler.HandleConnection))

	api.Post("/documents", documentHandler.UploadDocument)

	api.Post("/actions/plan", actionsHandler.PlanActions)
	api.Post("/actions/execute", actionsHandler.ExecuteActions)

	api.Get("/metrics", metrics.MetricsHandler())

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"time":   time.Now().Unix(),
			"features": map[string]bool{
				"redis_cache":    redisClient != nil,
				"web_search":     cfg.Search.Enabled,
				"websocket":      true,
				"aws_actions":    true,
				"metrics":        true,
			},
		})
	})

	api.Get("/ready", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ready",
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	appLogger.Info("Server starting with enhanced features",
		zap.String("address", addr),
		zap.Bool("redis_cache", redisClient != nil),
		zap.Bool("web_search", cfg.Search.Enabled),
		zap.Bool("websocket", true),
		zap.Bool("aws_actions", true),
	)

	go func() {
		if err := app.Listen(addr); err != nil {
			appLogger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	appLogger.Info("Server shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if redisClient != nil {
		appLogger.Info("Closing Redis connection...")
		redisClient.Close()
	}

	if err := neo4jClient.Close(shutdownCtx); err != nil {
		appLogger.Error("Error closing Neo4j connection", zap.Error(err))
	}

	if err := zillizClient.Close(); err != nil {
		appLogger.Error("Error closing Zilliz connection", zap.Error(err))
	}

	if err := sqliteClient.Close(); err != nil {
		appLogger.Error("Error closing SQLite connection", zap.Error(err))
	}

	rateLimiter.Stop()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		appLogger.Error("Error during server shutdown", zap.Error(err))
	}

	appLogger.Info("Server stopped successfully")
}
