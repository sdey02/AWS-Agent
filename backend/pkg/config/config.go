package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Neo4j    Neo4jConfig
	Zilliz   ZillizConfig
	SQLite   SQLiteConfig
	Redis    RedisConfig
	LLM      LLMConfig
	Search   SearchConfig
	Logging  LoggingConfig
}

type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  int
	WriteTimeout int
	BodyLimit    int
}

type Neo4jConfig struct {
	URI      string
	Username string
	Password string
	Database string
}

type ZillizConfig struct {
	Endpoint       string
	APIKey         string
	CollectionName string
	VectorDim      int
	IndexType      string
}

type SQLiteConfig struct {
	Path string
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type LLMConfig struct {
	Provider      string
	Model         string
	APIKey        string
	Temperature   float32
	MaxTokens     int
	TimeoutSec    int
	EmbeddingModel string
	EmbeddingDim   int
}

type SearchConfig struct {
	Enabled        bool
	SerpAPIKey     string
	MaxResults     int
	TimeoutSec     int
}

type LoggingConfig struct {
	Level      string
	Format     string
	OutputPath string
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/aws-agent")

	viper.SetEnvPrefix("AWS_AGENT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func setDefaults() {
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.readTimeout", 30)
	viper.SetDefault("server.writeTimeout", 30)
	viper.SetDefault("server.bodyLimit", 10485760)

	viper.SetDefault("neo4j.uri", "bolt://localhost:7687")
	viper.SetDefault("neo4j.username", "neo4j")
	viper.SetDefault("neo4j.password", "password")
	viper.SetDefault("neo4j.database", "neo4j")

	viper.SetDefault("zilliz.endpoint", "localhost:19530")
	viper.SetDefault("zilliz.collectionName", "aws_docs")
	viper.SetDefault("zilliz.vectorDim", 1536)
	viper.SetDefault("zilliz.indexType", "IVF_FLAT")

	viper.SetDefault("sqlite.path", "./data/awsrag.db")

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("llm.provider", "openai")
	viper.SetDefault("llm.model", "gpt-4")
	viper.SetDefault("llm.temperature", 0.2)
	viper.SetDefault("llm.maxTokens", 2048)
	viper.SetDefault("llm.timeoutSec", 60)
	viper.SetDefault("llm.embeddingModel", "text-embedding-3-large")
	viper.SetDefault("llm.embeddingDim", 1536)

	viper.SetDefault("search.enabled", true)
	viper.SetDefault("search.maxResults", 5)
	viper.SetDefault("search.timeoutSec", 10)

	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.outputPath", "stdout")
}
