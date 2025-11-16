package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/pkg/logger"
)

type Client struct {
	client *redis.Client
}

func NewClient(host string, port int, password string, db int) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	logger.Info("Redis client initialized", zap.String("addr", fmt.Sprintf("%s:%d", host, port)))

	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) SetQuery(ctx context.Context, queryHash string, response interface{}, ttl time.Duration) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	err = c.client.Set(ctx, fmt.Sprintf("query:%s", queryHash), data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set query cache: %w", err)
	}

	logger.Debug("Query cached", zap.String("query_hash", queryHash), zap.Duration("ttl", ttl))
	return nil
}

func (c *Client) GetQuery(ctx context.Context, queryHash string, response interface{}) (bool, error) {
	data, err := c.client.Get(ctx, fmt.Sprintf("query:%s", queryHash)).Bytes()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get query cache: %w", err)
	}

	err = json.Unmarshal(data, response)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.Debug("Query cache hit", zap.String("query_hash", queryHash))
	return true, nil
}

func (c *Client) SetEmbedding(ctx context.Context, textHash string, embedding []float32, ttl time.Duration) error {
	data, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	err = c.client.Set(ctx, fmt.Sprintf("embedding:%s", textHash), data, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set embedding cache: %w", err)
	}

	logger.Debug("Embedding cached", zap.String("text_hash", textHash))
	return nil
}

func (c *Client) GetEmbedding(ctx context.Context, textHash string) ([]float32, bool, error) {
	data, err := c.client.Get(ctx, fmt.Sprintf("embedding:%s", textHash)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get embedding cache: %w", err)
	}

	var embedding []float32
	err = json.Unmarshal(data, &embedding)
	if err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal embedding: %w", err)
	}

	logger.Debug("Embedding cache hit", zap.String("text_hash", textHash))
	return embedding, true, nil
}

func (c *Client) InvalidateDocumentCache(ctx context.Context) error {
	iter := c.client.Scan(ctx, 0, "query:*", 0).Iterator()
	for iter.Next(ctx) {
		err := c.client.Del(ctx, iter.Val()).Err()
		if err != nil {
			logger.Warn("Failed to delete cache key", zap.Error(err))
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to iterate cache keys: %w", err)
	}

	logger.Info("Document cache invalidated")
	return nil
}

func (c *Client) IncrementMetric(ctx context.Context, metricName string) error {
	return c.client.Incr(ctx, fmt.Sprintf("metric:%s", metricName)).Err()
}

func (c *Client) GetMetric(ctx context.Context, metricName string) (int64, error) {
	val, err := c.client.Get(ctx, fmt.Sprintf("metric:%s", metricName)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}
