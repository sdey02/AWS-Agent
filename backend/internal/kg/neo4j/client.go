package neo4j

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/pkg/circuitbreaker"
	"github.com/aws-agent/backend/pkg/logger"
	"github.com/aws-agent/backend/pkg/retry"
)

type Client struct {
	driver      neo4j.DriverWithContext
	cb          *circuitbreaker.CircuitBreaker
	retryConfig retry.Config
}

type Entity struct {
	ID            string
	Name          string
	Type          string
	CanonicalName string
	Properties    map[string]interface{}
}

type Relation struct {
	Subject    string
	Predicate  string
	Object     string
	Confidence float64
	SourceDocs []string
}

type Triple struct {
	Subject    Entity
	Predicate  string
	Object     Entity
	Confidence float64
	SourceURLs []string
}

func NewClient(uri, username, password, database string) (*Client, error) {
	driver, err := neo4j.NewDriverWithContext(
		uri,
		neo4j.BasicAuth(username, password, ""),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	ctx := context.Background()
	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify connectivity: %w", err)
	}

	cb := circuitbreaker.NewCircuitBreaker("neo4j", circuitbreaker.Config{
		MaxRequests:      3,
		Interval:         time.Minute,
		Timeout:          20 * time.Second,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Logger:           logger.GetLogger(),
	})

	retryConfig := retry.Config{
		MaxAttempts:    3,
		InitialDelay:   200 * time.Millisecond,
		MaxDelay:       3 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.1,
		Logger:         logger.GetLogger(),
	}

	logger.Info("Neo4j client initialized", zap.String("uri", uri))

	return &Client{
		driver:      driver,
		cb:          cb,
		retryConfig: retryConfig,
	}, nil
}

func (c *Client) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

func (c *Client) executeWithRetry(ctx context.Context, operation func(neo4j.SessionWithContext) error) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return c.cb.Execute(ctx, func() error {
		return retry.Do(ctx, c.retryConfig, func() error {
			session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
			defer session.Close(ctx)
			return operation(session)
		})
	})
}

