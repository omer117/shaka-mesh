package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-community-service/internal/domain"
)

type stubRepository struct {
	products       []domain.Product
	cart           domain.Cart
	order          domain.Order
	categories     []domain.ForumCategory
	threads        []domain.ForumThread
	detail         domain.ThreadDetail
	createdThread  domain.ForumThread
	createdPost    domain.ForumPost
	addedUserID    string
	addedProductID string
	addedQuantity  int
	checkoutErr    error
}

func (s *stubRepository) ListProducts(context.Context) ([]domain.Product, error) {
	return s.products, nil
}

func (s *stubRepository) GetCart(context.Context, string) (domain.Cart, error) {
	return s.cart, nil
}

func (s *stubRepository) AddCartItem(_ context.Context, userID, productID string, quantity int) error {
	s.addedUserID, s.addedProductID, s.addedQuantity = userID, productID, quantity
	return nil
}

func (s *stubRepository) RemoveCartItem(context.Context, string, string) error {
	return nil
}

func (s *stubRepository) Checkout(_ context.Context, _, orderID string) (domain.Order, error) {
	if s.checkoutErr != nil {
		return domain.Order{}, s.checkoutErr
	}
	s.order.ID = orderID
	return s.order, nil
}

func (s *stubRepository) ListCategories(context.Context) ([]domain.ForumCategory, error) {
	return s.categories, nil
}

func (s *stubRepository) ListThreads(context.Context, string) ([]domain.ForumThread, error) {
	return s.threads, nil
}

func (s *stubRepository) GetThread(context.Context, string) (domain.ThreadDetail, error) {
	return s.detail, nil
}

func (s *stubRepository) CreateThread(_ context.Context, thread domain.ForumThread, userID string) (domain.ForumThread, error) {
	thread.AuthorEmail = "surfer@example.com"
	s.createdThread = thread
	return thread, nil
}

func (s *stubRepository) CreatePost(_ context.Context, post domain.ForumPost, _ string) (domain.ForumPost, error) {
	post.AuthorEmail = "surfer@example.com"
	s.createdPost = post
	return post, nil
}

func TestListProductsReturnsCatalog(t *testing.T) {
	repository := &stubRepository{products: []domain.Product{{ID: "board-1", Name: "Fish"}}}
	response := httptest.NewRecorder()
	New(repository).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", response.Code, http.StatusOK)
	}
	var products []domain.Product
	if err := json.Unmarshal(response.Body.Bytes(), &products); err != nil {
		t.Fatalf("decode products: %v", err)
	}
	if len(products) != 1 || products[0].ID != "board-1" {
		t.Fatalf("unexpected products: %+v", products)
	}
}

func TestAddCartItemUsesVerifiedIdentityAndValidatesQuantity(t *testing.T) {
	repository := &stubRepository{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/shop/cart/items", strings.NewReader(
		`{"product_id":"board-1","quantity":2}`,
	))
	request.Header.Set("X-Authenticated-User-ID", "user-1")
	request.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	response := httptest.NewRecorder()
	New(repository).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d: %s", response.Code, http.StatusOK, response.Body)
	}
	if repository.addedUserID != "user-1" || repository.addedProductID != "board-1" || repository.addedQuantity != 2 {
		t.Fatalf("unexpected repository call: %+v", repository)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/api/v1/shop/cart/items", strings.NewReader(
		`{"product_id":"board-1","quantity":21}`,
	))
	invalid.Header.Set("X-Authenticated-User-ID", "user-1")
	invalid.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	invalidResponse := httptest.NewRecorder()
	New(repository).ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("got invalid quantity status %d, want %d", invalidResponse.Code, http.StatusBadRequest)
	}
}

func TestCheckoutRequiresIdentityAndMapsEmptyCart(t *testing.T) {
	repository := &stubRepository{checkoutErr: domain.ErrEmptyCart}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/shop/checkout", nil)
	response := httptest.NewRecorder()
	New(repository).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got unauthenticated status %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/shop/checkout", nil)
	request.Header.Set("X-Authenticated-User-ID", "user-1")
	request.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	response = httptest.NewRecorder()
	New(repository).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("got empty-cart status %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestCreateForumThreadUsesVerifiedAuthor(t *testing.T) {
	repository := &stubRepository{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forum/threads", strings.NewReader(
		`{"category_slug":"local-breaks","title":"Best dawn patrol?","body":"Any tips for tomorrow?"}`,
	))
	request.Header.Set("X-Authenticated-User-ID", "user-1")
	request.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	response := httptest.NewRecorder()
	New(repository).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("got status %d, want %d: %s", response.Code, http.StatusCreated, response.Body)
	}
	if repository.createdThread.ID == "" || repository.createdThread.AuthorEmail != "surfer@example.com" {
		t.Fatalf("unexpected created thread: %+v", repository.createdThread)
	}
}

func TestCreateForumThreadRejectsUnknownInputAndMissingIdentity(t *testing.T) {
	repository := &stubRepository{}
	handler := New(repository)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forum/threads", strings.NewReader(
		`{"category_slug":"local-breaks","title":"Hello","body":"World","author_email":"fake@example.com"}`,
	))
	request.Header.Set("X-Authenticated-User-ID", "user-1")
	request.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("got unknown-field status %d, want %d", response.Code, http.StatusBadRequest)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/forum/threads", strings.NewReader(
		`{"category_slug":"local-breaks","title":"Hello","body":"World"}`,
	))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got missing-identity status %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestCheckoutReturnsOrder(t *testing.T) {
	createdAt := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	repository := &stubRepository{
		order: domain.Order{
			Status: "paid", TotalCents: 64900, CreatedAt: createdAt,
			Items: []domain.OrderItem{{ProductID: "board-1", Quantity: 1}},
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/shop/checkout", nil)
	request.Header.Set("X-Authenticated-User-ID", "user-1")
	request.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	response := httptest.NewRecorder()
	New(repository).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("got status %d, want %d", response.Code, http.StatusCreated)
	}
	var order domain.Order
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if order.ID == "" || order.Status != "paid" || order.TotalCents != 64900 {
		t.Fatalf("unexpected order: %+v", order)
	}
}

func TestRepositoryErrorIsNotHiddenAsConflict(t *testing.T) {
	repository := &stubRepository{checkoutErr: errors.New("database unavailable")}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/shop/checkout", nil)
	request.Header.Set("X-Authenticated-User-ID", "user-1")
	request.Header.Set("X-Authenticated-User-Email", "surfer@example.com")
	response := httptest.NewRecorder()
	New(repository).ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
