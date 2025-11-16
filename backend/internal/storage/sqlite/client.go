package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/storage/models"
	"github.com/aws-agent/backend/pkg/logger"
)

type Client struct {
	db *sql.DB
}

func NewClient(dbPath string) (*Client, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	_, err = db.Exec("PRAGMA journal_mode = WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	logger.Info("SQLite client initialized", zap.String("path", dbPath))

	return &Client{db: db}, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}

func (c *Client) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS documents (
		id TEXT PRIMARY KEY,
		url TEXT UNIQUE NOT NULL,
		title TEXT NOT NULL,
		aws_service TEXT,
		doc_type TEXT,
		summary TEXT,
		raw_content TEXT,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		last_scraped INTEGER
	);
	CREATE INDEX IF NOT EXISTS idx_documents_service ON documents(aws_service);
	CREATE INDEX IF NOT EXISTS idx_documents_type ON documents(doc_type);
	CREATE INDEX IF NOT EXISTS idx_documents_updated ON documents(updated_at);

	CREATE TABLE IF NOT EXISTS document_chunks (
		id TEXT PRIMARY KEY,
		doc_id TEXT NOT NULL,
		chunk_index INTEGER NOT NULL,
		text TEXT NOT NULL,
		embedding_id TEXT,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (doc_id) REFERENCES documents(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_chunks_doc ON document_chunks(doc_id);

	CREATE TABLE IF NOT EXISTS query_history (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		query_text TEXT NOT NULL,
		response TEXT,
		confidence REAL,
		kg_results_count INTEGER,
		vector_results_count INTEGER,
		web_search_used INTEGER DEFAULT 0,
		latency_ms INTEGER,
		created_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_query_user ON query_history(user_id);
	CREATE INDEX IF NOT EXISTS idx_query_created ON query_history(created_at);

	CREATE TABLE IF NOT EXISTS query_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query_id TEXT NOT NULL,
		source_type TEXT NOT NULL,
		source_url TEXT,
		chunk_id TEXT,
		confidence REAL,
		FOREIGN KEY (query_id) REFERENCES query_history(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_sources_query ON query_sources(query_id);

	CREATE TABLE IF NOT EXISTS feedback (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query_id TEXT NOT NULL,
		helpful INTEGER NOT NULL,
		issue_category TEXT,
		comment TEXT,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (query_id) REFERENCES query_history(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_feedback_query ON feedback(query_id);
	CREATE INDEX IF NOT EXISTS idx_feedback_created ON feedback(created_at);

	CREATE TABLE IF NOT EXISTS evaluation_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query_id TEXT NOT NULL,
		relevance_score REAL,
		accuracy_score REAL,
		completeness_score REAL,
		citation_score REAL,
		overall_classification TEXT,
		reasoning TEXT,
		cosine_similarity REAL,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (query_id) REFERENCES query_history(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_eval_query ON evaluation_results(query_id);

	CREATE TABLE IF NOT EXISTS kg_entities (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		canonical_name TEXT,
		aliases TEXT,
		first_seen INTEGER NOT NULL,
		last_updated INTEGER NOT NULL,
		occurrence_count INTEGER DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS idx_entities_type ON kg_entities(type);
	CREATE INDEX IF NOT EXISTS idx_entities_name ON kg_entities(name);

	CREATE TABLE IF NOT EXISTS kg_relations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject_id TEXT NOT NULL,
		predicate TEXT NOT NULL,
		object_id TEXT NOT NULL,
		confidence REAL NOT NULL,
		source_doc_id TEXT,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (subject_id) REFERENCES kg_entities(id),
		FOREIGN KEY (object_id) REFERENCES kg_entities(id),
		FOREIGN KEY (source_doc_id) REFERENCES documents(id)
	);
	CREATE INDEX IF NOT EXISTS idx_relations_subject ON kg_relations(subject_id);
	CREATE INDEX IF NOT EXISTS idx_relations_object ON kg_relations(object_id);
	CREATE INDEX IF NOT EXISTS idx_relations_confidence ON kg_relations(confidence);

	CREATE TABLE IF NOT EXISTS seed_concepts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		description TEXT,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS system_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		metric_name TEXT NOT NULL,
		metric_value REAL NOT NULL,
		tags TEXT,
		timestamp INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_name ON system_metrics(metric_name);
	CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON system_metrics(timestamp);
	`

	_, err := c.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	logger.Info("SQLite schema initialized")
	return nil
}

func (c *Client) InsertDocument(doc *models.Document) error {
	query := `
		INSERT INTO documents (id, url, title, aws_service, doc_type, summary, raw_content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			summary = excluded.summary,
			raw_content = excluded.raw_content,
			updated_at = excluded.updated_at
	`

	_, err := c.db.Exec(
		query,
		doc.ID,
		doc.URL,
		doc.Title,
		doc.AWSService,
		doc.DocType,
		doc.Summary,
		doc.RawContent,
		doc.CreatedAt.Unix(),
		doc.UpdatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert document: %w", err)
	}

	logger.Debug("Document inserted", zap.String("doc_id", doc.ID), zap.String("url", doc.URL))
	return nil
}

func (c *Client) GetDocument(id string) (*models.Document, error) {
	query := `SELECT id, url, title, aws_service, doc_type, summary, raw_content, created_at, updated_at FROM documents WHERE id = ?`

	var doc models.Document
	var createdAt, updatedAt int64

	err := c.db.QueryRow(query, id).Scan(
		&doc.ID,
		&doc.URL,
		&doc.Title,
		&doc.AWSService,
		&doc.DocType,
		&doc.Summary,
		&doc.RawContent,
		&createdAt,
		&updatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	doc.CreatedAt = time.Unix(createdAt, 0)
	doc.UpdatedAt = time.Unix(updatedAt, 0)

	return &doc, nil
}

func (c *Client) InsertChunk(chunk *models.DocumentChunk) error {
	query := `INSERT INTO document_chunks (id, doc_id, chunk_index, text, embedding_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`

	_, err := c.db.Exec(
		query,
		chunk.ID,
		chunk.DocID,
		chunk.ChunkIndex,
		chunk.Text,
		chunk.EmbeddingID,
		chunk.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert chunk: %w", err)
	}

	return nil
}

func (c *Client) InsertQueryRecord(record *models.QueryRecord) error {
	query := `
		INSERT INTO query_history (id, user_id, query_text, response, confidence, kg_results_count,
			vector_results_count, web_search_used, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	webSearchUsed := 0
	if record.WebSearchUsed {
		webSearchUsed = 1
	}

	_, err := c.db.Exec(
		query,
		record.ID,
		record.UserID,
		record.QueryText,
		record.Response,
		record.Confidence,
		record.KGResultsCount,
		record.VectorResultsCount,
		webSearchUsed,
		record.LatencyMS,
		record.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert query record: %w", err)
	}

	logger.Info("Query recorded",
		zap.String("query_id", record.ID),
		zap.String("query", record.QueryText),
		zap.Float64("confidence", record.Confidence),
	)

	return nil
}

func (c *Client) InsertQuerySource(source *models.QuerySource) error {
	query := `INSERT INTO query_sources (query_id, source_type, source_url, chunk_id, confidence) VALUES (?, ?, ?, ?, ?)`

	_, err := c.db.Exec(
		query,
		source.QueryID,
		source.SourceType,
		source.SourceURL,
		source.ChunkID,
		source.Confidence,
	)

	if err != nil {
		return fmt.Errorf("failed to insert query source: %w", err)
	}

	return nil
}

func (c *Client) GetQueryHistory(userID string, limit int) ([]models.QueryRecord, error) {
	query := `
		SELECT id, query_text, response, confidence, created_at
		FROM query_history
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := c.db.Query(query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get query history: %w", err)
	}
	defer rows.Close()

	var records []models.QueryRecord
	for rows.Next() {
		var r models.QueryRecord
		var createdAt int64

		err := rows.Scan(&r.ID, &r.QueryText, &r.Response, &r.Confidence, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		r.CreatedAt = time.Unix(createdAt, 0)
		records = append(records, r)
	}

	return records, nil
}

func (c *Client) StoreFeedback(feedback *models.Feedback) error {
	query := `INSERT INTO feedback (query_id, helpful, issue_category, comment, created_at) VALUES (?, ?, ?, ?, ?)`

	helpful := 0
	if feedback.Helpful {
		helpful = 1
	}

	_, err := c.db.Exec(
		query,
		feedback.QueryID,
		helpful,
		feedback.IssueCategory,
		feedback.Comment,
		time.Now().Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to store feedback: %w", err)
	}

	logger.Info("Feedback stored",
		zap.String("query_id", feedback.QueryID),
		zap.Bool("helpful", feedback.Helpful),
	)

	return nil
}

func (c *Client) InsertKGEntity(entity *models.KGEntity) error {
	aliasesJSON, _ := json.Marshal(entity.Aliases)

	query := `
		INSERT INTO kg_entities (id, name, type, canonical_name, aliases, first_seen, last_updated, occurrence_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			occurrence_count = occurrence_count + 1,
			last_updated = excluded.last_updated
	`

	_, err := c.db.Exec(
		query,
		entity.ID,
		entity.Name,
		entity.Type,
		entity.CanonicalName,
		string(aliasesJSON),
		entity.FirstSeen.Unix(),
		entity.LastUpdated.Unix(),
		entity.OccurrenceCount,
	)

	if err != nil {
		return fmt.Errorf("failed to insert KG entity: %w", err)
	}

	return nil
}

func (c *Client) GetKGEntities(entityType string) ([]models.KGEntity, error) {
	query := `SELECT id, name, type, canonical_name, aliases FROM kg_entities WHERE type = ?`

	rows, err := c.db.Query(query, entityType)
	if err != nil {
		return nil, fmt.Errorf("failed to get KG entities: %w", err)
	}
	defer rows.Close()

	var entities []models.KGEntity
	for rows.Next() {
		var e models.KGEntity
		var aliasesJSON string

		err := rows.Scan(&e.ID, &e.Name, &e.Type, &e.CanonicalName, &aliasesJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		json.Unmarshal([]byte(aliasesJSON), &e.Aliases)
		entities = append(entities, e)
	}

	return entities, nil
}

func (c *Client) GetAllKGEntityNames() ([]string, error) {
	query := `SELECT name FROM kg_entities ORDER BY occurrence_count DESC`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		names = append(names, name)
	}

	return names, nil
}

func (c *Client) InsertKGRelation(relation *models.KGRelation) error {
	query := `
		INSERT INTO kg_relations (subject_id, predicate, object_id, confidence, source_doc_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := c.db.Exec(
		query,
		relation.SubjectID,
		relation.Predicate,
		relation.ObjectID,
		relation.Confidence,
		relation.SourceDocID,
		relation.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert KG relation: %w", err)
	}

	return nil
}

func (c *Client) InsertSeedConcept(concept *models.SeedConcept) error {
	query := `INSERT OR IGNORE INTO seed_concepts (id, name, type, description, created_at) VALUES (?, ?, ?, ?, ?)`

	_, err := c.db.Exec(
		query,
		concept.ID,
		concept.Name,
		concept.Type,
		concept.Description,
		concept.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to insert seed concept: %w", err)
	}

	return nil
}

func (c *Client) GetSeedConcepts() ([]models.SeedConcept, error) {
	query := `SELECT id, name, type, description FROM seed_concepts`

	rows, err := c.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get seed concepts: %w", err)
	}
	defer rows.Close()

	var concepts []models.SeedConcept
	for rows.Next() {
		var c models.SeedConcept
		err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Description)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		concepts = append(concepts, c)
	}

	return concepts, nil
}

func (c *Client) RecordMetric(name string, value float64, tags map[string]string) error {
	tagsJSON, _ := json.Marshal(tags)

	query := `INSERT INTO system_metrics (metric_name, metric_value, tags, timestamp) VALUES (?, ?, ?, ?)`

	_, err := c.db.Exec(query, name, value, string(tagsJSON), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("failed to record metric: %w", err)
	}

	return nil
}
