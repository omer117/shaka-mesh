//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/omer-asraf/shaka-community-service/internal/domain"
	"github.com/omer-asraf/shaka-community-service/internal/repository"
	"github.com/omer-asraf/shaka-community-service/migrations"
)

// newRepo applies the community schema exactly like cmd/main.go does and
// returns a repository over a clean shop state.
func newRepo(t *testing.T) *repository.Postgres {
	t.Helper()
	ctx := context.Background()
	repo := repository.NewPostgres(testPool)
	if err := repo.Migrate(ctx, migrations.Schema); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, stmt := range []string{
		`DELETE FROM forum_posts WHERE user_id IS NOT NULL`,
		`DELETE FROM forum_threads WHERE user_id IS NOT NULL`,
		`DELETE FROM shop_order_items`, `DELETE FROM shop_orders`, `DELETE FROM shop_cart_items`,
		`DELETE FROM shop_products`, `DELETE FROM users`,
	} {
		if _, err := testPool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return repo
}

func seedUser(t *testing.T, id string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'h')`, id, id+"@example.com"); err != nil {
		t.Fatal(err)
	}
}

func seedProduct(t *testing.T, id string, price, inventory int) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO shop_products (id, name, category, price_cents, inventory)
		VALUES ($1, $2, 'Test', $3, $4)`, id, id, price, inventory); err != nil {
		t.Fatal(err)
	}
}

func inventory(t *testing.T, id string) int {
	t.Helper()
	return count(t, `SELECT inventory FROM shop_products WHERE id=$1`, id)
}

func count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrateIsIdempotent(t *testing.T) {
	repo := newRepo(t)
	if err := repo.Migrate(context.Background(), migrations.Schema); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestCheckoutDecrementsStockCreatesOrderAndEmptiesCart(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	seedUser(t, "u1")
	seedProduct(t, "board", 50000, 5)
	seedProduct(t, "wax", 700, 10)

	if err := repo.AddCartItem(ctx, "u1", "board", 2); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddCartItem(ctx, "u1", "wax", 3); err != nil {
		t.Fatal(err)
	}

	order, err := repo.Checkout(ctx, "u1", "order-1")
	if err != nil {
		t.Fatal(err)
	}
	if order.TotalCents != 2*50000+3*700 || len(order.Items) != 2 || order.Status != "paid" {
		t.Fatalf("unexpected order: %+v", order)
	}
	if got := inventory(t, "board"); got != 3 {
		t.Errorf("board inventory = %d, want 3", got)
	}
	if got := inventory(t, "wax"); got != 7 {
		t.Errorf("wax inventory = %d, want 7", got)
	}
	if got := count(t, `SELECT COUNT(*) FROM shop_orders WHERE id='order-1' AND user_id='u1' AND total_cents=$1`, order.TotalCents); got != 1 {
		t.Errorf("persisted orders = %d, want 1", got)
	}
	if got := count(t, `SELECT COUNT(*) FROM shop_order_items WHERE order_id='order-1'`); got != 2 {
		t.Errorf("persisted order lines = %d, want 2", got)
	}
	cart, err := repo.GetCart(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cart.Items) != 0 {
		t.Errorf("cart should be empty, has %d items", len(cart.Items))
	}
}

func TestCheckoutEmptyCart(t *testing.T) {
	repo := newRepo(t)
	seedUser(t, "u1")
	if _, err := repo.Checkout(context.Background(), "u1", "o"); !errors.Is(err, domain.ErrEmptyCart) {
		t.Fatalf("expected ErrEmptyCart, got %v", err)
	}
}

func TestCheckoutRollsBackEverythingWhenOneLineIsOutOfStock(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	seedUser(t, "u1")
	seedProduct(t, "a", 100, 5)
	seedProduct(t, "b", 100, 5)
	if err := repo.AddCartItem(ctx, "u1", "a", 2); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddCartItem(ctx, "u1", "b", 2); err != nil {
		t.Fatal(err)
	}
	// Stock of "b" drops after the items were carted (e.g. an admin correction).
	if _, err := testPool.Exec(ctx, `UPDATE shop_products SET inventory = 1 WHERE id = 'b'`); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Checkout(ctx, "u1", "o1"); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
	if got := inventory(t, "a"); got != 5 {
		t.Errorf("inventory of a = %d, want untouched 5 (rollback)", got)
	}
	if got := count(t, `SELECT COUNT(*) FROM shop_orders`); got != 0 {
		t.Errorf("orders = %d, want 0", got)
	}
	if got := count(t, `SELECT COUNT(*) FROM shop_cart_items WHERE user_id='u1'`); got != 2 {
		t.Errorf("cart items = %d, want 2 (kept)", got)
	}
}

func TestAddCartItemRejectsMoreThanInventory(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	seedUser(t, "u1")
	seedProduct(t, "last", 100, 1)
	if err := repo.AddCartItem(ctx, "u1", "last", 2); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}
	if err := repo.AddCartItem(ctx, "u1", "nope", 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Every user legitimately carted the last unit (adding to a cart does not
// reserve stock). Only one checkout may succeed; stock must end at 0, never
// negative, and exactly one order must exist.
func TestConcurrentCheckoutsForLastUnitNeverOversell(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	seedProduct(t, "last", 9900, 1)
	const users = 6
	for i := 0; i < users; i++ {
		id := fmt.Sprintf("u%d", i)
		seedUser(t, id)
		if err := repo.AddCartItem(ctx, id, "last", 1); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, users)
	start := make(chan struct{})
	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = repo.Checkout(ctx, fmt.Sprintf("u%d", i), fmt.Sprintf("order-%d", i))
		}(i)
	}
	close(start)
	wg.Wait()

	var ok, soldOut int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInsufficientStock):
			soldOut++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || soldOut != users-1 {
		t.Fatalf("got %d successes and %d sold-out, want 1 and %d", ok, soldOut, users-1)
	}
	if got := inventory(t, "last"); got != 0 {
		t.Fatalf("inventory = %d, want 0 (never negative)", got)
	}
	if got := count(t, `SELECT COUNT(*) FROM shop_orders`); got != 1 {
		t.Fatalf("orders = %d, want 1", got)
	}
}

func TestInventoryCheckConstraintBlocksNegativeStock(t *testing.T) {
	newRepo(t)
	seedProduct(t, "p", 100, 1)
	_, err := testPool.Exec(context.Background(), `UPDATE shop_products SET inventory = inventory - 2 WHERE id='p'`)
	if err == nil {
		t.Fatal("expected CHECK constraint violation")
	}
}

func TestForumThreadAndPostRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	seedUser(t, "u1")

	thread, err := repo.CreateThread(ctx, domain.ForumThread{ID: "t1", CategorySlug: "community", Title: "Hi", Body: "Body"}, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if thread.AuthorName != "u1" {
		t.Errorf("author name = %q", thread.AuthorName)
	}
	if _, err := repo.CreateThread(ctx, domain.ForumThread{ID: "t2", CategorySlug: "nope", Title: "x", Body: "y"}, "u1"); !errors.Is(err, domain.ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if _, err := repo.CreatePost(ctx, domain.ForumPost{ID: "p1", ThreadID: "t1", Body: "reply"}, "u1"); err != nil {
		t.Fatal(err)
	}
	detail, err := repo.GetThread(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Posts) != 1 || detail.Thread.ReplyCount != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if _, err := repo.CreatePost(ctx, domain.ForumPost{ID: "p2", ThreadID: "ghost", Body: "x"}, "u1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