func (c *Client) CreateEntity(ctx context.Context, entity *Entity) error {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	query := `
		MERGE (e:Entity {id: $id})
		SET e.name = $name,
		    e.type = $type,
		    e.canonical_name = $canonical_name,
		    e.created_at = timestamp()
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"id":             entity.ID,
		"name":           entity.Name,
		"type":           entity.Type,
		"canonical_name": entity.CanonicalName,
	})

	if err != nil {
		return fmt.Errorf("failed to create entity: %w", err)
	}

	logger.Debug("Entity created in KG", zap.String("entity_id", entity.ID), zap.String("name", entity.Name))

	return nil
}

func (c *Client) CreateRelation(ctx context.Context, relation *Relation) error {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	query := `
		MATCH (s:Entity {id: $subject_id})
		MATCH (o:Entity {id: $object_id})
		MERGE (s)-[r:RELATES {type: $predicate}]->(o)
		SET r.confidence = $confidence,
		    r.source_docs = $source_docs,
		    r.created_at = timestamp()
	`

	_, err := session.Run(ctx, query, map[string]interface{}{
		"subject_id":  relation.Subject,
		"object_id":   relation.Object,
		"predicate":   relation.Predicate,
		"confidence":  relation.Confidence,
		"source_docs": relation.SourceDocs,
	})

	if err != nil {
		return fmt.Errorf("failed to create relation: %w", err)
	}

	logger.Debug("Relation created in KG",
		zap.String("subject", relation.Subject),
		zap.String("predicate", relation.Predicate),
		zap.String("object", relation.Object),
	)

	return nil
}

func (c *Client) SearchByEntities(ctx context.Context, entities []string, minConfidence float64) ([]Triple, error) {
	var triples []Triple

	err := c.executeWithRetry(ctx, func(session neo4j.SessionWithContext) error {
		query := `
			MATCH (s:Entity)-[r:RELATES]->(o:Entity)
			WHERE (s.name IN $entities OR o.name IN $entities)
			  AND r.confidence >= $min_confidence
			RETURN s.id, s.name, s.type, s.canonical_name,
			       r.type, r.confidence, r.source_docs,
			       o.id, o.name, o.type, o.canonical_name
			ORDER BY r.confidence DESC
			LIMIT 20
		`

		result, err := session.Run(ctx, query, map[string]interface{}{
			"entities":       entities,
			"min_confidence": minConfidence,
		})
		if err != nil {
			return fmt.Errorf("failed to search by entities: %w", err)
		}

		for result.Next(ctx) {
			record := result.Record()

			subjectID, _ := record.Get("s.id")
			subjectName, _ := record.Get("s.name")
			subjectType, _ := record.Get("s.type")
			subjectCanonical, _ := record.Get("s.canonical_name")

			objectID, _ := record.Get("o.id")
			objectName, _ := record.Get("o.name")
			objectType, _ := record.Get("o.type")
			objectCanonical, _ := record.Get("o.canonical_name")

			predicate, _ := record.Get("r.type")
			confidence, _ := record.Get("r.confidence")
			sourceDocs, _ := record.Get("r.source_docs")

			var sourceURLs []string
			if docs, ok := sourceDocs.([]interface{}); ok {
				for _, doc := range docs {
					if url, ok := doc.(string); ok {
						sourceURLs = append(sourceURLs, url)
					}
				}
			}

			triple := Triple{
				Subject: Entity{
					ID:            subjectID.(string),
					Name:          subjectName.(string),
					Type:          subjectType.(string),
					CanonicalName: subjectCanonical.(string),
				},
				Predicate: predicate.(string),
				Object: Entity{
					ID:            objectID.(string),
					Name:          objectName.(string),
					Type:          objectType.(string),
					CanonicalName: objectCanonical.(string),
				},
				Confidence: confidence.(float64),
				SourceURLs: sourceURLs,
			}

			triples = append(triples, triple)
		}

		if err = result.Err(); err != nil {
			return fmt.Errorf("error iterating results: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	logger.Info("KG search completed",
		zap.Int("num_entities", len(entities)),
		zap.Int("results_found", len(triples)),
	)

	return triples, nil
}

func (c *Client) FindSolutions(ctx context.Context, errorType string, minConfidence float64) ([]Triple, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	query := `
		MATCH (error:Entity {type: 'error'})-[r1:RELATES {type: 'CAUSED_BY'}]-(cause:Entity)
		MATCH (cause)-[r2:RELATES {type: 'RESOLVED_BY'}]->(solution:Entity)
		WHERE error.name CONTAINS $error_type
		  AND r1.confidence >= $min_confidence
		  AND r2.confidence >= $min_confidence
		RETURN error.id, error.name, error.type, error.canonical_name,
		       'RESOLVED_BY', r2.confidence, r2.source_docs,
		       solution.id, solution.name, solution.type, solution.canonical_name
		ORDER BY r2.confidence DESC
		LIMIT 10
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"error_type":     errorType,
		"min_confidence": minConfidence,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find solutions: %w", err)
	}

	var triples []Triple
	for result.Next(ctx) {
		record := result.Record()

		errorID, _ := record.Get("error.id")
		errorName, _ := record.Get("error.name")
		errorType, _ := record.Get("error.type")
		errorCanonical, _ := record.Get("error.canonical_name")

		solutionID, _ := record.Get("solution.id")
		solutionName, _ := record.Get("solution.name")
		solutionType, _ := record.Get("solution.type")
		solutionCanonical, _ := record.Get("solution.canonical_name")

		confidence, _ := record.Get("r2.confidence")
		sourceDocs, _ := record.Get("r2.source_docs")

		var sourceURLs []string
		if docs, ok := sourceDocs.([]interface{}); ok {
			for _, doc := range docs {
				if url, ok := doc.(string); ok {
					sourceURLs = append(sourceURLs, url)
				}
			}
		}

		triple := Triple{
			Subject: Entity{
				ID:            errorID.(string),
				Name:          errorName.(string),
				Type:          errorType.(string),
				CanonicalName: errorCanonical.(string),
			},
			Predicate: "RESOLVED_BY",
			Object: Entity{
				ID:            solutionID.(string),
				Name:          solutionName.(string),
				Type:          solutionType.(string),
				CanonicalName: solutionCanonical.(string),
			},
			Confidence: confidence.(float64),
			SourceURLs: sourceURLs,
		}

		triples = append(triples, triple)
	}

	if err = result.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return triples, nil
}

func (c *Client) GetEntityByName(ctx context.Context, name string) (*Entity, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	query := `
		MATCH (e:Entity)
		WHERE e.name = $name OR e.canonical_name = $name
		RETURN e.id, e.name, e.type, e.canonical_name
		LIMIT 1
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"name": name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get entity: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		id, _ := record.Get("e.id")
		name, _ := record.Get("e.name")
		entityType, _ := record.Get("e.type")
		canonical, _ := record.Get("e.canonical_name")

		return &Entity{
			ID:            id.(string),
			Name:          name.(string),
			Type:          entityType.(string),
			CanonicalName: canonical.(string),
		}, nil
	}

	return nil, fmt.Errorf("entity not found: %s", name)
}

func (c *Client) GetAllEntities(ctx context.Context) ([]Entity, error) {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer session.Close(ctx)

	query := `
		MATCH (e:Entity)
		RETURN e.id, e.name, e.type, e.canonical_name
		ORDER BY e.name
	`

	result, err := session.Run(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all entities: %w", err)
	}

	var entities []Entity
	for result.Next(ctx) {
		record := result.Record()
		id, _ := record.Get("e.id")
		name, _ := record.Get("e.name")
		entityType, _ := record.Get("e.type")
		canonical, _ := record.Get("e.canonical_name")

		entities = append(entities, Entity{
			ID:            id.(string),
			Name:          name.(string),
			Type:          entityType.(string),
			CanonicalName: canonical.(string),
		})
	}

	if err = result.Err(); err != nil {
		return nil, fmt.Errorf("error iterating results: %w", err)
	}

	return entities, nil
}
