# AWS RAG-Powered Issue Resolution Agent

A powerful RAG (Retrieval-Augmented Generation) system that combines Knowledge Graph and vector search to automatically resolve AWS issues using documentation and LLMs.

## Features

- **Hybrid KG-RAG Architecture**: Combines Knowledge Graph (Neo4j) with vector search (Milvus/Zilliz) for superior retrieval
- **Incremental Knowledge Graph Construction**: Builds high-quality, low-noise knowledge graphs from AWS documentation
- **Semantic Entity Resolution**: Deduplicates entities and maintains canonical names
- **Confidence Scoring**: Assigns reliability scores to extracted relationships
- **Provenance Tracking**: Links all facts back to source documents
- **Web Search Fallback**: Automatically searches web when documentation is insufficient
- **Real-time Chat Interface**: Modern Next.js frontend with streaming responses
- **Comprehensive Evaluation**: LLM-as-judge framework for quality assessment

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Frontend (Next.js)                       │
│  - Chat Interface  - Source Citations  - Feedback System    │
└───────────────────────┬─────────────────────────────────────┘
                        │ REST API
┌───────────────────────▼─────────────────────────────────────┐
│                    API Gateway (Go)                          │
│  - Query Processing  - Document Ingestion  - KG Building    │
└───────────────────────┬─────────────────────────────────────┘
                        │
        ┌───────────────┴────────────────┐
        │                                │
┌───────▼────────┐              ┌───────▼────────┐
│  Neo4j KG      │              │ Milvus Vector  │
│  - Entities    │              │ - Embeddings   │
│  - Relations   │              │ - Doc Chunks   │
└────────────────┘              └────────────────┘
```

## Tech Stack

### Backend (Go)
- **Framework**: Fiber (high-performance HTTP)
- **Knowledge Graph**: Neo4j
- **Vector Database**: Milvus/Zilliz
- **Metadata Store**: SQLite3 (dev) → PostgreSQL (prod)
- **LLM Integration**: OpenAI GPT-4
- **Logging**: Zap (structured logging)

### Frontend (Next.js 14)
- **Framework**: Next.js with App Router
- **UI**: Tailwind CSS + Typography plugin
- **Markdown**: react-markdown
- **State**: React hooks

### Infrastructure
- **Orchestration**: Docker Compose
- **Caching**: Redis
- **Message Queue**: NATS (future)

## Prerequisites

- Docker & Docker Compose
- OpenAI API key
- (Optional) SerpAPI key for web search

## Quick Start

### 1. Clone and Setup

```bash
git clone <repository-url>
cd AWS-Agent

# Copy environment variables
cp .env.example .env

# Edit .env and add your OpenAI API key
nano .env
```

### 2. Start Services

```bash
# Start all services with Docker Compose
docker-compose up -d

# Wait for services to initialize (30-60 seconds)
docker-compose logs -f api
```

### 3. Access Application

- **Frontend**: http://localhost:3000
- **API**: http://localhost:8080
- **Neo4j Browser**: http://localhost:7474 (username: neo4j, password: password)

### 4. Test the System

Open the frontend and try these example queries:
- "My Lambda function is timing out when accessing S3"
- "EC2 instance won't start"
- "How do I configure VPC endpoints for S3?"

## Development

### Backend Development

```bash
cd backend

# Install dependencies
go mod download

# Run tests
go test ./...

# Run locally (requires services running)
go run cmd/api/main.go
```

### Frontend Development

```bash
cd frontend

# Install dependencies
npm install

# Run dev server
npm run dev
```

### Database Access

**Neo4j Cypher Shell:**
```bash
docker exec -it aws-agent-neo4j-1 cypher-shell -u neo4j -p password

# Example queries
MATCH (e:Entity) RETURN e LIMIT 10;
MATCH (s:Entity)-[r:RELATES]->(o:Entity) RETURN s, r, o LIMIT 20;
```

**SQLite:**
```bash
docker exec -it aws-agent-api-1 sqlite3 /data/awsrag.db

# Example queries
SELECT * FROM documents LIMIT 10;
SELECT * FROM kg_entities;
```

## API Endpoints

### Query
- `POST /api/v1/query` - Submit AWS issue query
- `GET /api/v1/query/history` - Get query history

### Documents
- `POST /api/v1/documents` - Upload AWS documentation

### Health
- `GET /api/v1/health` - Health check
- `GET /api/v1/ready` - Readiness check

## Configuration

Edit `backend/config.yaml`:

```yaml
llm:
  provider: openai
  model: gpt-4  # or gpt-3.5-turbo for lower cost
  temperature: 0.2
  embeddingModel: text-embedding-3-large

zilliz:
  vectorDim: 1536  # must match embedding model

