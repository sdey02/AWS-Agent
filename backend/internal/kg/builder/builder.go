package builder

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/kg/neo4j"
	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/internal/storage/models"
	"github.com/aws-agent/backend/internal/storage/sqlite"
	"github.com/aws-agent/backend/pkg/logger"
)

type Builder struct {
	db        *sqlite.Client
	kgClient  *neo4j.Client
	llmClient *llm.Client
}

func NewBuilder(db *sqlite.Client, kgClient *neo4j.Client, llmClient *llm.Client) *Builder {
	return &Builder{
		db:        db,
		kgClient:  kgClient,
		llmClient: llmClient,
	}
}

func (b *Builder) BuildFromDocument(ctx context.Context, doc *models.Document) error {
	logger.Info("Building KG from document", zap.String("doc_id", doc.ID))

	seedConcepts, err := b.db.GetSeedConcepts()
	if err != nil {
		logger.Warn("Failed to get seed concepts", zap.Error(err))
		seedConcepts = []models.SeedConcept{}
	}

	knownEntities, err := b.db.GetAllKGEntityNames()
	if err != nil {
		logger.Warn("Failed to get known entities", zap.Error(err))
		knownEntities = []string{}
	}

	for _, concept := range seedConcepts {
		knownEntities = append(knownEntities, concept.Name)
	}

	newEntities, err := b.llmClient.ExtractEntities(ctx, doc.Summary, knownEntities)
	if err != nil {
		return fmt.Errorf("failed to extract entities: %w", err)
	}

	logger.Info("Entities extracted", zap.Int("count", len(newEntities)))

	uniqueEntities := b.deduplicateEntities(newEntities, knownEntities)

	for _, entityExt := range uniqueEntities {
		entityID := uuid.New().String()
		entity := &models.KGEntity{
			ID:              entityID,
			Name:            entityExt.Name,
			Type:            entityExt.Type,
			CanonicalName:   entityExt.Name,
			Aliases:         []string{},
			FirstSeen:       time.Now(),
			LastUpdated:     time.Now(),
			OccurrenceCount: 1,
		}

		err = b.db.InsertKGEntity(entity)
		if err != nil {
			logger.Error("Failed to insert entity to SQLite", zap.Error(err))
			continue
		}

		kgEntity := &neo4j.Entity{
			ID:            entityID,
			Name:          entity.Name,
			Type:          entity.Type,
			CanonicalName: entity.CanonicalName,
		}
		err = b.kgClient.CreateEntity(ctx, kgEntity)
		if err != nil {
			logger.Error("Failed to create entity in Neo4j", zap.Error(err))
		}
	}

	allEntityNames := append(knownEntities, extractNames(uniqueEntities)...)
	relations, err := b.llmClient.ExtractRelations(ctx, doc.RawContent[:min(len(doc.RawContent), 5000)], allEntityNames)
	if err != nil {
		return fmt.Errorf("failed to extract relations: %w", err)
	}

	logger.Info("Relations extracted", zap.Int("count", len(relations)))

	for _, rel := range relations {
		if rel.Confidence < 0.6 {
			continue
		}

		subjectEntity, err := b.kgClient.GetEntityByName(ctx, rel.Subject)
		if err != nil {
			logger.Debug("Subject entity not found", zap.String("subject", rel.Subject))
			continue
		}

		objectEntity, err := b.kgClient.GetEntityByName(ctx, rel.Object)
		if err != nil {
			logger.Debug("Object entity not found", zap.String("object", rel.Object))
			continue
		}

		relation := &neo4j.Relation{
			Subject:    subjectEntity.ID,
			Predicate:  rel.Predicate,
			Object:     objectEntity.ID,
			Confidence: rel.Confidence,
			SourceDocs: []string{doc.URL},
		}

		err = b.kgClient.CreateRelation(ctx, relation)
		if err != nil {
			logger.Error("Failed to create relation in Neo4j", zap.Error(err))
			continue
		}

		dbRelation := &models.KGRelation{
			SubjectID:   subjectEntity.ID,
			Predicate:   rel.Predicate,
			ObjectID:    objectEntity.ID,
			Confidence:  rel.Confidence,
			SourceDocID: doc.ID,
			CreatedAt:   time.Now(),
		}
		b.db.InsertKGRelation(dbRelation)
	}

	logger.Info("KG built from document",
		zap.String("doc_id", doc.ID),
		zap.Int("new_entities", len(uniqueEntities)),
		zap.Int("new_relations", len(relations)),
	)

	return nil
}

func (b *Builder) InitializeSeedConcepts() error {
	seeds := []models.SeedConcept{
		{ID: uuid.New().String(), Name: "Lambda", Type: "service", Description: "AWS Lambda serverless compute", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "S3", Type: "service", Description: "AWS S3 object storage", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "EC2", Type: "service", Description: "AWS EC2 virtual servers", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "RDS", Type: "service", Description: "AWS RDS relational database", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "DynamoDB", Type: "service", Description: "AWS DynamoDB NoSQL database", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "VPC", Type: "service", Description: "AWS VPC virtual private cloud", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "IAM", Type: "service", Description: "AWS IAM identity and access management", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "CloudWatch", Type: "service", Description: "AWS CloudWatch monitoring", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "timeout", Type: "error", Description: "Execution timeout error", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "AccessDenied", Type: "error", Description: "Access denied error", CreatedAt: time.Now()},
		{ID: uuid.New().String(), Name: "InvalidParameter", Type: "error", Description: "Invalid parameter error", CreatedAt: time.Now()},
	}

	for _, seed := range seeds {
		err := b.db.InsertSeedConcept(&seed)
		if err != nil {
			logger.Error("Failed to insert seed concept", zap.Error(err))
		}
	}

	logger.Info("Seed concepts initialized", zap.Int("count", len(seeds)))
	return nil
}

func (b *Builder) deduplicateEntities(newEntities []llm.EntityExtraction, knownNames []string) []llm.EntityExtraction {
	unique := []llm.EntityExtraction{}
	knownSet := make(map[string]bool)

	for _, name := range knownNames {
		knownSet[name] = true
	}

	for _, entity := range newEntities {
		if !knownSet[entity.Name] {
			unique = append(unique, entity)
			knownSet[entity.Name] = true
		}
	}

	return unique
}

func extractNames(entities []llm.EntityExtraction) []string {
	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = e.Name
	}
	return names
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
