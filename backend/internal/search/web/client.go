package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"

	"github.com/aws-agent/backend/internal/llm"
	"github.com/aws-agent/backend/pkg/logger"
)

type Client struct {
	serpAPIKey string
	llmClient  *llm.Client
	httpClient *http.Client
}

type SearchResult struct {
	Title   string
	URL     string
	Snippet string
	Content string
}

func NewClient(serpAPIKey string, llmClient *llm.Client) *Client {
	return &Client{
		serpAPIKey: serpAPIKey,
		llmClient:  llmClient,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	logger.Info("Performing web search", zap.String("query", query))

	optimizedQuery, err := c.optimizeQuery(ctx, query)
	if err != nil {
		logger.Warn("Failed to optimize query, using original", zap.Error(err))
		optimizedQuery = query
	}

	if c.serpAPIKey != "" {
		return c.searchWithSerpAPI(ctx, optimizedQuery, maxResults)
	}

	return c.searchWithGoogle(ctx, optimizedQuery, maxResults)
}

func (c *Client) optimizeQuery(ctx context.Context, query string) (string, error) {
	systemPrompt := `You are a search query optimizer for AWS technical documentation.
Transform user queries into effective web search queries.

Rules:
1. Add "AWS" prefix
2. Add solution-oriented keywords ("how to fix", "troubleshoot", "resolve")
3. Prefer official AWS sources
4. Add year context (2024 2025) for recent info

Return ONLY the optimized query, nothing else.`

	userPrompt := fmt.Sprintf("Optimize this query for AWS web search: %s", query)

	resp, err := c.llmClient.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.1,
		MaxTokens:    100,
	})

	if err != nil {
		return "", err
	}

	optimized := strings.TrimSpace(resp.Content)
	logger.Debug("Query optimized", zap.String("original", query), zap.String("optimized", optimized))

	return optimized, nil
}

func (c *Client) searchWithSerpAPI(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	baseURL := "https://serpapi.com/search"
	params := url.Values{}
	params.Add("q", query)
	params.Add("api_key", c.serpAPIKey)
	params.Add("num", fmt.Sprintf("%d", maxResults))

	resp, err := c.httpClient.Get(fmt.Sprintf("%s?%s", baseURL, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var searchResp struct {
		OrganicResults []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}

	err = json.Unmarshal(body, &searchResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResp.OrganicResults))
	for _, r := range searchResp.OrganicResults {
		content, err := c.scrapeContent(r.Link)
		if err != nil {
			logger.Warn("Failed to scrape content", zap.String("url", r.Link), zap.Error(err))
			content = r.Snippet
		}

		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.Link,
			Snippet: r.Snippet,
			Content: content,
		})
	}

	logger.Info("Web search completed", zap.Int("results", len(results)))

	return results, nil
}

func (c *Client) searchWithGoogle(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	searchQuery := url.QueryEscape(fmt.Sprintf("site:docs.aws.amazon.com OR site:repost.aws %s", query))
	searchURL := fmt.Sprintf("https://www.google.com/search?q=%s&num=%d", searchQuery, maxResults)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	results := make([]SearchResult, 0)
	doc.Find("div.g").Each(func(i int, s *goquery.Selection) {
		if i >= maxResults {
			return
		}

		title := s.Find("h3").Text()
		link, _ := s.Find("a").Attr("href")
		snippet := s.Find("div.VwiC3b").Text()

		if title != "" && link != "" {
			content, err := c.scrapeContent(link)
			if err != nil {
				content = snippet
			}

			results = append(results, SearchResult{
				Title:   title,
				URL:     link,
				Snippet: snippet,
				Content: content,
			})
		}
	})

	logger.Info("Google search completed", zap.Int("results", len(results)))

	return results, nil
}

func (c *Client) scrapeContent(urlStr string) (string, error) {
	resp, err := c.httpClient.Get(urlStr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	doc.Find("script, style, nav, footer, header").Remove()
	text := doc.Find("body").Text()
	text = strings.TrimSpace(text)

	if len(text) > 5000 {
		text = text[:5000]
	}

	return text, nil
}

func (c *Client) ShouldTriggerWebSearch(kgResultsCount, vectorResultsCount int, confidence float64) bool {
	totalResults := kgResultsCount + vectorResultsCount

	if totalResults < 3 {
		logger.Info("Triggering web search: insufficient results", zap.Int("total", totalResults))
		return true
	}

	if confidence < 0.5 {
		logger.Info("Triggering web search: low confidence", zap.Float64("confidence", confidence))
		return true
	}

	return false
}
