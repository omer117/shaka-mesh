package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
)

func TestRouterForwardsAuthAndBeachRequests(t *testing.T) {
	type observedRequest struct {
		path       string
		method     string
		authUserID string
	}
	authRequests := make(chan observedRequest, 1)
	beachRequests := make(chan observedRequest, 1)
	communityRequests := make(chan observedRequest, 1)
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authRequests <- observedRequest{path: r.URL.Path, method: r.Method}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "auth")
	}))
	defer authServer.Close()
	beachServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		beachRequests <- observedRequest{
			path:       r.URL.Path,
			method:     r.Method,
			authUserID: r.Header.Get("X-Authenticated-User-ID"),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "beach")
	}))
	defer beachServer.Close()
	communityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		communityRequests <- observedRequest{
			path:       r.URL.Path,
			method:     r.Method,
			authUserID: r.Header.Get("X-Authenticated-User-ID"),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "community")
	}))
	defer communityServer.Close()
	authURL, _ := url.Parse(authServer.URL)
	beachURL, _ := url.Parse(beachServer.URL)
	communityURL, _ := url.Parse(communityServer.URL)
	jwtMiddleware, err := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler, err := NewHandler(authURL, beachURL, communityURL, jwtMiddleware, "http://localhost:3000")
	if err != nil {
		t.Fatalf("create gateway handler: %v", err)
	}

	authResponse := httptest.NewRecorder()
	handler.ServeHTTP(authResponse, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil))
	if authResponse.Code != http.StatusCreated {
		t.Fatalf("expected auth response 201, got %d", authResponse.Code)
	}
	if got := <-authRequests; got.path != "/auth/register" || got.method != http.MethodPost {
		t.Fatalf("unexpected auth upstream request: %+v", got)
	}

	beachResponse := httptest.NewRecorder()
	handler.ServeHTTP(beachResponse, httptest.NewRequest(http.MethodGet, "/api/v1/beaches/nearby?lat=1", nil))
	if beachResponse.Code != http.StatusOK {
		t.Fatalf("expected beach response 200, got %d", beachResponse.Code)
	}
	if got := <-beachRequests; got.path != "/api/v1/beaches/nearby" || got.method != http.MethodGet {
		t.Fatalf("unexpected beach upstream request: %+v", got)
	}

	protectedResponse := httptest.NewRecorder()
	protectedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/beaches", nil)
	protectedRequest.Header.Set("X-Authenticated-User-ID", "forged-client-value")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, authn.Claims{
		UserID: "user-42",
		Email:  "surfer@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "test-issuer", Subject: "user-42",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}).SignedString([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	protectedRequest.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(protectedResponse, protectedRequest)
	if protectedResponse.Code != http.StatusOK {
		t.Fatalf("expected protected beach response 200, got %d", protectedResponse.Code)
	}
	if got := <-beachRequests; got.authUserID != "user-42" {
		t.Fatalf("expected verified user identity header, got %+v", got)
	}

	storeResponse := httptest.NewRecorder()
	handler.ServeHTTP(storeResponse, httptest.NewRequest(http.MethodGet, "/api/v1/shop/products", nil))
	if storeResponse.Code != http.StatusOK {
		t.Fatalf("expected public products response 200, got %d", storeResponse.Code)
	}
	if got := <-communityRequests; got.path != "/api/v1/shop/products" || got.authUserID != "" {
		t.Fatalf("unexpected public community request: %+v", got)
	}

	threadRequest := httptest.NewRequest(http.MethodPost, "/api/v1/forum/threads", nil)
	threadRequest.Header.Set("Authorization", "Bearer "+token)
	threadResponse := httptest.NewRecorder()
	handler.ServeHTTP(threadResponse, threadRequest)
	if threadResponse.Code != http.StatusOK {
		t.Fatalf("expected protected forum response 200, got %d", threadResponse.Code)
	}
	if got := <-communityRequests; got.path != "/api/v1/forum/threads" || got.authUserID != "user-42" {
		t.Fatalf("expected verified identity at community service, got %+v", got)
	}
}

func TestRouterReturnsNotFoundForUnknownRoute(t *testing.T) {
	authURL, _ := url.Parse("http://localhost:8081")
	beachURL, _ := url.Parse("http://localhost:8080")
	communityURL, _ := url.Parse("http://localhost:8083")
	jwtMiddleware, err := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler, err := NewHandler(authURL, beachURL, communityURL, jwtMiddleware, "http://localhost:3000")
	if err != nil {
		t.Fatalf("create gateway handler: %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", response.Code)
	}
}

func TestRouterAllowsConfiguredFrontendCORS(t *testing.T) {
	authURL, _ := url.Parse("http://localhost:8081")
	beachURL, _ := url.Parse("http://localhost:8080")
	communityURL, _ := url.Parse("http://localhost:8083")
	jwtMiddleware, err := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler, err := NewHandler(authURL, beachURL, communityURL, jwtMiddleware, "http://localhost:3000")
	if err != nil {
		t.Fatalf("create gateway handler: %v", err)
	}

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/beaches", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected preflight 204, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("unexpected CORS origin %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentialed CORS, got %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type, Accept" {
		t.Fatalf("unexpected allowed headers %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PATCH, DELETE, OPTIONS" {
		t.Fatalf("unexpected allowed methods %q", got)
	}
}

func TestRouterRejectsUnconfiguredCORSOrigin(t *testing.T) {
	authURL, _ := url.Parse("http://localhost:8081")
	beachURL, _ := url.Parse("http://localhost:8080")
	communityURL, _ := url.Parse("http://localhost:8083")
	jwtMiddleware, err := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler, err := NewHandler(authURL, beachURL, communityURL, jwtMiddleware, "http://localhost:3000")
	if err != nil {
		t.Fatalf("create gateway handler: %v", err)
	}

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/beaches", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected disallowed preflight 403, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS origin header %q", got)
	}
}

func TestRouterRejectsWildcardCORSOrigin(t *testing.T) {
	target, _ := url.Parse("http://localhost:8081")
	jwtMiddleware, err := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	if _, err := NewHandler(target, target, target, jwtMiddleware, "*"); err == nil {
		t.Fatal("expected wildcard origin to be rejected")
	}
}

func TestRouterForwardsRefreshCookieBothWays(t *testing.T) {
	gotCookie := make(chan string, 1)
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("shaka_refresh"); err == nil {
			gotCookie <- c.Value
		}
		http.SetCookie(w, &http.Cookie{Name: "shaka_refresh", Value: "rotated", Path: "/api/v1/auth", HttpOnly: true})
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()
	authURL, _ := url.Parse(authServer.URL)
	other, _ := url.Parse("http://localhost:1")
	jwtMiddleware, err := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler, err := NewHandler(authURL, other, other, jwtMiddleware, "http://localhost:3000")
	if err != nil {
		t.Fatalf("create gateway handler: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	request.AddCookie(&http.Cookie{Name: "shaka_refresh", Value: "old"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := <-gotCookie; got != "old" {
		t.Fatalf("auth-service saw cookie %q", got)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Value != "rotated" || !cookies[0].HttpOnly {
		t.Fatalf("Set-Cookie was not forwarded intact: %+v", cookies)
	}
}
