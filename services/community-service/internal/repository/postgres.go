package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omer-asraf/shaka-community-service/internal/domain"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (p *Postgres) Migrate(ctx context.Context, schema string) error {
	if _, err := p.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("apply community schema: %w", err)
	}
	return nil
}

func (p *Postgres) ListProducts(ctx context.Context) ([]domain.Product, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, name, description, category, price_cents, image_url, inventory, featured
		FROM shop_products
		ORDER BY featured DESC, category, name
	`)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	products := make([]domain.Product, 0)
	for rows.Next() {
		var product domain.Product
		if err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Description,
			&product.Category,
			&product.PriceCents,
			&product.ImageURL,
			&product.Inventory,
			&product.Featured,
		); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

func (p *Postgres) GetCart(ctx context.Context, userID string) (domain.Cart, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT p.id, p.name, p.description, p.category, p.price_cents,
		       p.image_url, p.inventory, p.featured, ci.quantity
		FROM shop_cart_items ci
		JOIN shop_products p ON p.id = ci.product_id
		WHERE ci.user_id = $1
		ORDER BY p.name
	`, userID)
	if err != nil {
		return domain.Cart{}, fmt.Errorf("query cart: %w", err)
	}
	defer rows.Close()

	cart := domain.Cart{Items: make([]domain.CartItem, 0)}
	for rows.Next() {
		var item domain.CartItem
		if err := rows.Scan(
			&item.Product.ID,
			&item.Product.Name,
			&item.Product.Description,
			&item.Product.Category,
			&item.Product.PriceCents,
			&item.Product.ImageURL,
			&item.Product.Inventory,
			&item.Product.Featured,
			&item.Quantity,
		); err != nil {
			return domain.Cart{}, fmt.Errorf("scan cart item: %w", err)
		}
		item.LineCents = item.Product.PriceCents * int64(item.Quantity)
		cart.TotalCents += item.LineCents
		cart.Items = append(cart.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Cart{}, fmt.Errorf("iterate cart: %w", err)
	}
	return cart, nil
}

