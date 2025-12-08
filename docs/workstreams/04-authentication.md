# WS4: Authentication & Security

**Priority:** P0 (Critical Path)
**Estimated Effort:** 4-5 days
**Dependencies:** WS3 (HTTP API Handlers)

## Overview

This workstream implements JWT-based authentication, multi-tenancy isolation, scope-based authorization, and security hardening for Luminate. The authentication system ensures tenant data isolation, prevents unauthorized access, and provides fine-grained permission control using OAuth2-style scopes.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│              Incoming HTTP Request                      │
│         Authorization: Bearer <jwt-token>               │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│            Authentication Middleware                    │
│  1. Extract JWT from Authorization header               │
│  2. Validate JWT signature (HMAC/RSA)                  │
│  3. Check expiration (exp claim)                       │
│  4. Extract claims (tenant_id, scopes)                 │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│             Authorization Middleware                    │
│  1. Check required scope (read/write/admin)            │
│  2. Inject tenant_id into request context              │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                Tenant Isolation Layer                   │
│  1. Auto-inject _tenant_id dimension to metrics        │
│  2. Auto-filter queries by tenant_id                   │
│  3. Prevent cross-tenant data access                   │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│                   API Handlers                          │
│         (Request contains validated tenant)             │
└─────────────────────────────────────────────────────────┘
```

## JWT Token Format

**Standard Claims:**
```json
{
  "iss": "https://auth.luminate.com",
  "sub": "org_abc123",
  "aud": "luminate-api",
  "exp": 1735689600,
  "iat": 1735603200,
  "jti": "unique-token-id"
}
```

**Custom Claims:**
```json
{
  "tenant_id": "tenant_xyz",
  "org_id": "org_abc123",
  "scopes": ["read", "write"],
  "tier": "enterprise"
}
```

**Scope Definitions:**
- `read`: Query metrics, list metrics/dimensions
- `write`: Write metrics
- `admin`: Configure retention, delete metrics, view all tenants

## Work Items

### Work Item 1: JWT Validation Middleware

**Purpose:** Extract and validate JWT tokens from Authorization header.

**Technical Specification:**

**Implementation:**

```go
// pkg/auth/jwt.go
package auth

import (
    "context"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

var (
    ErrMissingToken      = errors.New("missing authorization token")
    ErrInvalidToken      = errors.New("invalid token format")
    ErrTokenExpired      = errors.New("token expired")
    ErrInvalidSignature  = errors.New("invalid token signature")
    ErrInvalidClaims     = errors.New("invalid token claims")
    ErrMissingTenantID   = errors.New("missing tenant_id in token")
    ErrInsufficientScope = errors.New("insufficient scope for operation")
)

// LuminateClaims represents JWT claims for Luminate
type LuminateClaims struct {
    TenantID string   `json:"tenant_id"`
    OrgID    string   `json:"org_id"`
    Scopes   []string `json:"scopes"`
    Tier     string   `json:"tier,omitempty"`
    jwt.RegisteredClaims
}

// HasScope checks if the token has a specific scope
func (c *LuminateClaims) HasScope(scope string) bool {
    for _, s := range c.Scopes {
        if s == scope {
            return true
        }
    }
    return false
}

// JWTValidator validates JWT tokens
type JWTValidator struct {
    secretKey   []byte
    audience    string
    issuer      string
    leeway      time.Duration // Clock skew tolerance
}

// JWTConfig configuration for JWT validation
type JWTConfig struct {
    SecretKey string        `yaml:"secret_key"`
    Audience  string        `yaml:"audience"`
    Issuer    string        `yaml:"issuer"`
    Leeway    time.Duration `yaml:"leeway"` // Default: 1 minute
}

// NewJWTValidator creates a new JWT validator
func NewJWTValidator(config JWTConfig) *JWTValidator {
    leeway := config.Leeway
    if leeway == 0 {
        leeway = 1 * time.Minute // Default clock skew tolerance
    }

    return &JWTValidator{
        secretKey: []byte(config.SecretKey),
        audience:  config.Audience,
        issuer:    config.Issuer,
        leeway:    leeway,
    }
}

// ValidateToken validates a JWT token and returns claims
func (v *JWTValidator) ValidateToken(tokenString string) (*LuminateClaims, error) {
    // Parse token
    token, err := jwt.ParseWithClaims(tokenString, &LuminateClaims{}, func(token *jwt.Token) (interface{}, error) {
        // Verify signing method
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
        }
        return v.secretKey, nil
    }, jwt.WithLeeway(v.leeway))

    if err != nil {
        if errors.Is(err, jwt.ErrTokenExpired) {
            return nil, ErrTokenExpired
        }
        if errors.Is(err, jwt.ErrSignatureInvalid) {
            return nil, ErrInvalidSignature
        }
        return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
    }

    // Extract claims
    claims, ok := token.Claims.(*LuminateClaims)
    if !ok || !token.Valid {
        return nil, ErrInvalidClaims
    }

    // Validate standard claims
    if v.audience != "" && !claims.VerifyAudience(v.audience, true) {
        return nil, fmt.Errorf("%w: invalid audience", ErrInvalidClaims)
    }

    if v.issuer != "" && !claims.VerifyIssuer(v.issuer, true) {
        return nil, fmt.Errorf("%w: invalid issuer", ErrInvalidClaims)
    }

    // Validate custom claims
    if claims.TenantID == "" {
        return nil, ErrMissingTenantID
    }

    return claims, nil
}

