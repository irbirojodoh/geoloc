package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// EnsureIndex creates an Elasticsearch index with the given name and mapping file
// if it does not already exist. This operation is idempotent.
func EnsureIndex(ctx context.Context, indexName, mappingFilePath, esURL string) error {
	// Check if index already exists
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf("%s/%s", esURL, indexName), nil)
	if err != nil {
		return fmt.Errorf("failed to create head request for index %s: %w", indexName, err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to check index %s: %w", indexName, err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		slog.Info("elasticsearch index already exists, skipping creation", "index", indexName)
		return nil
	}

	// Read mapping file
	mappingData, err := os.ReadFile(mappingFilePath)
	if err != nil {
		return fmt.Errorf("failed to read mapping file %s: %w", mappingFilePath, err)
	}

	// Validate JSON
	var mappingJSON map[string]interface{}
	if err := json.Unmarshal(mappingData, &mappingJSON); err != nil {
		return fmt.Errorf("invalid mapping JSON in %s: %w", mappingFilePath, err)
	}

	// Create index with mapping
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/%s", esURL, indexName), bytes.NewReader(mappingData))
	if err != nil {
		return fmt.Errorf("failed to create put request for index %s: %w", indexName, err)
	}
	createReq.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(createReq)
	if err != nil {
		return fmt.Errorf("failed to create index %s: %w", indexName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body bytes.Buffer
		body.ReadFrom(resp.Body)
		return fmt.Errorf("failed to create index %s (status %d): %s", indexName, resp.StatusCode, body.String())
	}

	slog.Info("elasticsearch index created successfully", "index", indexName)
	return nil
}

// SetupIndexes ensures all required ES indexes exist. Calls EnsureIndex for each.
func SetupIndexes(ctx context.Context, esURL, postsIndex, usersIndex string) error {
	// Resolve mapping file paths relative to the migrations/elasticsearch directory
	postsMapping := os.Getenv("ES_POSTS_MAPPING_PATH")
	if postsMapping == "" {
		postsMapping = "migrations/elasticsearch/posts_mapping.json"
	}
	usersMapping := os.Getenv("ES_USERS_MAPPING_PATH")
	if usersMapping == "" {
		usersMapping = "migrations/elasticsearch/users_mapping.json"
	}

	if err := EnsureIndex(ctx, postsIndex, postsMapping, esURL); err != nil {
		return fmt.Errorf("failed to setup posts index: %w", err)
	}

	if err := EnsureIndex(ctx, usersIndex, usersMapping, esURL); err != nil {
		return fmt.Errorf("failed to setup users index: %w", err)
	}

	return nil
}
