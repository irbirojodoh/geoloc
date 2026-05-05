package search
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// ESClient is a lightweight HTTP client for Elasticsearch.
// We use raw HTTP instead of the official Go client to keep the dependency
// footprint minimal and the API surface explicit.
type ESClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewESClient creates a new ESClient from environment configuration.
func NewESClient() *ESClient {
	baseURL := os.Getenv("ELASTICSEARCH_URL")
	if baseURL == "" {
		baseURL = "http://localhost:9200"
	}

	return &ESClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

// BaseURL returns the configured Elasticsearch base URL.
func (c *ESClient) BaseURL() string {
	return c.baseURL
}

// Search executes a raw search query against the given index and returns the response body.
func (c *ESClient) Search(ctx context.Context, index string, query interface{}) ([]byte, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search query: %w", err)
	}

	url := fmt.Sprintf("%s/%s/_search", c.baseURL, index)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("es search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read es response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("es search returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// IndexDocument indexes a document into Elasticsearch. Uses the id as the document _id
// for idempotent indexing (same id = same document, no duplicates on replay).
func (c *ESClient) IndexDocument(ctx context.Context, index, id string, document interface{}) error {
	body, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	url := fmt.Sprintf("%s/%s/_doc/%s", c.baseURL, index, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create index request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("es index request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("es index returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// BulkIndex performs a bulk indexing operation. The documents parameter is a slice
// of BulkDocument, each containing an ID and the document body.
func (c *ESClient) BulkIndex(ctx context.Context, index string, documents []BulkDocument) error {
	if len(documents) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range documents {
		// action line
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": index,
				"_id":    doc.ID,
			},
		}
		actionBytes, _ := json.Marshal(action)
		buf.Write(actionBytes)
		buf.WriteByte('\n')

		// document line
		docBytes, err := json.Marshal(doc.Document)
		if err != nil {
			return fmt.Errorf("failed to marshal bulk document %s: %w", doc.ID, err)
		}
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	url := fmt.Sprintf("%s/_bulk", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create bulk request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("es bulk request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("es bulk returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Aggregate runs an aggregation query against an index and returns the parsed response.
func (c *ESClient) Aggregate(ctx context.Context, index string, query interface{}) ([]byte, error) {
	return c.Search(ctx, index, query)
}

// HealthCheck checks if Elasticsearch is reachable.
func (c *ESClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("es health check returned status %d", resp.StatusCode)
	}

	return nil
}

// BulkDocument represents a single document in a bulk indexing operation.
type BulkDocument struct {
	ID       string
	Document interface{}
}

// ESResponse is a minimal Elasticsearch search response parser.
type ESResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		MaxScore float64 `json:"max_score"`
		Hits     []struct {
			Index  string                 `json:"_index"`
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
}

// ParseESResponse parses a raw ES search response.
func ParseESResponse(data []byte) (*ESResponse, error) {
	var resp ESResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse es response: %w", err)
	}
	return &resp, nil
}

// Ensure indexes are created — called at startup.
func EnsureESIndexes(ctx context.Context, esURL, postsIndex, usersIndex string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	indexes := []struct {
		name string
		path string
	}{
		{postsIndex, os.Getenv("ES_POSTS_MAPPING_PATH")},
		{usersIndex, os.Getenv("ES_USERS_MAPPING_PATH")},
	}

	for _, idx := range indexes {
		// Skip if mapping file is empty and not set
		mappingPath := idx.path
		if mappingPath == "" {
			switch idx.name {
			case postsIndex:
				mappingPath = "migrations/elasticsearch/posts_mapping.json"
			case usersIndex:
				mappingPath = "migrations/elasticsearch/users_mapping.json"
			}
		}

		// Check existence
		headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf("%s/%s", esURL, idx.name), nil)
		if err != nil {
			return fmt.Errorf("failed to check index %s: %w", idx.name, err)
		}
		headResp, err := client.Do(headReq)
		if err != nil {
			slog.Warn("cannot check ES index (ES may be unavailable)", "index", idx.name, "error", err)
			continue
		}
		headResp.Body.Close()

		if headResp.StatusCode == http.StatusOK {
			slog.Info("ES index already exists", "index", idx.name)
			continue
		}

		// Read mapping
		mappingData, err := os.ReadFile(mappingPath)
		if err != nil {
			return fmt.Errorf("failed to read mapping %s: %w", mappingPath, err)
		}

		putReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
			fmt.Sprintf("%s/%s", esURL, idx.name), bytes.NewReader(mappingData))
		if err != nil {
			return fmt.Errorf("failed to create index request: %w", err)
		}
		putReq.Header.Set("Content-Type", "application/json")

		putResp, err := client.Do(putReq)
		if err != nil {
			return fmt.Errorf("failed to create index %s: %w", idx.name, err)
		}
		putResp.Body.Close()

		if putResp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to create index %s (status %d)", idx.name, putResp.StatusCode)
		}

		slog.Info("ES index created", "index", idx.name)
	}

	return nil
}