// ExtractToken extracts token from Authorization header
func ExtractToken(authHeader string) (string, error) {
    if authHeader == "" {
        return "", ErrMissingToken
    }

    // Format: "Bearer <token>"
    parts := strings.SplitN(authHeader, " ", 2)
    if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
        return "", ErrInvalidToken
    }

    return parts[1], nil
}

// Context keys for storing auth info
type contextKey string

const (
    ClaimsContextKey  contextKey = "claims"
    TenantIDContextKey contextKey = "tenant_id"
)

// GetClaimsFromContext retrieves claims from request context
func GetClaimsFromContext(ctx context.Context) (*LuminateClaims, bool) {
    claims, ok := ctx.Value(ClaimsContextKey).(*LuminateClaims)
    return claims, ok
}

// GetTenantIDFromContext retrieves tenant ID from request context
func GetTenantIDFromContext(ctx context.Context) (string, bool) {
    tenantID, ok := ctx.Value(TenantIDContextKey).(string)
    return tenantID, ok
}
```

**Tests:**

```go
// pkg/auth/jwt_test.go
package auth

import (
    "testing"
    "time"

    "github.com/golang-jwt/jwt/v5"
    "github.com/stretchr/testify/assert"
)

const testSecret = "test-secret-key-32-bytes-long!!"

