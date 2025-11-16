package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/kg/neo4j"
	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/internal/storage/models"
	"github.com/aws-agent/backend/internal/storage/sqlite"
	"github.com/aws-agent/backend/internal/vector/zilliz"
	"github.com/aws-agent/backend/pkg/logger"
)

type Engine struct {
	db        *sqlite.Client
	kgClient  *neo4j.Client
	vectorDB  *zilliz.Client
	llmClient *llm.Client
}

type QueryRequest struct {
	Query  string
	UserID string
}

type QueryResponse struct {
	ID         string
	Query      string
	Response   string
	Sources    []Source
	Confidence float64
	LatencyMS  int
}

type Source struct {
	Type       string
	URL        string
	ChunkID    string
	Confidence float64
}

func NewEngine(db *sqlite.Client, kgClient *neo4j.Client, vectorDB *zilliz.Client, llmClient *llm.Client) *Engine {
	return &Engine{
		db:        db,
		kgClient:  kgClient,
		vectorDB:  vectorDB,
		llmClient: llmClient,
	}
}

func (e *Engine) ProcessQuery(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	startTime := time.Now()
	queryID := uuid.New().String()

	logger.Info("Processing query",
		zap.String("query_id", queryID),
		zap.String("query", req.Query),
	)

	entities := e.extractEntitiesFromQuery(req.Query)
	logger.Debug("Extracted entities from query", zap.Strings("entities", entities))

	kgResults, err := e.retrieveFromKG(ctx, entities)
	if err != nil {
		logger.Warn("KG retrieval failed", zap.Error(err))
	}

	vectorResults, err := e.retrieveFromVector(ctx, req.Query, entities)
	if err != nil {
		logger.Warn("Vector retrieval failed", zap.Error(err))
	}

	fusedResults := e.fuseResults(kgResults, vectorResults)
	logger.Info("Results fused",
		zap.Int("kg_results", len(kgResults)),
		zap.Int("vector_results", len(vectorResults)),
		zap.Int("fused_results", len(fusedResults)),
	)

	kgContext := e.formatKGContext(kgResults)
	vectorContext := e.formatVectorContext(vectorResults)

	response, err := e.llmClient.GenerateResponse(ctx, req.Query, kgContext, vectorContext)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	confidence := e.calculateConfidence(kgResults, vectorResults, response)

	sources := make([]Source, 0)
	for _, result := range kgResults {
		for _, url := range result.SourceURLs {
			sources = append(sources, Source{
				Type:       "kg",
				URL:        url,
				Confidence: result.Confidence,
			})
		}
	}
	for _, result := range vectorResults {
		sources = append(sources, Source{
			Type:       "vector",
			URL:        result.DocURL,
			ChunkID:    result.ChunkID,
			Confidence: float64(result.Score),
		})
	}

	latency := int(time.Since(startTime).Milliseconds())

	record := &models.QueryRecord{
		ID:                 queryID,
		UserID:             req.UserID,
		QueryText:          req.Query,
		Response:           response,
		Confidence:         confidence,
		KGResultsCount:     len(kgResults),
		VectorResultsCount: len(vectorResults),
		WebSearchUsed:      false,
		LatencyMS:          latency,
		CreatedAt:          time.Now(),
	}

	e.db.InsertQueryRecord(record)

	for _, source := range sources {
		e.db.InsertQuerySource(&models.QuerySource{
			QueryID:    queryID,
			SourceType: source.Type,
			SourceURL:  source.URL,
			ChunkID:    source.ChunkID,
			Confidence: source.Confidence,
		})
	}

	logger.Info("Query processed successfully",
		zap.String("query_id", queryID),
		zap.Float64("confidence", confidence),
		zap.Int("latency_ms", latency),
	)

	return &QueryResponse{
		ID:         queryID,
		Query:      req.Query,
		Response:   response,
		Sources:    sources,
		Confidence: confidence,
		LatencyMS:  latency,
	}, nil
}

