package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const oAuthPrefix = "gho_"

type User struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Login      string `json:"login"`
	Allowed    bool   `json:"allowed"`
	JoinedDate string `json:"created_at"`
}

type Authenticator interface {
	Authenticate(token string) (User, error)
}

type github struct {
	userEndpoint string
	client       *http.Client
	cache        sync.Map // token -> *cachedAuth
	posTTL       time.Duration
	negTTL       time.Duration
}

type cachedAuth struct {
	user      User
	err       error
	expiresAt time.Time
}

// devAuth is a development authenticator that bypasses all auth checks.
type devAuth struct{}

func (d *devAuth) Authenticate(token string) (User, error) {
	return User{ID: 1, Name: "Dev User", Login: "dev", Allowed: true}, nil
}

func New() Authenticator {
	if os.Getenv("XPOS_DEV_NO_AUTH") == "1" {
		return &devAuth{}
	}
	return &github{
		userEndpoint: "https://api.github.com/user",
		client:       &http.Client{Timeout: 5 * time.Second},
		posTTL:       5 * time.Minute,
		negTTL:       30 * time.Second,
	}
}

func (g *github) Authenticate(token string) (User, error) {
	if token == "" {
		return User{}, errors.New("empty auth token")
	}
	if v, ok := g.cache.Load(token); ok {
		c := v.(*cachedAuth)
		if time.Now().Before(c.expiresAt) {
			return c.user, c.err
		}
		g.cache.Delete(token)
	}
	user, err := g.authenticate(g.userEndpoint, token)
	ttl := g.posTTL
	if err != nil {
		ttl = g.negTTL
	}
	g.cache.Store(token, &cachedAuth{
		user:      user,
		err:       err,
		expiresAt: time.Now().Add(ttl),
	})
	return user, err
}

func (g *github) authenticate(endpoint, token string) (User, error) {
	user := User{}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return user, fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s%s", oAuthPrefix, token))

	resp, err := g.client.Do(req)
	if err != nil {
		return user, fmt.Errorf("authentication request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return user, fmt.Errorf("invalid token (status %d)", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return user, fmt.Errorf("failed to decode user data: %w", err)
	}
	user.Login = strings.ToLower(user.Login)
	return user, nil
}