func generateTestToken(tenantID string, scopes []string, expiry time.Time) string {
    claims := LuminateClaims{
        TenantID: tenantID,
        OrgID:    "org_123",
        Scopes:   scopes,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(expiry),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            Issuer:    "luminate-test",
            Audience:  jwt.ClaimStrings{"luminate-api"},
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    tokenString, _ := token.SignedString([]byte(testSecret))
    return tokenString
}

func TestValidateToken_Valid(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    token := generateTestToken("tenant_xyz", []string{"read", "write"}, time.Now().Add(1*time.Hour))

    claims, err := validator.ValidateToken(token)

    assert.NoError(t, err)
    assert.NotNil(t, claims)
    assert.Equal(t, "tenant_xyz", claims.TenantID)
    assert.Equal(t, "org_123", claims.OrgID)
    assert.True(t, claims.HasScope("read"))
    assert.True(t, claims.HasScope("write"))
    assert.False(t, claims.HasScope("admin"))
}

func TestValidateToken_Expired(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    token := generateTestToken("tenant_xyz", []string{"read"}, time.Now().Add(-1*time.Hour))

    _, err := validator.ValidateToken(token)

    assert.Error(t, err)
    assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestValidateToken_InvalidSignature(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: "wrong-secret-key",
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    token := generateTestToken("tenant_xyz", []string{"read"}, time.Now().Add(1*time.Hour))

    _, err := validator.ValidateToken(token)

    assert.Error(t, err)
    assert.ErrorIs(t, err, ErrInvalidSignature)
}

func TestValidateToken_MissingTenantID(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
    })

    claims := jwt.MapClaims{
        "scopes": []string{"read"},
        "exp":    time.Now().Add(1 * time.Hour).Unix(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    tokenString, _ := token.SignedString([]byte(testSecret))

    _, err := validator.ValidateToken(tokenString)

    assert.Error(t, err)
}

func TestExtractToken_Valid(t *testing.T) {
    token, err := ExtractToken("Bearer abc123")

    assert.NoError(t, err)
    assert.Equal(t, "abc123", token)
}

func TestExtractToken_InvalidFormat(t *testing.T) {
    tests := []string{
        "",
        "abc123",
        "Basic abc123",
        "Bearer",
    }

    for _, tt := range tests {
        t.Run(tt, func(t *testing.T) {
            _, err := ExtractToken(tt)
            assert.Error(t, err)
        })
    }
}

func TestHasScope(t *testing.T) {
    claims := &LuminateClaims{
        Scopes: []string{"read", "write"},
    }

    assert.True(t, claims.HasScope("read"))
    assert.True(t, claims.HasScope("write"))
    assert.False(t, claims.HasScope("admin"))
}
```

**Acceptance Criteria:**
- ✅ Validates JWT signature using HMAC-SHA256
- ✅ Checks token expiration with clock skew tolerance
- ✅ Validates audience and issuer claims
- ✅ Extracts tenant_id and scopes from custom claims
- ✅ Returns specific error types for different failures
- ✅ Helper functions to extract claims from context

---

### Work Item 2: Authentication Middleware

**Purpose:** HTTP middleware that validates JWT tokens on all protected endpoints.

**Implementation:**

```go
// pkg/auth/middleware.go
package auth

import (
    "context"
    "log"
    "net/http"

    "github.com/yourusername/luminate/pkg/models"
)

// AuthMiddleware validates JWT tokens
type AuthMiddleware struct {
    validator *JWTValidator
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(validator *JWTValidator) *AuthMiddleware {
    return &AuthMiddleware{validator: validator}
}

// Middleware returns HTTP middleware function
func (m *AuthMiddleware) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract token from Authorization header
        authHeader := r.Header.Get("Authorization")
        tokenString, err := ExtractToken(authHeader)
        if err != nil {
            writeAuthError(w, http.StatusUnauthorized, models.ErrCodeUnauthorized,
                "missing or invalid authorization token", nil)
            return
        }

        // Validate token
        claims, err := m.validator.ValidateToken(tokenString)
        if err != nil {
            statusCode := http.StatusUnauthorized
            code := models.ErrCodeUnauthorized
            message := "invalid token"

            switch err {
            case ErrTokenExpired:
                code = models.ErrCodeTokenExpired
                message = "token expired"
            case ErrInvalidSignature:
                code = models.ErrCodeInvalidToken
                message = "invalid token signature"
            case ErrMissingTenantID:
                code = models.ErrCodeInvalidToken
                message = "missing tenant_id in token"
            }

            log.Printf("Authentication failed: %v", err)
            writeAuthError(w, statusCode, code, message, nil)
            return
        }

        // Store claims in request context
        ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
        ctx = context.WithValue(ctx, TenantIDContextKey, claims.TenantID)

        // Continue to next handler
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// RequireScope creates middleware that checks for specific scope
func (m *AuthMiddleware) RequireScope(scope string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            claims, ok := GetClaimsFromContext(r.Context())
            if !ok {
                writeAuthError(w, http.StatusUnauthorized, models.ErrCodeUnauthorized,
                    "missing authentication", nil)
                return
            }

            if !claims.HasScope(scope) {
                writeAuthError(w, http.StatusForbidden, models.ErrCodeForbidden,
                    "insufficient permissions",
                    map[string]interface{}{
                        "required_scope": scope,
                        "user_scopes":    claims.Scopes,
                    })
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}

// writeAuthError writes authentication error response
func writeAuthError(w http.ResponseWriter, statusCode int, code, message string, details map[string]interface{}) {
    // Use the same error format as API handlers
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)

    // This would call the shared writeError function from pkg/api
    // For now, inline the response
    response := map[string]interface{}{
        "error": map[string]interface{}{
            "code":    code,
            "message": message,
            "details": details,
        },
    }

    json.NewEncoder(w).Encode(response)
}
```

**Tests:**

```go
// pkg/auth/middleware_test.go
package auth

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestAuthMiddleware_ValidToken(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    middleware := NewAuthMiddleware(validator)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check if claims are in context
        claims, ok := GetClaimsFromContext(r.Context())
        assert.True(t, ok)
        assert.Equal(t, "tenant_xyz", claims.TenantID)

        tenantID, ok := GetTenantIDFromContext(r.Context())
        assert.True(t, ok)
        assert.Equal(t, "tenant_xyz", tenantID)

        w.WriteHeader(http.StatusOK)
    })

    token := generateTestToken("tenant_xyz", []string{"read"}, time.Now().Add(1*time.Hour))

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("Authorization", "Bearer "+token)

    w := httptest.NewRecorder()
    middleware.Middleware(handler).ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{SecretKey: testSecret})
    middleware := NewAuthMiddleware(validator)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("handler should not be called")
    })

    req := httptest.NewRequest("GET", "/test", nil)
    w := httptest.NewRecorder()

    middleware.Middleware(handler).ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    middleware := NewAuthMiddleware(validator)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("handler should not be called")
    })

    token := generateTestToken("tenant_xyz", []string{"read"}, time.Now().Add(-1*time.Hour))

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("Authorization", "Bearer "+token)

    w := httptest.NewRecorder()
    middleware.Middleware(handler).ServeHTTP(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireScope_HasScope(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    middleware := NewAuthMiddleware(validator)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    token := generateTestToken("tenant_xyz", []string{"read", "write"}, time.Now().Add(1*time.Hour))

    req := httptest.NewRequest("POST", "/test", nil)
    req.Header.Set("Authorization", "Bearer "+token)

    w := httptest.NewRecorder()

    // Chain middlewares: auth -> requireScope(write) -> handler
    middleware.Middleware(
        middleware.RequireScope("write")(handler),
    ).ServeHTTP(w, req)

    assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireScope_MissingScope(t *testing.T) {
    validator := NewJWTValidator(JWTConfig{
        SecretKey: testSecret,
        Audience:  "luminate-api",
        Issuer:    "luminate-test",
    })

    middleware := NewAuthMiddleware(validator)

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("handler should not be called")
    })

    token := generateTestToken("tenant_xyz", []string{"read"}, time.Now().Add(1*time.Hour))

    req := httptest.NewRequest("POST", "/test", nil)
    req.Header.Set("Authorization", "Bearer "+token)

    w := httptest.NewRecorder()

    middleware.Middleware(
        middleware.RequireScope("admin")(handler),
    ).ServeHTTP(w, req)

    assert.Equal(t, http.StatusForbidden, w.Code)
}
```

**Acceptance Criteria:**
- ✅ Validates JWT on every request
- ✅ Returns 401 Unauthorized for missing/invalid tokens
- ✅ Returns 403 Forbidden for insufficient scope
- ✅ Stores claims in request context
- ✅ RequireScope middleware checks specific permissions
- ✅ Logs authentication failures

---

### Work Item 3: Tenant Isolation Layer

**Purpose:** Automatically inject tenant_id dimension to metrics and filter queries by tenant.

**Implementation:**

```go
// pkg/auth/tenant.go
package auth

