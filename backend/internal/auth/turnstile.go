package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type turnstileResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// VerifyTurnstile checks a widget token with Cloudflare. Fail closed when secret is set.
func VerifyTurnstile(token, remoteIP string) error {
	secret := strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY"))
	if secret == "" {
		return fmt.Errorf("turnstile is not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("turnstile token missing")
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", form)
	if err != nil {
		return fmt.Errorf("turnstile request failed: %w", err)
	}
	defer res.Body.Close()

	var parsed turnstileResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("turnstile decode failed: %w", err)
	}
	if !parsed.Success {
		if len(parsed.ErrorCodes) > 0 {
			return fmt.Errorf("turnstile rejected: %s", strings.Join(parsed.ErrorCodes, ", "))
		}
		return fmt.Errorf("turnstile rejected")
	}
	return nil
}
