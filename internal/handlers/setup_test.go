package handlers

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/testcontainers/testcontainers-go/modules/cassandra"
)

// This variable will be available to all tests in the handlers package
var testSession *gocql.Session

func TestMain(m *testing.M) {
	// 1. Start Cassandra Container
	ctx := context.Background()

	// Note: Adjust the path to 'migrations' relative to internal/handlers
	// Since handlers is 2 levels deep (internal/handlers), we go up 2 levels
	schemaPath := filepath.Join("..", "..", "migrations", "cassandra_schema.cql")

	cassandraContainer, err := cassandra.Run(
		ctx,
		"cassandra:4.1",
		cassandra.WithInitScripts(schemaPath),
	)
	if err != nil {
		log.Fatalf("failed to start container: %s", err)
	}

	// 2. Connect to Database
	host, _ := cassandraContainer.ConnectionHost(ctx)
	cluster := gocql.NewCluster(host)
	cluster.Keyspace = "geoloc"
	cluster.Consistency = gocql.Quorum
	cluster.Timeout = 10 * time.Second
	// Use a simple strategy for single-node test cluster
	cluster.ProtoVersion = 4

	// Retry connection loop
	for i := 0; i < 15; i++ {
		testSession, err = cluster.CreateSession()
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("failed to connect to cassandra: %s", err)
	}

	// 3. Run Tests
	exitCode := m.Run()

	// 4. Cleanup
	testSession.Close()
	if err := cassandraContainer.Terminate(ctx); err != nil {
		log.Printf("failed to terminate container: %s", err)
	}

	os.Exit(exitCode)
}
