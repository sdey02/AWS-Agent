package ingestion

import (
	"context"
	"crypto/md5"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/internal/storage/models"
	"github.com/aws-agent/backend/internal/storage/sqlite"
	"github.com/aws-agent/backend/internal/vector/zilliz"
	"github.com/aws-agent/backend/pkg/logger"
)

type Processor struct {
	db          *sqlite.Client
	vectorDB    *zilliz.Client
	llmClient   *llm.Client
	chunkSize   int
	chunkOverlap int
}

func NewProcessor(db *sqlite.Client, vectorDB *zilliz.Client, llmClient *llm.Client) *Processor {
	return &Processor{
		db:           db,
		vectorDB:     vectorDB,
		llmClient:    llmClient,
		chunkSize:    1000,
		chunkOverlap: 100,
	}
}

func (p *Processor) ProcessDocument(ctx context.Context, url, htmlContent string) error {
	logger.Info("Processing document", zap.String("url", url))

	cleanedText := p.cleanHTML(htmlContent)
	if cleanedText == "" {
		return fmt.Errorf("no content extracted from HTML")
	}

	awsService := p.extractAWSService(url)
	docType := p.extractDocType(url)

	summary, err := p.llmClient.SummarizeDocument(ctx, cleanedText[:min(len(cleanedText), 4000)])
	if err != nil {
		logger.Warn("Failed to summarize document", zap.Error(err))
		summary = "Summary unavailable"
	}

	docID := generateID(url)
	doc := &models.Document{
		ID:         docID,
		URL:        url,
		Title:      p.extractTitle(htmlContent),
		AWSService: awsService,
		DocType:    docType,
		Summary:    summary,
		RawContent: cleanedText,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = p.db.InsertDocument(doc)
	if err != nil {
		return fmt.Errorf("failed to insert document: %w", err)
	}

	chunks := p.chunkText(cleanedText)
	logger.Info("Document chunked", zap.Int("chunks", len(chunks)))

	embeddings, err := p.llmClient.GenerateBatchEmbeddings(ctx, chunks)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(embeddings) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, expected %d", len(embeddings), len(chunks))
	}

	vectorChunks := make([]zilliz.DocumentChunk, 0, len(chunks))
	for i, chunkText := range chunks {
		chunkID := fmt.Sprintf("%s_chunk_%d", docID, i)
		vectorChunk := zilliz.DocumentChunk{
			ID:         chunkID,
			Embedding:  embeddings[i],
			Text:       chunkText,
			DocURL:     url,
			AWSService: awsService,
			DocType:    docType,
			Summary:    summary,
			Timestamp:  time.Now(),
		}
		vectorChunks = append(vectorChunks, vectorChunk)

		dbChunk := &models.DocumentChunk{
			ID:          chunkID,
			DocID:       docID,
			ChunkIndex:  i,
			Text:        chunkText,
			EmbeddingID: chunkID,
			CreatedAt:   time.Now(),
		}
		p.db.InsertChunk(dbChunk)
	}

	if len(vectorChunks) > 0 {
		err = p.vectorDB.Insert(ctx, vectorChunks)
		if err != nil {
			return fmt.Errorf("failed to insert into vector DB: %w", err)
		}
	}

	logger.Info("Document processed successfully",
		zap.String("doc_id", docID),
		zap.Int("chunks", len(vectorChunks)),
	)

	return nil
}

func (p *Processor) cleanHTML(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	doc.Find("script, style, nav, footer, header, aside").Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	text := doc.Find("body").Text()

	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text
}

func (p *Processor) extractTitle(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "Untitled"
	}

	title := doc.Find("title").First().Text()
	if title == "" {
		title = doc.Find("h1").First().Text()
	}

	if title == "" {
		title = "Untitled"
	}

	return strings.TrimSpace(title)
}

func (p *Processor) extractAWSService(url string) string {
	serviceMap := map[string]string{
		"ec2":      "EC2",
		"s3":       "S3",
		"lambda":   "Lambda",
		"rds":      "RDS",
		"dynamodb": "DynamoDB",
		"vpc":      "VPC",
		"iam":      "IAM",
		"cloudwatch": "CloudWatch",
		"eks":      "EKS",
		"ecs":      "ECS",
	}

	lowerURL := strings.ToLower(url)
	for key, service := range serviceMap {
		if strings.Contains(lowerURL, key) {
			return service
		}
	}

	return "General"
}

func (p *Processor) extractDocType(url string) string {
	lowerURL := strings.ToLower(url)

	if strings.Contains(lowerURL, "troubleshoot") {
		return "troubleshooting"
	}
	if strings.Contains(lowerURL, "guide") {
		return "guide"
	}
	if strings.Contains(lowerURL, "reference") {
		return "reference"
	}
	if strings.Contains(lowerURL, "tutorial") {
		return "tutorial"
	}

	return "documentation"
}

func (p *Processor) chunkText(text string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []string
	var currentChunk strings.Builder
	currentSize := 0

	for _, word := range words {
		wordLen := len(word) + 1

		if currentSize+wordLen > p.chunkSize && currentChunk.Len() > 0 {
			chunks = append(chunks, currentChunk.String())

			overlapWords := strings.Fields(currentChunk.String())
			overlapStart := max(0, len(overlapWords)-p.chunkOverlap/10)
			currentChunk.Reset()
			currentChunk.WriteString(strings.Join(overlapWords[overlapStart:], " ") + " ")
			currentSize = currentChunk.Len()
		}

		currentChunk.WriteString(word + " ")
		currentSize += wordLen
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	return chunks
}

func generateID(input string) string {
	hash := md5.Sum([]byte(input))
	return fmt.Sprintf("%x", hash)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
