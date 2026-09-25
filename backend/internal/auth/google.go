package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"visora-backend/internal/db"
)

// ─── Config ───────────────────────────────────────────────────────────────────

func googleOAuthConfig() *oauth2.Config {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	apiURL := os.Getenv("API_BASE_EXTERNAL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	return &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  apiURL + "/auth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

var jwtSecret = func() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "visora_jwt_secret_change_in_prod"
	}
	return []byte(s)
}()

// ─── Google OAuth flow ────────────────────────────────────────────────────────

// GET /auth/google  →  redirect to Google consent screen
func GoogleLogin(c *gin.Context) {
	if err := VerifyTurnstile(c.Query("turnstile"), c.ClientIP()); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "human verification failed"})
		return
	}

	// random state to prevent CSRF
	b := make([]byte, 16)
	rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)

	// Remember where to redirect after login (default: frontend /start)
	redirectAfter := c.Query("redirect")
	if redirectAfter == "" {
		frontendURL := os.Getenv("FRONTEND_URL")
		if frontendURL == "" {
			frontendURL = "http://localhost:3000"
		}
		redirectAfter = frontendURL + "/start"
	}

	// Encode state + redirect destination together
	statePayload := state + "|" + redirectAfter

	setAuthCookie(c, "oauth_state", statePayload, 600)

	url := googleOAuthConfig().AuthCodeURL(
		statePayload,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("prompt", "select_account"),
	)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GET /auth/google/callback  →  exchange code, issue JWT, redirect to frontend
func GoogleCallback(c *gin.Context) {
	// Verify state
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie != c.Query("state") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid oauth state"})
		return
	}
	setAuthCookie(c, "oauth_state", "", -1)

	// Parse redirect destination from state payload
	redirectAfter := os.Getenv("FRONTEND_URL") + "/start"
	if len(stateCookie) > 24 { // 24 = base64(16 bytes) length
		redirectAfter = stateCookie[23:] // everything after the state|
		// Find the pipe separator
		for i, ch := range stateCookie {
			if ch == '|' {
				redirectAfter = stateCookie[i+1:]
				break
			}
		}
	}

	// Exchange code for token
	cfg := googleOAuthConfig()
	token, err := cfg.Exchange(context.Background(), c.Query("code"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange token: " + err.Error()})
		return
	}

	// Fetch Google user info
	client := cfg.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch user info"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var userInfo struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &userInfo); err != nil || userInfo.Email == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user info from Google"})
		return
	}

	// Upsert user in DB
	var user db.User
	db.DB.Where("email = ?", userInfo.Email).First(&user)
	if user.ID == 0 {
		user = db.User{Email: userInfo.Email, Name: userInfo.Name}
		db.DB.Create(&user)
	}

	// Issue JWT
	jwtToken, err := issueJWT(userInfo.Email, userInfo.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	setAuthCookie(c, "visora_token", jwtToken, 60*60*24*30)

	c.Redirect(http.StatusTemporaryRedirect, redirectAfter)
}

// GET /auth/logout — browser must hit this host (the one that set visora_token)
// so Set-Cookie expire actually reaches the client.
func Logout(c *gin.Context) {
	expireAuthCookies(c)

	frontendURL := strings.TrimRight(os.Getenv("FRONTEND_URL"), "/")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
	c.Redirect(http.StatusFound, frontendURL+"/?logged_out=1")
}

// GET /auth/me  →  returns logged-in user info (called by frontend on load)
func Me(c *gin.Context) {
	claims, ok := c.Get("auth_claims")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.JSON(http.StatusOK, claims)
}

func cookieDomain() string {
	return strings.TrimPrefix(strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")), ".")
}

func cookieSecure() bool {
	front := strings.ToLower(os.Getenv("FRONTEND_URL"))
	api := strings.ToLower(os.Getenv("API_BASE_EXTERNAL"))
	return strings.HasPrefix(front, "https://") || strings.HasPrefix(api, "https://")
}

func parentCookieDomains() []string {
	seen := map[string]bool{"": true}
	out := []string{""}
	add := func(d string) {
		d = strings.TrimPrefix(strings.TrimSpace(d), ".")
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		out = append(out, d, "."+d)
	}
	add(cookieDomain())
	for _, raw := range []string{os.Getenv("FRONTEND_URL"), os.Getenv("API_BASE_EXTERNAL")} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		host := strings.TrimPrefix(u.Hostname(), "www.")
		add(host)
		parts := strings.Split(host, ".")
		for i := 1; i < len(parts)-1; i++ {
			add(strings.Join(parts[i:], "."))
		}
	}
	return out
}

func writeCookie(c *gin.Context, name, value, domain string, maxAge int, secure bool) {
	expires := time.Time{}
	if maxAge < 0 {
		expires = time.Unix(0, 0)
		maxAge = -1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		MaxAge:   maxAge,
		Expires:  expires,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func setAuthCookie(c *gin.Context, name, value string, maxAge int) {
	secure := cookieSecure()
	if maxAge < 0 {
		expireNamedCookie(c, name)
		return
	}
	writeCookie(c, name, value, cookieDomain(), maxAge, secure)
}

func expireNamedCookie(c *gin.Context, name string) {
	for _, domain := range parentCookieDomains() {
		writeCookie(c, name, "", domain, -1, true)
		writeCookie(c, name, "", domain, -1, false)
	}
}

func expireAuthCookies(c *gin.Context) {
	expireNamedCookie(c, "visora_token")
	expireNamedCookie(c, "oauth_state")
}

// ─── JWT helpers ──────────────────────────────────────────────────────────────

type Claims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

func issueJWT(email, name string) (string, error) {
	claims := Claims{
		Email: email,
		Name:  name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "visora",
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(jwtSecret)
}

// ValidateJWT parses and validates a JWT string, returns claims on success.
func ValidateJWT(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// ─── Gin middleware ───────────────────────────────────────────────────────────

// AuthMiddleware reads visora_token cookie and injects claims into context.
// On failure it just sets email to "" — individual handlers decide whether to reject.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := c.Cookie("visora_token")
		if err == nil && tokenStr != "" {
			if claims, err := ValidateJWT(tokenStr); err == nil {
				c.Set("auth_claims", claims)
				c.Set("user_email", claims.Email)
				c.Request.Header.Set("X-User-Email", claims.Email)
			}
		}
		c.Next()
	}
}

// RequireAuth rejects requests without a valid token.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get("user_email")
		if !exists || c.GetString("user_email") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Next()
	}
}
