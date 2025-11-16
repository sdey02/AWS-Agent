package zilliz

import (
	"context"
	"fmt"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/pkg/logger"
)

type Client struct {
	client         client.Client
	collectionName string
	vectorDim      int
}

type DocumentChunk struct {
	ID         string
	Embedding  []float32
	Text       string
	DocURL     string
	AWSService string
	DocType    string
	Summary    string
	Timestamp  time.Time
}

type SearchResult struct {
	ChunkID    string
	Text       string
	DocURL     string
	AWSService string
	DocType    string
	Summary    string
	Score      float32
}

func NewClient(endpoint, apiKey, collectionName string, vectorDim int) (*Client, error) {
	c, err := client.NewGrpcClient(
		context.Background(),
		endpoint,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create milvus client: %w", err)
	}

	logger.Info("Zilliz/Milvus client initialized",
		zap.String("endpoint", endpoint),
		zap.String("collection", collectionName),
	)

	return &Client{
		client:         c,
		collectionName: collectionName,
		vectorDim:      vectorDim,
	}, nil
}

func (z *Client) Close() error {
	return z.client.Close()
}

func (z *Client) CreateCollection(ctx context.Context) error {
	has, err := z.client.HasCollection(ctx, z.collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection: %w", err)
	}

	if has {
		logger.Info("Collection already exists", zap.String("collection", z.collectionName))
		return nil
	}

	schema := &entity.Schema{
		CollectionName: z.collectionName,
		Description:    "AWS documentation embeddings",
		Fields: []*entity.Field{
			{
				Name:       "chunk_id",
				DataType:   entity.FieldTypeVarChar,
				PrimaryKey: true,
				AutoID:     false,
				TypeParams: map[string]string{
					"max_length": "64",
				},
			},
			{
				Name:     "embedding",
				DataType: entity.FieldTypeFloatVector,
				TypeParams: map[string]string{
					"dim": fmt.Sprintf("%d", z.vectorDim),
				},
			},
			{
				Name:     "text",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "4096",
				},
			},
			{
				Name:     "doc_url",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "512",
				},
			},
			{
				Name:     "aws_service",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "128",
				},
			},
			{
				Name:     "doc_type",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "64",
				},
			},
			{
				Name:     "summary",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					"max_length": "1024",
				},
			},
			{
				Name:     "timestamp",
				DataType: entity.FieldTypeInt64,
			},
		},
	}

	err = z.client.CreateCollection(ctx, schema, entity.DefaultShardNumber)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	idx := entity.NewIndexIVFFlat(entity.L2, 1024)
	err = z.client.CreateIndex(ctx, z.collectionName, "embedding", idx, false)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	err = z.client.LoadCollection(ctx, z.collectionName, false)
	if err != nil {
		return fmt.Errorf("failed to load collection: %w", err)
	}

	logger.Info("Collection created and loaded", zap.String("collection", z.collectionName))

	return nil
}

func (z *Client) Insert(ctx context.Context, chunks []DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	chunkIDs := make([]string, len(chunks))
	embeddings := make([][]float32, len(chunks))
	texts := make([]string, len(chunks))
	docURLs := make([]string, len(chunks))
	services := make([]string, len(chunks))
	docTypes := make([]string, len(chunks))
	summaries := make([]string, len(chunks))
	timestamps := make([]int64, len(chunks))

	for i, chunk := range chunks {
		chunkIDs[i] = chunk.ID
		embeddings[i] = chunk.Embedding
		texts[i] = chunk.Text
		docURLs[i] = chunk.DocURL
		services[i] = chunk.AWSService
		docTypes[i] = chunk.DocType
		summaries[i] = chunk.Summary
		timestamps[i] = chunk.Timestamp.Unix()
	}

	_, err := z.client.Insert(
		ctx,
		z.collectionName,
		"",
		entity.NewColumnVarChar("chunk_id", chunkIDs),
		entity.NewColumnFloatVector("embedding", z.vectorDim, embeddings),
		entity.NewColumnVarChar("text", texts),
		entity.NewColumnVarChar("doc_url", docURLs),
		entity.NewColumnVarChar("aws_service", services),
		entity.NewColumnVarChar("doc_type", docTypes),
		entity.NewColumnVarChar("summary", summaries),
		entity.NewColumnInt64("timestamp", timestamps),
	)

	if err != nil {
		return fmt.Errorf("failed to insert chunks: %w", err)
	}

	err = z.client.Flush(ctx, z.collectionName, false)
	if err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	logger.Info("Chunks inserted into vector DB", zap.Int("count", len(chunks)))

	return nil
}

func (z *Client) Search(ctx context.Context, queryEmbedding []float32, topK int, filters map[string]string) ([]SearchResult, error) {
	expr := ""
	if service, ok := filters["aws_service"]; ok && service != "" {
		expr = fmt.Sprintf(`aws_service == "%s"`, service)
	}
	if docType, ok := filters["doc_type"]; ok && docType != "" {
		if expr != "" {
			expr += " && "
		}
		expr += fmt.Sprintf(`doc_type == "%s"`, docType)
	}

	sp, _ := entity.NewIndexIVFFlatSearchParam(16)

	searchResult, err := z.client.Search(
		ctx,
		z.collectionName,
		[]string{},
		expr,
		[]string{"chunk_id", "text", "doc_url", "aws_service", "doc_type", "summary"},
		[]entity.Vector{entity.FloatVector(queryEmbedding)},
		"embedding",
		entity.L2,
		topK,
		sp,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	results := make([]SearchResult, 0)
	for _, sr := range searchResult {
		for i := 0; i < sr.ResultCount; i++ {
			chunkIDCol := sr.Fields.GetColumn("chunk_id")
			textCol := sr.Fields.GetColumn("text")
			docURLCol := sr.Fields.GetColumn("doc_url")
			serviceCol := sr.Fields.GetColumn("aws_service")
			docTypeCol := sr.Fields.GetColumn("doc_type")
			summaryCol := sr.Fields.GetColumn("summary")

			chunkID, _ := chunkIDCol.Get(i)
			text, _ := textCol.Get(i)
			docURL, _ := docURLCol.Get(i)
			service, _ := serviceCol.Get(i)
			docType, _ := docTypeCol.Get(i)
			summary, _ := summaryCol.Get(i)

			results = append(results, SearchResult{
				ChunkID:    chunkID.(string),
				Text:       text.(string),
				DocURL:     docURL.(string),
				AWSService: service.(string),
				DocType:    docType.(string),
				Summary:    summary.(string),
				Score:      sr.Scores[i],
			})
		}
	}

	logger.Info("Vector search completed",
		zap.Int("topK", topK),
		zap.Int("results", len(results)),
		zap.String("filters", expr),
	)

	return results, nil
}