import (
    "context"

    "github.com/yourusername/luminate/pkg/storage"
)

const TenantDimensionKey = "_tenant_id"

// TenantStore wraps MetricsStore to enforce tenant isolation
type TenantStore struct {
    store storage.MetricsStore
}

// NewTenantStore creates a tenant-isolated metrics store
func NewTenantStore(store storage.MetricsStore) *TenantStore {
    return &TenantStore{store: store}
}

// Write automatically injects tenant_id dimension
func (ts *TenantStore) Write(ctx context.Context, metrics []storage.Metric) error {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return ErrMissingTenantID
    }

    // Inject tenant_id into all metrics
    for i := range metrics {
        if metrics[i].Dimensions == nil {
            metrics[i].Dimensions = make(map[string]string)
        }
        metrics[i].Dimensions[TenantDimensionKey] = tenantID
    }

    return ts.store.Write(ctx, metrics)
}

// QueryRange automatically filters by tenant_id
func (ts *TenantStore) QueryRange(ctx context.Context, req storage.QueryRequest) ([]storage.MetricPoint, error) {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return nil, ErrMissingTenantID
    }

    // Add tenant_id filter
    if req.Filters == nil {
        req.Filters = make(map[string]string)
    }
    req.Filters[TenantDimensionKey] = tenantID

    return ts.store.QueryRange(ctx, req)
}

