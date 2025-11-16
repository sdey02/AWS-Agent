package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/internal/storage/models"
	"github.com/aws-agent/backend/internal/storage/sqlite"
	"github.com/aws-agent/backend/pkg/logger"
)

type Evaluator struct {
	db        *sqlite.Client
	llmClient *llm.Client
}

type EvaluationDataset struct {
	Items []DatasetItem
}

type DatasetItem struct {
	Query       string
	GroundTruth string
	Category    string
}

type EvaluationReport struct {
	TotalQueries          int
	IrrelevantCount       int
	ModerateCount         int
	FullyRelevantCount    int
	AvgRelevanceScore     float64
	AvgAccuracyScore      float64
	AvgCompletenessScore  float64
	AvgCitationScore      float64
	AvgCosineSimilarity   float64
	IrrelevantPercentage  float64
	ModeratePercentage    float64
	FullyRelevantPercentage float64
}

func NewEvaluator(db *sqlite.Client, llmClient *llm.Client) *Evaluator {
	return &Evaluator{
		db:        db,
		llmClient: llmClient,
	}
}

func (e *Evaluator) EvaluateQuery(ctx context.Context, queryID, query, response, groundTruth string) (*models.EvaluationResult, error) {
	logger.Info("Evaluating query", zap.String("query_id", queryID))

	score, err := e.llmClient.EvaluateResponse(ctx, query, response, groundTruth)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM evaluation: %w", err)
	}

	cosineSim := 0.0
	if groundTruth != "" {
		cosineSim, err = e.calculateCosineSimilarity(ctx, response, groundTruth)
		if err != nil {
			logger.Warn("Failed to calculate cosine similarity", zap.Error(err))
		}
	}

	result := &models.EvaluationResult{
		QueryID:               queryID,
		RelevanceScore:        score.Relevance,
		AccuracyScore:         score.Accuracy,
		CompletenessScore:     score.Completeness,
		CitationScore:         score.Citations,
		OverallClassification: score.Classification,
		Reasoning:             score.Reasoning,
		CosineSimilarity:      cosineSim,
	}

	logger.Info("Query evaluated",
		zap.String("query_id", queryID),
		zap.String("classification", score.Classification),
		zap.Float64("relevance", score.Relevance),
	)

	return result, nil
}

func (e *Evaluator) RunDatasetEvaluation(ctx context.Context, dataset *EvaluationDataset) (*EvaluationReport, error) {
	logger.Info("Running dataset evaluation", zap.Int("items", len(dataset.Items)))

	report := &EvaluationReport{
		TotalQueries: len(dataset.Items),
	}

	var totalRelevance, totalAccuracy, totalCompleteness, totalCitation, totalCosineSim float64

	for i, item := range dataset.Items {
		logger.Info("Evaluating item", zap.Int("index", i+1), zap.Int("total", len(dataset.Items)))

		queryID := fmt.Sprintf("eval_%d", i)

		result, err := e.EvaluateQuery(ctx, queryID, item.Query, item.GroundTruth, item.GroundTruth)
		if err != nil {
			logger.Error("Failed to evaluate query", zap.Error(err))
			continue
		}

		switch result.OverallClassification {
		case "irrelevant":
			report.IrrelevantCount++
		case "moderate":
			report.ModerateCount++
		case "fully_relevant":
			report.FullyRelevantCount++
		}

		totalRelevance += result.RelevanceScore
		totalAccuracy += result.AccuracyScore
		totalCompleteness += result.CompletenessScore
		totalCitation += result.CitationScore
		totalCosineSim += result.CosineSimilarity
	}

	if report.TotalQueries > 0 {
		report.AvgRelevanceScore = totalRelevance / float64(report.TotalQueries)
		report.AvgAccuracyScore = totalAccuracy / float64(report.TotalQueries)
		report.AvgCompletenessScore = totalCompleteness / float64(report.TotalQueries)
		report.AvgCitationScore = totalCitation / float64(report.TotalQueries)
		report.AvgCosineSimilarity = totalCosineSim / float64(report.TotalQueries)

		report.IrrelevantPercentage = float64(report.IrrelevantCount) / float64(report.TotalQueries) * 100
		report.ModeratePercentage = float64(report.ModerateCount) / float64(report.TotalQueries) * 100
		report.FullyRelevantPercentage = float64(report.FullyRelevantCount) / float64(report.TotalQueries) * 100
	}

	logger.Info("Dataset evaluation completed",
		zap.Int("total", report.TotalQueries),
		zap.Int("irrelevant", report.IrrelevantCount),
		zap.Int("moderate", report.ModerateCount),
		zap.Int("fully_relevant", report.FullyRelevantCount),
	)

	return report, nil
}

func (e *Evaluator) calculateCosineSimilarity(ctx context.Context, text1, text2 string) (float64, error) {
	emb1, err := e.llmClient.GenerateEmbedding(ctx, text1)
	if err != nil {
		return 0, err
	}

	emb2, err := e.llmClient.GenerateEmbedding(ctx, text2)
	if err != nil {
		return 0, err
	}

	return cosineSimilarity(emb1, emb2), nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (e *Evaluator) LoadDatasetFromJSON(jsonData string) (*EvaluationDataset, error) {
	var dataset EvaluationDataset
	err := json.Unmarshal([]byte(jsonData), &dataset)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal dataset: %w", err)
	}

	return &dataset, nil
}

func (e *Evaluator) GenerateReport(report *EvaluationReport) string {
	return fmt.Sprintf(`
Evaluation Report
=================

Total Queries: %d

Classifications:
- Irrelevant: %d (%.1f%%)
- Moderately Relevant: %d (%.1f%%)
- Fully Relevant: %d (%.1f%%)

Average Scores:
- Relevance: %.2f / 3.0
- Accuracy: %.2f / 3.0
- Completeness: %.2f / 3.0
- Citations: %.2f / 3.0

Cosine Similarity: %.3f

Improvement vs Baseline:
- Irrelevant Reduction: %.1f%% target (actual: %.1f%%)
- Fully Relevant Increase: %.1f%% target (actual: %.1f%%)
`,
		report.TotalQueries,
		report.IrrelevantCount, report.IrrelevantPercentage,
		report.ModerateCount, report.ModeratePercentage,
		report.FullyRelevantCount, report.FullyRelevantPercentage,
		report.AvgRelevanceScore,
		report.AvgAccuracyScore,
		report.AvgCompletenessScore,
		report.AvgCitationScore,
		report.AvgCosineSimilarity,
		50.0, report.IrrelevantPercentage,
		80.0, report.FullyRelevantPercentage,
	)
}
