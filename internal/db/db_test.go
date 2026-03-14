package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *Client
var testContainer testcontainers.Container

func TestMain(m *testing.M) {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	ctx := context.Background()

	var err error
	testContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "surrealdb/surrealdb:v3.0.2",
			ExposedPorts: []string{"8000/tcp"},
			Cmd:          []string{"start", "--log", "info", "--user", "root", "--pass", "root"},
			WaitingFor:   wait.ForLog("Started web server").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		log.Fatalf("Failed to start SurrealDB container: %v", err)
	}

	host, err := testContainer.Host(ctx)
	if err != nil {
		log.Fatalf("Failed to get container host: %v", err)
	}
	// Colima VM IPs are not reachable; ports are forwarded to localhost
	if host == "" || host == "null" || host == "192.168.64.2" {
		host = "localhost"
	}
	mappedPort, err := testContainer.MappedPort(ctx, "8000")
	if err != nil {
		log.Fatalf("Failed to get mapped port: %v", err)
	}

	testDB, err = NewClient(ctx, Config{
		URL:       fmt.Sprintf("ws://%s:%s/rpc", host, mappedPort.Port()),
		Namespace: "test_v2",
		Database:  "test_v2",
		Username:  "root",
		Password:  "root",
		AuthLevel: "root",
	}, nil, nil)
	if err != nil {
		log.Fatalf("Failed to connect to test database: %v", err)
	}

	if err := testDB.InitSchema(ctx, 384); err != nil {
		log.Fatalf("Failed to initialize v2 schema: %v", err)
	}

	code := m.Run()

	// Cleanup: best-effort, process is exiting
	if err := testDB.Close(ctx); err != nil {
		log.Printf("warning: failed to close test DB: %v", err)
	}
	if err := testContainer.Terminate(ctx); err != nil {
		log.Printf("warning: failed to terminate container: %v", err)
	}

	os.Exit(code)
}

func dummyEmbedding() []float32 {
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = float32(i) / 384.0
	}
	return embedding
}

func createTestUser(t *testing.T, ctx context.Context) *models.User {
	t.Helper()
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "testuser-" + fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return user
}

func createTestVault(t *testing.T, ctx context.Context, userID string) *models.Vault {
	t.Helper()
	vault, err := testDB.CreateVault(ctx, userID, models.VaultInput{
		Name: "test-vault-" + fmt.Sprint(time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatalf("Failed to create test vault: %v", err)
	}
	return vault
}