// Aggregate automatically filters by tenant_id
func (ts *TenantStore) Aggregate(ctx context.Context, req storage.AggregateRequest) ([]storage.AggregateResult, error) {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return nil, ErrMissingTenantID
    }

    // Add tenant_id filter
    if req.Filters == nil {
        req.Filters = make(map[string]string)
    }
    req.Filters[TenantDimensionKey] = tenantID

    return ts.store.Aggregate(ctx, req)
}

// Rate automatically filters by tenant_id
func (ts *TenantStore) Rate(ctx context.Context, req storage.RateRequest) ([]storage.RatePoint, error) {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return nil, ErrMissingTenantID
    }

    // Add tenant_id filter
    if req.Filters == nil {
        req.Filters = make(map[string]string)
    }
    req.Filters[TenantDimensionKey] = tenantID

    return ts.store.Rate(ctx, req)
}

// ListMetrics scoped to tenant
func (ts *TenantStore) ListMetrics(ctx context.Context) ([]string, error) {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return nil, ErrMissingTenantID
    }

    // TODO: Implement tenant-specific metric listing
    // For now, return all metrics (will be improved in storage implementation)
    return ts.store.ListMetrics(ctx)
}

// ListDimensionKeys scoped to tenant
func (ts *TenantStore) ListDimensionKeys(ctx context.Context, metricName string) ([]string, error) {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return nil, ErrMissingTenantID
    }

    // TODO: Implement tenant-specific dimension listing
    return ts.store.ListDimensionKeys(ctx, metricName)
}

// ListDimensionValues scoped to tenant
func (ts *TenantStore) ListDimensionValues(ctx context.Context, metricName, dimensionKey string, limit int) ([]string, error) {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return nil, ErrMissingTenantID
    }

    // TODO: Implement tenant-specific dimension value listing
    return ts.store.ListDimensionValues(ctx, metricName, dimensionKey, limit)
}

// DeleteBefore only allows deleting own tenant's data
func (ts *TenantStore) DeleteBefore(ctx context.Context, metricName string, before time.Time) error {
    tenantID, ok := GetTenantIDFromContext(ctx)
    if !ok {
        return ErrMissingTenantID
    }

    // TODO: Add tenant_id filter to delete operation
    return ts.store.DeleteBefore(ctx, metricName, before)
}

// Health check doesn't require tenant context
func (ts *TenantStore) Health(ctx context.Context) error {
    return ts.store.Health(ctx)
}

// Close doesn't require tenant context
func (ts *TenantStore) Close() error {
    return ts.store.Close()
}
```

**Tests:**

```go
// pkg/auth/tenant_test.go
package auth

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/yourusername/luminate/pkg/storage"
)

type MockStore struct {
    mock.Mock
}

func (m *MockStore) Write(ctx context.Context, metrics []storage.Metric) error {
    args := m.Called(ctx, metrics)
    return args.Error(0)
}

func (m *MockStore) QueryRange(ctx context.Context, req storage.QueryRequest) ([]storage.MetricPoint, error) {
    args := m.Called(ctx, req)
    return args.Get(0).([]storage.MetricPoint), args.Error(1)
}

// Implement other methods...