func (p *Postgres) AddCartItem(ctx context.Context, userID, productID string, quantity int) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cart update: %w", err)
	}
	defer tx.Rollback(ctx)

	var inventory int
	if err := tx.QueryRow(ctx, `SELECT inventory FROM shop_products WHERE id = $1 FOR UPDATE`, productID).Scan(&inventory); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return fmt.Errorf("load product inventory: %w", err)
	}

	var currentQuantity int
	err = tx.QueryRow(ctx, `
		SELECT quantity FROM shop_cart_items
		WHERE user_id = $1 AND product_id = $2
	`, userID, productID).Scan(&currentQuantity)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load current cart quantity: %w", err)
	}
	if currentQuantity+quantity > inventory {
		return domain.ErrInsufficientStock
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO shop_cart_items (user_id, product_id, quantity)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, product_id)
		DO UPDATE SET quantity = shop_cart_items.quantity + EXCLUDED.quantity,
		              updated_at = CURRENT_TIMESTAMP
	`, userID, productID, quantity)
	if err != nil {
		return fmt.Errorf("update cart item: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cart update: %w", err)
	}
	return nil
}

func (p *Postgres) RemoveCartItem(ctx context.Context, userID, productID string) error {
	result, err := p.pool.Exec(ctx, `
		DELETE FROM shop_cart_items WHERE user_id = $1 AND product_id = $2
	`, userID, productID)
	if err != nil {
		return fmt.Errorf("remove cart item: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (p *Postgres) Checkout(ctx context.Context, userID, orderID string) (domain.Order, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("begin checkout: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT p.id, p.name, p.price_cents, p.inventory, ci.quantity
		FROM shop_cart_items ci
		JOIN shop_products p ON p.id = ci.product_id
		WHERE ci.user_id = $1
		ORDER BY p.id
		FOR UPDATE OF p, ci
	`, userID)
	if err != nil {
		return domain.Order{}, fmt.Errorf("load checkout items: %w", err)
	}
	type checkoutItem struct {
		id, name       string
		price          int64
		inventory, qty int
	}
	items := make([]checkoutItem, 0)
	for rows.Next() {
		var item checkoutItem
		if err := rows.Scan(&item.id, &item.name, &item.price, &item.inventory, &item.qty); err != nil {
			rows.Close()
			return domain.Order{}, fmt.Errorf("scan checkout item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.Order{}, fmt.Errorf("iterate checkout items: %w", err)
	}
	rows.Close()
	if len(items) == 0 {
		return domain.Order{}, domain.ErrEmptyCart
	}

	order := domain.Order{
		ID:     orderID,
		Status: "paid",
		Items:  make([]domain.OrderItem, 0, len(items)),
	}
	for _, item := range items {
		if item.qty > item.inventory {
			return domain.Order{}, domain.ErrInsufficientStock
		}
		result, err := tx.Exec(ctx, `
			UPDATE shop_products SET inventory = inventory - $1
			WHERE id = $2 AND inventory >= $1
		`, item.qty, item.id)
		if err != nil {
			return domain.Order{}, fmt.Errorf("reserve product inventory: %w", err)
		}
		if result.RowsAffected() != 1 {
			return domain.Order{}, domain.ErrInsufficientStock
		}
		lineTotal := item.price * int64(item.qty)
		order.TotalCents += lineTotal
		order.Items = append(order.Items, domain.OrderItem{
			ProductID:      item.id,
			ProductName:    item.name,
			Quantity:       item.qty,
			UnitPriceCents: item.price,
			LineCents:      lineTotal,
		})
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO shop_orders (id, user_id, status, total_cents)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`, order.ID, userID, order.Status, order.TotalCents).Scan(&order.CreatedAt); err != nil {
		return domain.Order{}, fmt.Errorf("create order: %w", err)
	}
	for _, item := range order.Items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shop_order_items (order_id, product_id, product_name, quantity, unit_price_cents)
			VALUES ($1, $2, $3, $4, $5)
		`, order.ID, item.ProductID, item.ProductName, item.Quantity, item.UnitPriceCents); err != nil {
			return domain.Order{}, fmt.Errorf("create order line: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM shop_cart_items WHERE user_id = $1`, userID); err != nil {
		return domain.Order{}, fmt.Errorf("clear cart after checkout: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("commit checkout: %w", err)
	}
	return order, nil
}

func (p *Postgres) ListCategories(ctx context.Context) ([]domain.ForumCategory, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT c.slug, c.name, c.description, COUNT(t.id)::INTEGER
		FROM forum_categories c
		LEFT JOIN forum_threads t ON t.category_slug = c.slug
		GROUP BY c.slug, c.name, c.description, c.sort_order
		ORDER BY c.sort_order
	`)
	if err != nil {
		return nil, fmt.Errorf("query forum categories: %w", err)
	}
	defer rows.Close()
	categories := make([]domain.ForumCategory, 0)
	for rows.Next() {
		var category domain.ForumCategory
		if err := rows.Scan(&category.Slug, &category.Name, &category.Description, &category.ThreadCount); err != nil {
			return nil, fmt.Errorf("scan forum category: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forum categories: %w", err)
	}
	return categories, nil
}

func (p *Postgres) ListThreads(ctx context.Context, category string) ([]domain.ForumThread, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT t.id, t.category_slug, t.title, t.body,
		       COALESCE(t.author_name, split_part(u.email, '@', 1), 'Local surfer'),
		       COALESCE(u.email, ''), t.created_at, t.updated_at,
		       (SELECT COUNT(*)::INTEGER FROM forum_posts p WHERE p.thread_id = t.id)
		FROM forum_threads t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE ($1 = '' OR t.category_slug = $1)
		ORDER BY t.updated_at DESC, t.id
		LIMIT 100
	`, category)
	if err != nil {
		return nil, fmt.Errorf("query forum threads: %w", err)
	}
	defer rows.Close()
	threads := make([]domain.ForumThread, 0)
	for rows.Next() {
		var thread domain.ForumThread
		if err := rows.Scan(
			&thread.ID,
			&thread.CategorySlug,
			&thread.Title,
			&thread.Body,
			&thread.AuthorName,
			&thread.AuthorEmail,
			&thread.CreatedAt,
			&thread.UpdatedAt,
			&thread.ReplyCount,
		); err != nil {
			return nil, fmt.Errorf("scan forum thread: %w", err)
		}
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate forum threads: %w", err)
	}
	return threads, nil
}

func (p *Postgres) GetThread(ctx context.Context, threadID string) (domain.ThreadDetail, error) {
	var detail domain.ThreadDetail
	err := p.pool.QueryRow(ctx, `
		SELECT t.id, t.category_slug, t.title, t.body,
		       COALESCE(t.author_name, split_part(u.email, '@', 1), 'Local surfer'),
		       COALESCE(u.email, ''), t.created_at, t.updated_at,
		       (SELECT COUNT(*)::INTEGER FROM forum_posts p WHERE p.thread_id = t.id)
		FROM forum_threads t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.id = $1
	`, threadID).Scan(
		&detail.Thread.ID,
		&detail.Thread.CategorySlug,
		&detail.Thread.Title,
		&detail.Thread.Body,
		&detail.Thread.AuthorName,
		&detail.Thread.AuthorEmail,
		&detail.Thread.CreatedAt,
		&detail.Thread.UpdatedAt,
		&detail.Thread.ReplyCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ThreadDetail{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("load forum thread: %w", err)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT p.id, p.thread_id, p.body,
		       COALESCE(p.author_name, split_part(u.email, '@', 1), 'Local surfer'),
		       COALESCE(u.email, ''), p.created_at
		FROM forum_posts p LEFT JOIN users u ON u.id = p.user_id
		WHERE p.thread_id = $1
		ORDER BY p.created_at, p.id
	`, threadID)
	if err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("query thread posts: %w", err)
	}
	defer rows.Close()
	detail.Posts = make([]domain.ForumPost, 0)
	for rows.Next() {
		var post domain.ForumPost
		if err := rows.Scan(&post.ID, &post.ThreadID, &post.Body, &post.AuthorName, &post.AuthorEmail, &post.CreatedAt); err != nil {
			return domain.ThreadDetail{}, fmt.Errorf("scan forum post: %w", err)
		}
		detail.Posts = append(detail.Posts, post)
	}
	if err := rows.Err(); err != nil {
		return domain.ThreadDetail{}, fmt.Errorf("iterate thread posts: %w", err)
	}
	return detail, nil
}

func (p *Postgres) CreateThread(ctx context.Context, thread domain.ForumThread, userID string) (domain.ForumThread, error) {
	err := p.pool.QueryRow(ctx, `
		INSERT INTO forum_threads (id, category_slug, user_id, title, body)
		SELECT $1, c.slug, $2, $4, $5
		FROM forum_categories c WHERE c.slug = $3
		RETURNING created_at, updated_at
	`, thread.ID, userID, thread.CategorySlug, thread.Title, thread.Body).Scan(&thread.CreatedAt, &thread.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ForumThread{}, domain.ErrInvalidCategory
	}
	if err != nil {
		return domain.ForumThread{}, fmt.Errorf("create forum thread: %w", err)
	}
	if err := p.pool.QueryRow(ctx, `
		SELECT email, split_part(email, '@', 1) FROM users WHERE id = $1
	`, userID).Scan(&thread.AuthorEmail, &thread.AuthorName); err != nil {
		return domain.ForumThread{}, fmt.Errorf("load thread author: %w", err)
	}
	return thread, nil
}

func (p *Postgres) CreatePost(ctx context.Context, post domain.ForumPost, userID string) (domain.ForumPost, error) {
	err := p.pool.QueryRow(ctx, `
		INSERT INTO forum_posts (id, thread_id, user_id, body)
		SELECT $1, t.id, $3, $4 FROM forum_threads t WHERE t.id = $2
		RETURNING created_at
	`, post.ID, post.ThreadID, userID, post.Body).Scan(&post.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ForumPost{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ForumPost{}, fmt.Errorf("create forum post: %w", err)
	}
	if _, err := p.pool.Exec(ctx, `
		UPDATE forum_threads SET updated_at = $1 WHERE id = $2
	`, post.CreatedAt, post.ThreadID); err != nil {
		return domain.ForumPost{}, fmt.Errorf("update forum thread activity: %w", err)
	}
	if err := p.pool.QueryRow(ctx, `
		SELECT email, split_part(email, '@', 1) FROM users WHERE id = $1
	`, userID).Scan(&post.AuthorEmail, &post.AuthorName); err != nil {
		return domain.ForumPost{}, fmt.Errorf("load post author: %w", err)
	}
	return post, nil
}
