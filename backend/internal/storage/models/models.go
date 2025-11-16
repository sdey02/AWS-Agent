package models

import "time"

type Document struct {
	ID           string
	URL          string
	Title        string
	AWSService   string
	DocType      string
	Summary      string
	RawContent   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastScraped  *time.Time
}

type DocumentChunk struct {
	ID          string
	DocID       string
	ChunkIndex  int
	Text        string
	EmbeddingID string
	CreatedAt   time.Time
}

type QueryRecord struct {
	ID                  string
	UserID              string
	QueryText           string
	Response            string
	Confidence          float64
	KGResultsCount      int
	VectorResultsCount  int
	WebSearchUsed       bool
	LatencyMS           int
	CreatedAt           time.Time
}

type QuerySource struct {
	ID         int
	QueryID    string
	SourceType string
	SourceURL  string
	ChunkID    string
	Confidence float64
}

type Feedback struct {
	ID            int
	QueryID       string
	Helpful       bool
	IssueCategory string
	Comment       string
	CreatedAt     time.Time
}

type EvaluationResult struct {
	ID                     int
	QueryID                string
	RelevanceScore         float64
	AccuracyScore          float64
	CompletenessScore      float64
	CitationScore          float64
	OverallClassification  string
	Reasoning              string
	CosineSimilarity       float64
	CreatedAt              time.Time
}

type KGEntity struct {
	ID              string
	Name            string
	Type            string
	CanonicalName   string
	Aliases         []string
	FirstSeen       time.Time
	LastUpdated     time.Time
	OccurrenceCount int
}

type KGRelation struct {
	ID          int
	SubjectID   string
	Predicate   string
	ObjectID    string
	Confidence  float64
	SourceDocID string
	CreatedAt   time.Time
}

type SeedConcept struct {
	ID          string
	Name        string
	Type        string
	Description string
	CreatedAt   time.Time
}

type SystemMetric struct {
	ID          int
	MetricName  string
	MetricValue float64
	Tags        string
	Timestamp   time.Time
}
