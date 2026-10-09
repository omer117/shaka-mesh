package domain

import "time"

type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PriceCents  int64  `json:"price_cents"`
	ImageURL    string `json:"image_url"`
	Inventory   int    `json:"inventory"`
	Featured    bool   `json:"featured"`
}

type CartItem struct {
	Product   Product `json:"product"`
	Quantity  int     `json:"quantity"`
	LineCents int64   `json:"line_cents"`
}

type Cart struct {
	Items      []CartItem `json:"items"`
	TotalCents int64      `json:"total_cents"`
}

type OrderItem struct {
	ProductID      string `json:"product_id"`
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	LineCents      int64  `json:"line_cents"`
}

type Order struct {
	ID         string      `json:"id"`
	Status     string      `json:"status"`
	TotalCents int64       `json:"total_cents"`
	CreatedAt  time.Time   `json:"created_at"`
	Items      []OrderItem `json:"items"`
}

type ForumCategory struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ThreadCount int    `json:"thread_count"`
}

type ForumThread struct {
	ID           string    `json:"id"`
	CategorySlug string    `json:"category_slug"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	AuthorName   string    `json:"author_name"`
	AuthorEmail  string    `json:"author_email"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ReplyCount   int       `json:"reply_count"`
}

type ForumPost struct {
	ID          string    `json:"id"`
	ThreadID    string    `json:"thread_id"`
	Body        string    `json:"body"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	CreatedAt   time.Time `json:"created_at"`
}

type ThreadDetail struct {
	Thread ForumThread `json:"thread"`
	Posts  []ForumPost `json:"posts"`
}
