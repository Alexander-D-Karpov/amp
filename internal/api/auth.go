package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// AuthResponse represents the response from authentication endpoints
type AuthResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// User represents user information from the API
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// LoginRequest represents a login request payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenRequest represents a token validation request
type TokenRequest struct {
	Token string `json:"token"`
}

// Login authenticates a user with username and password
func (c *Client) Login(ctx context.Context, username, password string) (*AuthResponse, error) {
	loginReq := LoginRequest{
		Username: username,
		Password: password,
	}

	resp, _, err := c.makeRequest(ctx, "POST", "/auth/login/", nil, loginReq)
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Failed to close response body: %v", closeErr)
		}
	}()

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("decode auth response: %w", err)
	}

	c.token = authResp.Token
	return &authResp, nil
}

// ValidateToken validates an authentication token
func (c *Client) ValidateToken(ctx context.Context, token string) (*User, error) {
	oldToken := c.token
	c.token = token

	resp, _, err := c.makeRequest(ctx, "GET", "/users/self/", nil, nil)
	if err != nil {
		c.token = oldToken
		return nil, fmt.Errorf("validate token: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		c.token = oldToken
		return nil, fmt.Errorf("invalid token")
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		c.token = oldToken
		return nil, fmt.Errorf("decode user response: %w", err)
	}

	return &user, nil
}

// RefreshToken refreshes an existing authentication token
func (c *Client) RefreshToken(ctx context.Context) (*AuthResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("no token to refresh")
	}

	resp, _, err := c.makeRequest(ctx, "POST", "/auth/refresh/", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Failed to close response body: %v", closeErr)
		}
	}()

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}

	c.token = authResp.Token
	return &authResp, nil
}

// Logout logs out the current user session
func (c *Client) Logout(ctx context.Context) error {
	c.debugLog("Logging out...")

	if c.token != "" && !c.isAnonymous {
		_, _, err := c.makeRequest(ctx, "POST", "/auth/logout/", nil, nil)
		if err != nil {
			c.debugLog("Logout request failed: %v", err)
		}
	}

	c.token = ""
	c.isAnonymous = true
	c.debugLog("Logged out successfully")

	return nil
}

// SetToken sets the authentication token
func (c *Client) SetToken(token string) {
	oldToken := c.token
	c.token = token

	if token == "" {
		c.isAnonymous = true
		c.debugLog("Token cleared - switching to anonymous mode")
	} else {
		c.isAnonymous = false
		c.debugLog("Token updated: %s... (was: %s...)",
			token[:min(len(token), 10)],
			oldToken[:min(len(oldToken), 10)])
	}
}

// GetToken returns the current authentication token
func (c *Client) GetToken() string {
	return c.token
}
