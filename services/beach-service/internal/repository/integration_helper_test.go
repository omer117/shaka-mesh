//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testPool is shared by every test in this package. It points at a throwaway
// postgis/postgis:16-3.4 container started once in TestMain.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// The same file docker compose mounts into /docker-entrypoint-initdb.d.
	initScript, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "deployments", "init_db.sql"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve init script:", err)
		return 1
	}
	container, err := tcpostgres.Run(ctx, "postgis/postgis:16-3.4",
		tcpostgres.WithDatabase("shaka_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.WithInitScripts(initScript),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres container (is Docker running?):", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(container) }()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "connection string:", err)
		return 1
	}
	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create pool:", err)
		return 1
	}
	defer testPool.Close()
	if err := testPool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		return 1
	}
	return m.Run()
}
