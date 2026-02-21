package data

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

// Global session for all tests to share
var testSession *gocql.Session

func TestMain(m *testing.M) {
	// 1. Setup (Start Container)
	ctx := context.Background()

	// We check for existing session to prevent double-start if running in IDE
	cassandraContainer, err := cassandra.Run(
		ctx,
		"cassandra:4.1",
		cassandra.WithInitScripts(
			filepath.Join("..", "..", "migrations", "cassandra_schema.cql"),
		),
	)
	if err != nil {
		log.Fatalf("failed to start container: %s", err)
	}

	// 2. Connect
	host, _ := cassandraContainer.ConnectionHost(ctx)

	cluster := gocql.NewCluster(host)
	cluster.Keyspace = "geoloc"     // Direct to our keyspace
	cluster.Consistency = gocql.One // ONE for single-node testcontainer
	cluster.ProtoVersion = 4
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 20 * time.Second

	// Retry loop for connection (Container is "Ready" before CQL port is actually up)
	for range 15 {
		testSession, err = cluster.CreateSession()
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("failed to connect to cassandra: %s", err)
	}

	// Downgrade replication factor for single-node test environment to allow LWT Quorum
	_ = testSession.Query(`ALTER KEYSPACE geoloc WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}`).Exec()

	// 3. Run All Tests
	exitCode := m.Run()

	// 4. Teardown
	testSession.Close()
	if err := cassandraContainer.Terminate(ctx); err != nil {
		log.Printf("failed to terminate container: %s", err)
	}

	os.Exit(exitCode)
}
