package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
}

func New() Authenticator {
	return github{
		userEndpoint: "https://api.github.com/user",
	}
}

func (g github) Authenticate(token string) (User, error) {
	return g.authenticate(g.userEndpoint, token)
}

func (g github) authenticate(endpoint, token string) (User, error) {
	user := User{}
	client := &http.Client{}

	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", fmt.Sprintf("token %s%s", oAuthPrefix, token))
	resp, err := client.Do(req)

	if err != nil {
		return user, fmt.Errorf("authentication request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return user, fmt.Errorf("invalid token %v", token)
	}
	err = json.NewDecoder(resp.Body).Decode(&user)
	if err != nil {
		return user, fmt.Errorf("failed to decode user data: %v", err)
	}
	user.Login = strings.ToLower(user.Login)
	return user, nil
}
