package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/omer-asraf/shaka-community-service/internal/domain"
)

const maxBodyBytes = 64 << 10

type Repository interface {
	ListProducts(context.Context) ([]domain.Product, error)
	GetCart(context.Context, string) (domain.Cart, error)
	AddCartItem(context.Context, string, string, int) error
	RemoveCartItem(context.Context, string, string) error
	Checkout(context.Context, string, string) (domain.Order, error)
	ListCategories(context.Context) ([]domain.ForumCategory, error)
	ListThreads(context.Context, string) ([]domain.ForumThread, error)
	GetThread(context.Context, string) (domain.ThreadDetail, error)
	CreateThread(context.Context, domain.ForumThread, string) (domain.ForumThread, error)
	CreatePost(context.Context, domain.ForumPost, string) (domain.ForumPost, error)
}

type Handler struct {
	repository Repository
}

func New(repository Repository) http.Handler {
	h := Handler{repository: repository}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /api/v1/shop/products", h.listProducts)
	mux.HandleFunc("GET /api/v1/shop/cart", h.getCart)
	mux.HandleFunc("POST /api/v1/shop/cart/items", h.addCartItem)
	mux.HandleFunc("DELETE /api/v1/shop/cart/items/{productID}", h.removeCartItem)
	mux.HandleFunc("POST /api/v1/shop/checkout", h.checkout)
	mux.HandleFunc("GET /api/v1/forum/categories", h.listCategories)
	mux.HandleFunc("GET /api/v1/forum/threads", h.listThreads)
	mux.HandleFunc("POST /api/v1/forum/threads", h.createThread)
	mux.HandleFunc("GET /api/v1/forum/threads/{threadID}", h.getThread)
	mux.HandleFunc("POST /api/v1/forum/threads/{threadID}/posts", h.createPost)
	return mux
}

func (h Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.repository.ListProducts(r.Context())
	if err != nil {
		h.internalError(w, "list products", err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (h Handler) getCart(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	cart, err := h.repository.GetCart(r.Context(), userID)
	if err != nil {
		h.internalError(w, "get cart", err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

func (h Handler) addCartItem(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var input struct {
		ProductID string `json:"product_id"`
		Quantity  int    `json:"quantity"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.ProductID) == "" || input.Quantity < 1 || input.Quantity > 20 {
		writeError(w, http.StatusBadRequest, "product_id is required and quantity must be between 1 and 20")
		return
	}
	if err := h.repository.AddCartItem(r.Context(), userID, input.ProductID, input.Quantity); err != nil {
		h.repositoryError(w, "add cart item", err)
		return
	}
	cart, err := h.repository.GetCart(r.Context(), userID)
	if err != nil {
		h.internalError(w, "get updated cart", err)
		return
	}
	writeJSON(w, http.StatusOK, cart)
}

func (h Handler) removeCartItem(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	if err := h.repository.RemoveCartItem(r.Context(), userID, r.PathValue("productID")); err != nil {
		h.repositoryError(w, "remove cart item", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) checkout(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	orderID, err := newID()
	if err != nil {
		h.internalError(w, "generate order ID", err)
		return
	}
	order, err := h.repository.Checkout(r.Context(), userID, orderID)
	if err != nil {
		h.repositoryError(w, "checkout", err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.repository.ListCategories(r.Context())
	if err != nil {
		h.internalError(w, "list forum categories", err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h Handler) listThreads(w http.ResponseWriter, r *http.Request) {
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	if len(category) > 50 {
		writeError(w, http.StatusBadRequest, "category is too long")
		return
	}
	threads, err := h.repository.ListThreads(r.Context(), category)
	if err != nil {
		h.internalError(w, "list forum threads", err)
		return
	}
	writeJSON(w, http.StatusOK, threads)
}

func (h Handler) getThread(w http.ResponseWriter, r *http.Request) {
	detail, err := h.repository.GetThread(r.Context(), r.PathValue("threadID"))
	if err != nil {
		h.repositoryError(w, "get forum thread", err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h Handler) createThread(w http.ResponseWriter, r *http.Request) {
	userID, email, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var input struct {
		CategorySlug string `json:"category_slug"`
		Title        string `json:"title"`
		Body         string `json:"body"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	input.CategorySlug = strings.TrimSpace(input.CategorySlug)
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	if input.CategorySlug == "" || len(input.CategorySlug) > 50 {
		writeError(w, http.StatusBadRequest, "category_slug is required")
		return
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 180 {
		writeError(w, http.StatusBadRequest, "title is required and must be at most 180 characters")
		return
	}
	if input.Body == "" || utf8.RuneCountInString(input.Body) > 10000 {
		writeError(w, http.StatusBadRequest, "body is required and must be at most 10000 characters")
		return
	}
	threadID, err := newID()
	if err != nil {
		h.internalError(w, "generate thread ID", err)
		return
	}
	thread, err := h.repository.CreateThread(r.Context(), domain.ForumThread{
		ID: threadID, CategorySlug: input.CategorySlug, Title: input.Title, Body: input.Body, AuthorEmail: email,
	}, userID)
	if err != nil {
		h.repositoryError(w, "create forum thread", err)
		return
	}
	writeJSON(w, http.StatusCreated, thread)
}

func (h Handler) createPost(w http.ResponseWriter, r *http.Request) {
	userID, email, ok := authenticatedUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" || utf8.RuneCountInString(input.Body) > 10000 {
		writeError(w, http.StatusBadRequest, "body is required and must be at most 10000 characters")
		return
	}
	postID, err := newID()
	if err != nil {
		h.internalError(w, "generate forum post ID", err)
		return
	}
	post, err := h.repository.CreatePost(r.Context(), domain.ForumPost{
		ID: postID, ThreadID: r.PathValue("threadID"), Body: input.Body, AuthorEmail: email,
	}, userID)
	if err != nil {
		h.repositoryError(w, "create forum post", err)
		return
	}
	writeJSON(w, http.StatusCreated, post)
}

func authenticatedUser(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	userID := strings.TrimSpace(r.Header.Get("X-Authenticated-User-ID"))
	email := strings.TrimSpace(r.Header.Get("X-Authenticated-User-Email"))
	if userID == "" || email == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", "", false
	}
	if _, err := mail.ParseAddress(email); err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", "", false
	}
	return userID, email, true
}

func (h Handler) repositoryError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, domain.ErrInvalidCategory):
		writeError(w, http.StatusBadRequest, "forum category does not exist")
	case errors.Is(err, domain.ErrEmptyCart):
		writeError(w, http.StatusConflict, "cart is empty")
	case errors.Is(err, domain.ErrInsufficientStock):
		writeError(w, http.StatusConflict, "requested quantity is no longer available")
	default:
		h.internalError(w, operation, err)
	}
}

func (h Handler) internalError(w http.ResponseWriter, operation string, err error) {
	log.Printf("%s: %v", operation, err)
	writeError(w, http.StatusInternalServerError, "request could not be completed")
}

func decodeBody(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain a single JSON value")
		return false
	}
	return true
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode community-service response: %v", err)
	}
}