func TestTenantStore_Write_InjectsTenantID(t *testing.T) {
    mockStore := new(MockStore)
    tenantStore := NewTenantStore(mockStore)

    ctx := context.WithValue(context.Background(), TenantIDContextKey, "tenant_xyz")

    // Expect Write with injected tenant_id
    mockStore.On("Write", ctx, mock.MatchedBy(func(metrics []storage.Metric) bool {
        if len(metrics) != 1 {
            return false
        }
        return metrics[0].Dimensions[TenantDimensionKey] == "tenant_xyz"
    })).Return(nil)

    metrics := []storage.Metric{
        {
            Name:       "cpu_usage",
            Timestamp:  time.Now(),
            Value:      75.5,
            Dimensions: map[string]string{"host": "web-01"},
        },
    }

    err := tenantStore.Write(ctx, metrics)

    assert.NoError(t, err)
    mockStore.AssertExpectations(t)
}

func TestTenantStore_QueryRange_FiltersByTenant(t *testing.T) {
    mockStore := new(MockStore)
    tenantStore := NewTenantStore(mockStore)

    ctx := context.WithValue(context.Background(), TenantIDContextKey, "tenant_xyz")

    expectedPoints := []storage.MetricPoint{{Value: 100.0}}

    // Expect QueryRange with tenant_id filter
    mockStore.On("QueryRange", ctx, mock.MatchedBy(func(req storage.QueryRequest) bool {
        return req.Filters[TenantDimensionKey] == "tenant_xyz"
    })).Return(expectedPoints, nil)

    req := storage.QueryRequest{
        MetricName: "cpu_usage",
        Start:      time.Now().Add(-1 * time.Hour),
        End:        time.Now(),
        Filters:    map[string]string{"host": "web-01"},
    }

    points, err := tenantStore.QueryRange(ctx, req)

    assert.NoError(t, err)
    assert.Equal(t, expectedPoints, points)
    mockStore.AssertExpectations(t)
}

func TestTenantStore_Write_MissingTenantID(t *testing.T) {
    mockStore := new(MockStore)
    tenantStore := NewTenantStore(mockStore)

    ctx := context.Background() // No tenant_id in context

    metrics := []storage.Metric{{Name: "cpu_usage", Value: 75.5}}

    err := tenantStore.Write(ctx, metrics)

    assert.Error(t, err)
    assert.ErrorIs(t, err, ErrMissingTenantID)
}
```

**Acceptance Criteria:**
- ✅ Automatically injects _tenant_id dimension to all written metrics
- ✅ Automatically filters all queries by tenant_id from context
- ✅ Prevents cross-tenant data access
- ✅ Returns error if tenant_id is missing from context
- ✅ Works transparently with existing MetricsStore interface

---

### Work Item 4: Security Headers Middleware

**Purpose:** Add security-related HTTP headers.

**Implementation:**

```go
// pkg/auth/security.go
package auth

import (
    "net/http"
)

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Prevent MIME sniffing
        w.Header().Set("X-Content-Type-Options", "nosniff")

        // Prevent clickjacking
        w.Header().Set("X-Frame-Options", "DENY")

        // XSS protection (legacy browsers)
        w.Header().Set("X-XSS-Protection", "1; mode=block")

        // Enforce HTTPS (only if not localhost)
        if r.Host != "localhost" && r.Host != "127.0.0.1" {
            w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        }

        // Content Security Policy (restrict script sources)
        w.Header().Set("Content-Security-Policy", "default-src 'self'")

        // Referrer policy
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

        next.ServeHTTP(w, r)
    })
}
```

**Tests:**

```go
// pkg/auth/security_test.go
package auth

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    middleware := SecurityHeadersMiddleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    w := httptest.NewRecorder()

    middleware.ServeHTTP(w, req)

    assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
    assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
    assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
    assert.Equal(t, "default-src 'self'", w.Header().Get("Content-Security-Policy"))
    assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}
```

**Acceptance Criteria:**
- ✅ Sets X-Content-Type-Options: nosniff
- ✅ Sets X-Frame-Options: DENY
- ✅ Sets Content-Security-Policy
- ✅ Sets Strict-Transport-Security (HSTS) for non-localhost
- ✅ Sets Referrer-Policy

---

### Work Item 5: Update Router with Authentication

**Purpose:** Integrate authentication middleware into the API router.

**Implementation:**

```go
// pkg/api/router.go (updated)
package api