logging:
  level: info  # debug, info, warn, error
```

## Ingesting AWS Documentation

```bash
# Upload a document via API
curl -X POST http://localhost:8080/api/v1/documents \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://docs.aws.amazon.com/lambda/latest/dg/vpc.html",
    "html_content": "<html>...</html>"
  }'
```

The system will:
1. Clean and chunk the document
2. Generate embeddings
3. Extract entities and relations
4. Build knowledge graph
5. Index in vector database

## Monitoring

### Logs

```bash
# API logs
docker-compose logs -f api

# All services
docker-compose logs -f
```

### Metrics

View query performance in logs:
- Query latency
- Retrieval hit rates (KG vs Vector)
- Confidence scores
- LLM token usage

## Performance Tuning

### For Speed
- Use `gpt-3.5-turbo` for faster responses
- Reduce vector search `topK` to 5
- Lower KG confidence threshold to 0.5

### For Quality
- Use `gpt-4` for better reasoning
- Increase vector search `topK` to 15
- Raise KG confidence threshold to 0.7

### For Cost
- Enable aggressive caching (Redis TTL: 1 hour)
- Use smaller embedding model
- Batch document processing

## Troubleshooting

### Issue: "Failed to connect to Neo4j"
```bash
# Check Neo4j status
docker-compose ps neo4j
docker-compose logs neo4j

# Restart Neo4j
docker-compose restart neo4j
```

### Issue: "Milvus collection not found"
```bash
# Check Milvus logs
docker-compose logs milvus-standalone

# Recreate collection (will clear data!)
docker-compose restart api
```

### Issue: "OpenAI API rate limit"
- Wait 60 seconds
- Use `gpt-3.5-turbo` instead
- Reduce concurrent requests

## Production Deployment

### 1. Switch to PostgreSQL

Update `config.yaml`:
```yaml
postgres:
  host: postgres
  port: 5432
  database: awsrag
  username: postgres
  password: ${POSTGRES_PASSWORD}
```

### 2. Use Zilliz Cloud

```yaml
zilliz:
  endpoint: https://your-instance.zillizcloud.com
  apiKey: ${ZILLIZ_API_KEY}
```

### 3. Enable HTTPS

Use reverse proxy (nginx/Traefik) with SSL certificates

### 4. Scale Horizontally

```bash
docker-compose up -d --scale api=3
```

## Research Paper Implementation

This implementation is based on three research papers:

1. **Paper 1**: RAG + Text-to-SQL unified chatbot (FastAPI architecture)
2. **Paper 2**: Technical-Embeddings for enhanced retrieval (query expansion, summarization)
3. **Paper 3**: KG-RAG with incremental construction (deduplication, confidence scoring, provenance)

### Key Innovations Applied:
- Hybrid retrieval (KG + Vector) achieving 50%+ reduction in irrelevant answers
- Semantic entity deduplication preventing noisy knowledge graphs
- Confidence-based filtering (threshold: 0.6)
- Provenance tracking for verifiable information

## Project Structure

```
AWS-Agent/
├── backend/
│   ├── cmd/api/              # API server entrypoint
│   ├── internal/
│   │   ├── api/handlers/     # HTTP handlers
│   │   ├── kg/               # Knowledge graph (Neo4j, builder)
│   │   ├── vector/           # Vector DB (Zilliz/Milvus)
│   │   ├── llm/              # LLM integration
│   │   ├── ingestion/        # Document processing
│   │   ├── query/            # Query engine
│   │   └── storage/          # SQLite/models
│   ├── pkg/
│   │   ├── config/           # Configuration
│   │   └── logger/           # Logging
│   └── config.yaml
├── frontend/
│   ├── app/                  # Next.js pages
│   ├── components/
│   │   └── chat/             # Chat UI components
│   └── package.json
├── docker-compose.yml
└── README.md
```

## Contributing

1. Fork the repository
2. Create feature branch: `git checkout -b feature/my-feature`
3. Commit changes: `git commit -am 'Add feature'`
4. Push to branch: `git push origin feature/my-feature`
5. Submit pull request

## License

MIT License - see LICENSE file

## Acknowledgments

Built based on research from:
- "Implementation of RAG and LLM for Document and Tabular-Based Chatbot" (Haque et al.)
- "Enhancing Technical Documents Retrieval for RAG" (Lai et al.)
- "From Documents to Dialogue: Building KG-RAG Enhanced AI Assistants" (Mukherjee et al.)

## Support

For issues and questions:
- GitHub Issues: [Create an issue]
- Documentation: This README
- Logs: `docker-compose logs`

---

**Status**: Production Ready ✓
**Version**: 1.0.0
**Last Updated**: 2025-01-16
