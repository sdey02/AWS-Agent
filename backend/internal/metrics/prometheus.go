package metrics

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	QueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aws_rag_query_duration_seconds",
			Help:    "Query processing duration in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"query_type"},
	)

	QueryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_rag_query_total",
			Help: "Total number of queries processed",
		},
		[]string{"status"},
	)

	RetrievalHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_rag_retrieval_hit_rate",
			Help: "Percentage of queries with results",
		},
		[]string{"source"},
	)

	LLMTokensUsed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_rag_llm_tokens_used",
			Help: "Total LLM tokens used",
		},
		[]string{"model", "type"},
	)

	LLMCost = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_rag_llm_cost_usd",
			Help: "Estimated LLM API cost in USD",
		},
		[]string{"model"},
	)

	UserSatisfaction = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "aws_rag_satisfaction_score",
			Help: "User feedback satisfaction score",
		},
		[]string{"helpful"},
	)

	ConfidenceScore = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "aws_rag_confidence_score",
			Help:    "Response confidence scores",
			Buckets: []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
		},
		[]string{},
	)

	KGResultsCount = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "aws_rag_kg_results_count",
			Help:    "Number of KG results per query",
			Buckets: []float64{0, 1, 2, 5, 10, 20, 50},
		},
	)

	VectorResultsCount = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "aws_rag_vector_results_count",
			Help:    "Number of vector results per query",
			Buckets: []float64{0, 1, 2, 5, 10, 20, 50},
		},
	)

	WebSearchTriggered = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aws_rag_web_search_triggered_total",
			Help: "Total number of web searches triggered",
		},
	)

	CacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_rag_cache_hits_total",
			Help: "Total cache hits",
		},
		[]string{"cache_type"},
	)

	CacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_rag_cache_misses_total",
			Help: "Total cache misses",
		},
		[]string{"cache_type"},
	)

	DocumentsProcessed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aws_rag_documents_processed_total",
			Help: "Total documents processed",
		},
	)

	KGEntitiesTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aws_rag_kg_entities_total",
			Help: "Total entities in knowledge graph",
		},
	)

	KGRelationsTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aws_rag_kg_relations_total",
			Help: "Total relations in knowledge graph",
		},
	)

	AWSActionsExecuted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aws_rag_aws_actions_executed_total",
			Help: "Total AWS actions executed",
		},
		[]string{"service", "action", "status"},
	)
)

func Init() {
	prometheus.MustRegister(QueryDuration)
	prometheus.MustRegister(QueryTotal)
	prometheus.MustRegister(RetrievalHitRate)
	prometheus.MustRegister(LLMTokensUsed)
	prometheus.MustRegister(LLMCost)
	prometheus.MustRegister(UserSatisfaction)
	prometheus.MustRegister(ConfidenceScore)
	prometheus.MustRegister(KGResultsCount)
	prometheus.MustRegister(VectorResultsCount)
	prometheus.MustRegister(WebSearchTriggered)
	prometheus.MustRegister(CacheHits)
	prometheus.MustRegister(CacheMisses)
	prometheus.MustRegister(DocumentsProcessed)
	prometheus.MustRegister(KGEntitiesTotal)
	prometheus.MustRegister(KGRelationsTotal)
	prometheus.MustRegister(AWSActionsExecuted)
}

func MetricsHandler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.Handler())
}