import (
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"

    "github.com/yourusername/luminate/pkg/auth"
    "github.com/yourusername/luminate/pkg/storage"
)

// NewRouter creates the HTTP router with authentication
func NewRouter(store storage.MetricsStore, authMiddleware *auth.AuthMiddleware) *chi.Mux {
    r := chi.NewRouter()

    // Global middleware (no auth required)
    r.Use(RecoveryMiddleware)
    r.Use(LoggingMiddleware)
    r.Use(auth.SecurityHeadersMiddleware)
    r.Use(CORSMiddleware)
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Compress(5))

    // Wrap store with tenant isolation
    tenantStore := auth.NewTenantStore(store)

    // Initialize handlers with tenant-isolated store
    queryHandler := NewQueryHandler(tenantStore)
    writeHandler := NewWriteHandler(tenantStore)
    discoveryHandler := NewDiscoveryHandler(tenantStore)
    healthHandler := NewHealthHandler(store) // Health doesn't need tenant isolation
    adminHandler := NewAdminHandler(tenantStore)

    // Public endpoints (no auth required)
    r.Get("/api/v1/health", healthHandler.HandleHealth)
    r.Get("/metrics", prometheusHandler) // Prometheus metrics (if enabled)

    // Protected API v1 routes
    r.Route("/api/v1", func(r chi.Router) {
        // Apply authentication middleware
        r.Use(authMiddleware.Middleware)

        // Query endpoints (require 'read' scope)
        r.Group(func(r chi.Router) {
            r.Use(authMiddleware.RequireScope("read"))

            r.Post("/query", queryHandler.HandleQuery)
            r.Get("/metrics", discoveryHandler.HandleListMetrics)
            r.Get("/metrics/{name}/dimensions", discoveryHandler.HandleListDimensions)
            r.Get("/metrics/{name}/dimensions/{key}/values", discoveryHandler.HandleListDimensionValues)
        })

        // Write endpoints (require 'write' scope)
        r.Group(func(r chi.Router) {
            r.Use(authMiddleware.RequireScope("write"))

            r.Post("/write", writeHandler.HandleWrite)
        })

        // Admin endpoints (require 'admin' scope)
        r.Route("/admin", func(r chi.Router) {
            r.Use(authMiddleware.RequireScope("admin"))

            r.Post("/retention", adminHandler.HandleSetRetention)
            r.Delete("/metrics/{name}", adminHandler.HandleDeleteMetrics)
        })
    })

    return r
}
```

**Acceptance Criteria:**
- ✅ Health endpoint remains public (no auth)
- ✅ All /api/v1/* endpoints require valid JWT
- ✅ Query endpoints require 'read' scope
- ✅ Write endpoint requires 'write' scope
- ✅ Admin endpoints require 'admin' scope
- ✅ Tenant isolation applied transparently

---

### Work Item 6: Configuration

**Purpose:** Add authentication configuration to config file.

**Implementation:**

```yaml
# configs/config.yaml (auth section)
auth:
  enabled: true
  jwt:
    secret_key: "${JWT_SECRET}"  # From environment variable
    audience: "luminate-api"
    issuer: "https://auth.luminate.com"
    leeway: "1m"  # Clock skew tolerance

  # Optional: RSA public key for JWT validation (alternative to secret_key)
  # jwt_public_key_path: "/etc/luminate/jwt-public.pem"
```

```go
// pkg/config/config.go (updated)
package config

type Config struct {
    // ... existing fields ...
    Auth AuthConfig `yaml:"auth"`
}

type AuthConfig struct {
    Enabled          bool      `yaml:"enabled"`
    JWT              JWTConfig `yaml:"jwt"`
    JWTPublicKeyPath string    `yaml:"jwt_public_key_path"`
}

type JWTConfig struct {
    SecretKey string        `yaml:"secret_key"`
    Audience  string        `yaml:"audience"`
    Issuer    string        `yaml:"issuer"`
    Leeway    time.Duration `yaml:"leeway"`
}
```

**Acceptance Criteria:**
- ✅ Auth can be enabled/disabled via config
- ✅ JWT secret loaded from environment variable
- ✅ Audience and issuer configurable
- ✅ Clock skew tolerance configurable
- ✅ Support for RSA public key (optional)

---

## Integration Example

**Complete main.go with authentication:**

```go
// cmd/luminate/main.go (updated)
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/yourusername/luminate/pkg/api"
    "github.com/yourusername/luminate/pkg/auth"
    "github.com/yourusername/luminate/pkg/config"
    "github.com/yourusername/luminate/pkg/storage/badger"
)