func (e *Engine) extractEntitiesFromQuery(query string) []string {
	entities := []string{}

	serviceKeywords := map[string]string{
		"lambda":    "Lambda",
		"s3":        "S3",
		"ec2":       "EC2",
		"rds":       "RDS",
		"dynamodb":  "DynamoDB",
		"vpc":       "VPC",
		"iam":       "IAM",
		"cloudwatch": "CloudWatch",
	}

	lowerQuery := strings.ToLower(query)
	for keyword, service := range serviceKeywords {
		if strings.Contains(lowerQuery, keyword) {
			entities = append(entities, service)
		}
	}

	if strings.Contains(lowerQuery, "timeout") {
		entities = append(entities, "timeout")
	}
	if strings.Contains(lowerQuery, "permission") || strings.Contains(lowerQuery, "access denied") {
		entities = append(entities, "AccessDenied")
	}

	return entities
}

func (e *Engine) retrieveFromKG(ctx context.Context, entities []string) ([]neo4j.Triple, error) {
	if len(entities) == 0 {
		return nil, nil
	}

	triples, err := e.kgClient.SearchByEntities(ctx, entities, 0.6)
	if err != nil {
		return nil, err
	}

	return triples, nil
}

func (e *Engine) retrieveFromVector(ctx context.Context, query string, entities []string) ([]zilliz.SearchResult, error) {
	embedding, err := e.llmClient.GenerateEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}

	filters := make(map[string]string)
	if len(entities) > 0 {
		for _, entity := range entities {
			if isAWSService(entity) {
				filters["aws_service"] = entity
				break
			}
		}
	}

	results, err := e.vectorDB.Search(ctx, embedding, 10, filters)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func (e *Engine) fuseResults(kgResults []neo4j.Triple, vectorResults []zilliz.SearchResult) []interface{} {
	var fused []interface{}

	for _, kg := range kgResults {
		fused = append(fused, kg)
	}

	for _, vec := range vectorResults {
		fused = append(fused, vec)
	}

	return fused
}

func (e *Engine) formatKGContext(triples []neo4j.Triple) string {
	if len(triples) == 0 {
		return "No structured knowledge available."
	}

	var builder strings.Builder
	builder.WriteString("Structured Knowledge:\n")

	for i, triple := range triples {
		if i >= 5 {
			break
		}
		builder.WriteString(fmt.Sprintf("- %s %s %s (confidence: %.2f)\n",
			triple.Subject.Name,
			triple.Predicate,
			triple.Object.Name,
			triple.Confidence,
		))
	}

	return builder.String()
}

func (e *Engine) formatVectorContext(results []zilliz.SearchResult) string {
	if len(results) == 0 {
		return "No documentation found."
	}

	var builder strings.Builder
	builder.WriteString("\nRelevant Documentation:\n")

	for i, result := range results {
		if i >= 5 {
			break
		}
		builder.WriteString(fmt.Sprintf("\n[Source %d]: %s\n%s\nURL: %s\n",
			i+1,
			result.Summary,
			result.Text[:min(len(result.Text), 500)],
			result.DocURL,
		))
	}

	return builder.String()
}

func (e *Engine) calculateConfidence(kgResults []neo4j.Triple, vectorResults []zilliz.SearchResult, response string) float64 {
	if len(kgResults) == 0 && len(vectorResults) == 0 {
		return 0.3
	}

	confidence := 0.5

	if len(kgResults) > 0 {
		var avgKGConfidence float64
		for _, triple := range kgResults {
			avgKGConfidence += triple.Confidence
		}
		avgKGConfidence /= float64(len(kgResults))
		confidence += avgKGConfidence * 0.3
	}

	if len(vectorResults) > 0 {
		confidence += 0.2
	}

	if strings.Contains(response, "http") {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func isAWSService(entity string) bool {
	services := []string{"Lambda", "S3", "EC2", "RDS", "DynamoDB", "VPC", "IAM", "CloudWatch"}
	for _, service := range services {
		if entity == service {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
