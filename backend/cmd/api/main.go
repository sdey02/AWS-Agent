package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/api/handlers"
	"github.com/aws-agent/backend/internal/ingestion"
	"github.com/aws-agent/backend/internal/kg/builder"
	"github.com/aws-agent/backend/internal/kg/neo4j"
	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/internal/query"
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

	appLogger.Info("Starting AWS RAG Agent API Server")

	sqliteClient, err := sqlite.NewClient(cfg.SQLite.Path)
	if err != nil {
		appLogger.Fatal("Failed to create SQLite client", zap.Error(err))
	}
	defer sqliteClient.Close()

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

	processor := ingestion.NewProcessor(sqliteClient, zillizClient, llmClient)
	queryEngine := query.NewEngine(sqliteClient, neo4jClient, zillizClient, llmClient)

	app := fiber.New(fiber.Config{
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		BodyLimit:    cfg.Server.BodyLimit,
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	queryHandler := handlers.NewQueryHandler(queryEngine)
	documentHandler := handlers.NewDocumentHandler(processor)

	api := app.Group("/api/v1")

	api.Post("/query", queryHandler.HandleQuery)
	api.Get("/query/history", queryHandler.GetQueryHistory)

	api.Post("/documents", documentHandler.UploadDocument)

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"time":   time.Now().Unix(),
		})
	})

	api.Get("/ready", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ready",
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	appLogger.Info("Server starting", zap.String("address", addr))

	go func() {
		if err := app.Listen(addr); err != nil {
			appLogger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	appLogger.Info("Server shutting down gracefully...")
	app.Shutdown()
	appLogger.Info("Server stopped")
}