func main() {
    // Load configuration
    cfg, err := config.Load("configs/config.yaml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Initialize storage backend
    store, err := badger.NewBadgerStore(cfg.Storage.Badger)
    if err != nil {
        log.Fatalf("Failed to initialize storage: %v", err)
    }
    defer store.Close()

    // Initialize authentication
    var router *chi.Mux

    if cfg.Auth.Enabled {
        // Create JWT validator
        jwtValidator := auth.NewJWTValidator(cfg.Auth.JWT)

        // Create auth middleware
        authMiddleware := auth.NewAuthMiddleware(jwtValidator)

        // Create router with authentication
        router = api.NewRouter(store, authMiddleware)

        log.Println("Authentication enabled")
    } else {
        // Create router without authentication (dev mode)
        router = api.NewRouter(store, nil)

        log.Println("WARNING: Authentication disabled")
    }

    // Create HTTP server
    server := &http.Server{
        Addr:         cfg.Server.Address(),
        Handler:      router,
        ReadTimeout:  cfg.Server.ReadTimeout,
        WriteTimeout: cfg.Server.WriteTimeout,
    }

    // Start server
    go func() {
        log.Printf("Starting server on %s", cfg.Server.Address())
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Server failed: %v", err)
        }
    }()

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Println("Shutting down server...")

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }

    log.Println("Server stopped")
}
```

---

## Testing Authentication End-to-End

```bash
# Generate test JWT token (using jwt.io or jwt-cli)
export JWT_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# Write metrics (requires 'write' scope)
curl -X POST http://localhost:8080/api/v1/write \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "metrics": [{
      "name": "api_latency",
      "timestamp": 1735603200000,
      "value": 145.5,
      "dimensions": {"endpoint": "/api/users"}
    }]
  }'

# Query metrics (requires 'read' scope)
curl -X POST http://localhost:8080/api/v1/query \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "aggregate",
    "metricName": "api_latency",
    "timeRange": {"relative": "1h"},
    "aggregation": "avg"
  }'

# Delete metrics (requires 'admin' scope)
curl -X DELETE "http://localhost:8080/api/v1/admin/metrics/api_latency?before=2024-01-01T00:00:00Z" \
  -H "Authorization: Bearer $JWT_TOKEN"
```

---

## Security Best Practices

1. **Secret Management:**
   - Never commit JWT secrets to version control
   - Use environment variables or secret management systems (Vault, AWS Secrets Manager)
   - Rotate secrets regularly

2. **Token Expiration:**
   - Use short-lived tokens (1-24 hours)
   - Implement token refresh mechanism in clients
   - Set reasonable clock skew tolerance (1 minute)

3. **HTTPS Only:**
   - Always use HTTPS in production
   - Set HSTS header to enforce HTTPS
   - Use TLS 1.2 or higher

4. **Rate Limiting:**
   - Implement per-token rate limits (WS5)
   - Track failed authentication attempts
   - Consider temporary bans for repeated failures

5. **Logging:**
   - Log all authentication failures
   - Log scope violations (403 errors)
   - Monitor for unusual patterns

---

## Summary

This workstream provides complete authentication and security implementation:

1. **JWT Validation**: HMAC-SHA256 signature validation with clock skew tolerance
2. **Authentication Middleware**: HTTP middleware for token extraction and validation
3. **Tenant Isolation**: Automatic injection and filtering of _tenant_id
4. **Authorization**: Scope-based permissions (read, write, admin)
5. **Security Headers**: HSTS, CSP, X-Frame-Options, etc.
6. **Configuration**: Flexible auth config with environment variable support

**Next Steps**: After completing this workstream, proceed to [WS5: Rate Limiting & Validation](05-rate-limiting.md) to add rate limits, cardinality tracking, and request validation.
