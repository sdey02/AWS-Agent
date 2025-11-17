package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/pkg/circuitbreaker"
	"github.com/aws-agent/backend/pkg/logger"
	"github.com/aws-agent/backend/pkg/retry"
)

type Client struct {
	client         *openai.Client
	model          string
	embeddingModel string
	temperature    float32
	maxTokens      int
	cb             *circuitbreaker.CircuitBreaker
	retryConfig    retry.Config
}

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float32
	MaxTokens    int
}

type CompletionResponse struct {
	Content string
	Usage   Usage
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func NewClient(apiKey, model, embeddingModel string, temperature float32, maxTokens int) *Client {
	client := openai.NewClient(apiKey)

	cb := circuitbreaker.NewCircuitBreaker("llm", circuitbreaker.Config{
		MaxRequests:      5,
		Interval:         time.Minute,
		Timeout:          30 * time.Second,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Logger:           logger.GetLogger(),
	})

	retryConfig := retry.Config{
		MaxAttempts:    3,
		InitialDelay:   500 * time.Millisecond,
		MaxDelay:       5 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.1,
		Logger:         logger.GetLogger(),
	}

	logger.Info("LLM client initialized",
		zap.String("model", model),
		zap.String("embedding_model", embeddingModel),
	)

	return &Client{
		client:         client,
		model:          model,
		embeddingModel: embeddingModel,
		temperature:    temperature,
		maxTokens:      maxTokens,
		cb:             cb,
		retryConfig:    retryConfig,
	}
}

func (c *Client) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	temperature := req.Temperature
	if temperature == 0 {
		temperature = c.temperature
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.maxTokens
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: req.SystemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: req.UserPrompt,
		},
	}

	var result *CompletionResponse

	err := c.cb.Execute(ctx, func() error {
		return retry.Do(ctx, c.retryConfig, func() error {
			resp, err := c.client.CreateChatCompletion(
				ctx,
				openai.ChatCompletionRequest{
					Model:       c.model,
					Messages:    messages,
					Temperature: temperature,
					MaxTokens:   maxTokens,
				},
			)

			if err != nil {
				return fmt.Errorf("failed to create completion: %w", err)
			}

			logger.Debug("LLM completion generated",
				zap.Int("prompt_tokens", resp.Usage.PromptTokens),
				zap.Int("completion_tokens", resp.Usage.CompletionTokens),
			)

			result = &CompletionResponse{
				Content: resp.Choices[0].Message.Content,
				Usage: Usage{
					PromptTokens:     resp.Usage.PromptTokens,
					CompletionTokens: resp.Usage.CompletionTokens,
					TotalTokens:      resp.Usage.TotalTokens,
				},
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var embedding []float32

	err := c.cb.Execute(ctx, func() error {
		return retry.Do(ctx, c.retryConfig, func() error {
			resp, err := c.client.CreateEmbeddings(
				ctx,
				openai.EmbeddingRequest{
					Input: []string{text},
					Model: openai.EmbeddingModel(c.embeddingModel),
				},
			)

			if err != nil {
				return fmt.Errorf("failed to generate embedding: %w", err)
			}

			embedding = make([]float32, len(resp.Data[0].Embedding))
			for i, v := range resp.Data[0].Embedding {
				embedding[i] = v
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return embedding, nil
}

func (c *Client) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var embeddings [][]float32

	batchSize := 100
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]

		err := c.cb.Execute(ctx, func() error {
			return retry.Do(ctx, c.retryConfig, func() error {
				resp, err := c.client.CreateEmbeddings(
					ctx,
					openai.EmbeddingRequest{
						Input: batch,
						Model: openai.EmbeddingModel(c.embeddingModel),
					},
				)

				if err != nil {
					return fmt.Errorf("failed to generate batch embeddings: %w", err)
				}

				for _, data := range resp.Data {
					embedding := make([]float32, len(data.Embedding))
					for j, v := range data.Embedding {
						embedding[j] = v
					}
					embeddings = append(embeddings, embedding)
				}

				return nil
			})
		})

		if err != nil {
			return nil, err
		}
	}

	logger.Debug("Batch embeddings generated", zap.Int("count", len(embeddings)))

	return embeddings, nil
}

func (c *Client) SummarizeDocument(ctx context.Context, content string) (string, error) {
	systemPrompt := `You are an AWS documentation expert. Generate a concise 2-3 sentence summary of the given AWS documentation.
Focus on:
- Primary AWS service(s)
- Main purpose/use case
- Key operations or commands
- Common issues addressed

Be specific and technical.`

	userPrompt := fmt.Sprintf("Summarize this AWS documentation:\n\n%s", content)

	resp, err := c.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.3,
		MaxTokens:    300,
	})

	if err != nil {
		return "", fmt.Errorf("failed to summarize: %w", err)
	}

	logger.Info("Document summarized", zap.Int("summary_length", len(resp.Content)))

	return resp.Content, nil
}

func (c *Client) ExtractEntities(ctx context.Context, documentSummary string, seedConcepts []string) ([]EntityExtraction, error) {
	systemPrompt := `You are an AWS knowledge graph expert. Extract entities from AWS documentation.

Entity types:
- service: AWS services (EC2, S3, Lambda, etc.)
- error: Error codes or types (InvalidParameter, AccessDenied, etc.)
- resource: AWS resources (Instance, Bucket, Function, etc.)
- operation: AWS operations (CreateInstance, PutObject, etc.)
- concept: Technical concepts (VPC, IAM, encryption, etc.)

Return ONLY new entities not in the known list. Format as JSON array:
[{"name": "entity_name", "type": "entity_type", "confidence": 0.9}]`

	knownEntities := strings.Join(seedConcepts, ", ")
	userPrompt := fmt.Sprintf(`Known entities: %s

Extract NEW entities from this AWS documentation summary:
%s

Return JSON only.`, knownEntities, documentSummary)

	resp, err := c.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
		MaxTokens:    500,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to extract entities: %w", err)
	}

	entities := parseEntityExtractions(resp.Content)

	logger.Info("Entities extracted", zap.Int("count", len(entities)))

	return entities, nil
}

func (c *Client) ExtractRelations(ctx context.Context, documentText string, entities []string) ([]RelationExtraction, error) {
	systemPrompt := `You are an AWS knowledge graph expert. Extract relationships between AWS entities.

Relationship types:
- USES: service uses another service
- REQUIRES: service requires resource/config
- INTEGRATES_WITH: services integrate
- MONITORS: service monitors another
- LOGS_TO: service logs to another
- CAUSED_BY: error caused by condition
- RESOLVED_BY: error resolved by action
- HAS_ERROR: operation has error
- PART_OF: resource part of service

Return JSON array:
[{"subject": "entity1", "predicate": "USES", "object": "entity2", "confidence": 0.85}]`

	entityList := strings.Join(entities, ", ")
	userPrompt := fmt.Sprintf(`Entities: %s

Extract relationships from this AWS documentation:
%s

Return JSON only.`, entityList, documentText)

	resp, err := c.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
		MaxTokens:    800,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to extract relations: %w", err)
	}

	relations := parseRelationExtractions(resp.Content)

	logger.Info("Relations extracted", zap.Int("count", len(relations)))

	return relations, nil
}

func (c *Client) GenerateResponse(ctx context.Context, query string, kgContext, vectorContext string) (string, error) {
	systemPrompt := `You are an AWS Solutions Architect AI assistant specialized in troubleshooting and resolving AWS service issues.

Your responses must:
1. Be technically accurate and based ONLY on provided context
2. Cite sources using [source_id] notation
3. Provide step-by-step solutions when applicable
4. Acknowledge limitations when context is insufficient
5. Suggest web search when documentation doesn't cover the issue

Be concise, technical, and actionable.`

	userPrompt := fmt.Sprintf(`Issue: %s

Knowledge Graph Facts:
%s

Documentation:
%s

Provide a solution that:
1. Explains the root cause
2. Lists specific steps to resolve
3. Includes relevant AWS CLI/Console commands if applicable
4. Cites sources for verification

If information is insufficient, explain what additional details are needed.`, query, kgContext, vectorContext)

	resp, err := c.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
		MaxTokens:    2048,
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	logger.Info("Response generated",
		zap.String("query", query),
		zap.Int("response_length", len(resp.Content)),
	)

	return resp.Content, nil
}

func (c *Client) EvaluateResponse(ctx context.Context, query, response, groundTruth string) (*EvaluationScore, error) {
	systemPrompt := `You are an AI evaluation expert. Rate the quality of AWS troubleshooting responses.

Rate on scale 1-3:
1. Relevance: Does it address the issue?
2. Accuracy: Is information correct?
3. Completeness: Are steps actionable?
4. Citation quality: Proper sources?

Return JSON:
{"relevance": 3, "accuracy": 3, "completeness": 2, "citations": 3, "classification": "fully_relevant", "reasoning": "explanation"}`

	userPrompt := fmt.Sprintf(`Query: %s

Response: %s

Ground Truth: %s

Evaluate the response.`, query, response, groundTruth)

	resp, err := c.Complete(ctx, CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
		MaxTokens:    400,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to evaluate response: %w", err)
	}

	score := parseEvaluationScore(resp.Content)

	return score, nil
}

type EntityExtraction struct {
	Name       string
	Type       string
	Confidence float64
}

type RelationExtraction struct {
	Subject    string
	Predicate  string
	Object     string
	Confidence float64
}

type EvaluationScore struct {
	Relevance      float64
	Accuracy       float64
	Completeness   float64
	Citations      float64
	Classification string
	Reasoning      string
}

func parseEntityExtractions(content string) []EntityExtraction {
	var entities []EntityExtraction
	return entities
}

func parseRelationExtractions(content string) []RelationExtraction {
	var relations []RelationExtraction
	return relations
}

func parseEvaluationScore(content string) *EvaluationScore {
	return &EvaluationScore{
		Relevance:      2.5,
		Accuracy:       2.5,
		Completeness:   2.5,
		Citations:      2.5,
		Classification: "moderate",
		Reasoning:      "Default scoring",
	}
}
