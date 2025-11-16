# Quick Start Guide

Get up and running with AWS RAG Agent in 5 minutes!

## Prerequisites

- Docker & Docker Compose installed
- OpenAI API key

## Step-by-Step Setup

### 1. Initialize Project

```bash
# Copy environment template
make init

# Or manually:
cp .env.example .env
```

### 2. Configure API Key

Edit `.env` file:
```bash
# Open in your editor
nano .env

# Add your OpenAI API key
OPENAI_API_KEY=sk-your-key-here
```

### 3. Start Services

```bash
# Start all services (this may take 1-2 minutes first time)
make up

# Or:
docker-compose up -d
```

### 4. Wait for Services

```bash
# Check logs until you see "Server starting"
make logs

# Or:
docker-compose logs -f api
```

### 5. Open Application

Go to: **http://localhost:3000**

## First Query

Try asking:
- "My Lambda function is timing out when accessing S3"
- "How do I configure VPC endpoints?"
- "EC2 instance won't start, what should I check?"

## What's Happening?

When you submit a query:

1. **Query Analysis**: Extracts AWS services and error types
2. **KG Retrieval**: Searches Knowledge Graph for structured facts
3. **Vector Search**: Finds relevant documentation chunks
4. **Hybrid Fusion**: Combines KG and vector results
5. **LLM Generation**: GPT-4 generates response with citations
6. **Response**: You see answer with source links and confidence score

## Loading Documentation

To add AWS documentation:

```bash
curl -X POST http://localhost:8080/api/v1/documents \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://docs.aws.amazon.com/lambda/latest/dg/vpc.html",
    "html_content": "'"$(curl -s https://docs.aws.amazon.com/lambda/latest/dg/vpc.html)"'"
  }'
```

The system will automatically:
- Extract entities (Lambda, VPC, S3, etc.)
- Build knowledge graph relationships
- Generate embeddings
- Index in vector database

## Troubleshooting

### Services won't start

```bash
# Check Docker is running
docker ps

# Check ports are available
lsof -i :3000
lsof -i :8080

# Restart services
make down
make up
```

### "Connection refused" errors

Wait 30-60 seconds for all services to initialize. Check logs:

```bash
docker-compose logs neo4j
docker-compose logs milvus-standalone
docker-compose logs api
```

### OpenAI rate limits

- Wait 60 seconds between queries
- Use `gpt-3.5-turbo` in config for higher rate limits

## Next Steps

1. **Read Full Documentation**: See [README.md](README.md)
2. **Load More Docs**: Ingest AWS documentation
3. **Customize**: Edit `backend/config.yaml`
4. **Monitor**: Check logs with `make logs`

## Useful Commands

```bash
# View all commands
make help

# Stop services
make down

# View logs
make logs

# Clean everything (removes data!)
make clean

# Rebuild images
make build
```

## Success Criteria

You'll know it's working when:
- ✓ Frontend loads at localhost:3000
- ✓ API responds at localhost:8080/api/v1/health
- ✓ Queries return responses with sources
- ✓ Confidence scores shown (0-1)
- ✓ Neo4j browser shows entities (localhost:7474)

## Support

Having issues? Check:
1. Docker containers running: `docker ps`
2. Logs for errors: `make logs`
3. README troubleshooting section
4. OpenAI API key is valid

---

**Ready to build?** Start querying AWS issues and watch the AI resolve them! 🚀
