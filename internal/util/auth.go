package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LoginResponse holds the Matrix login response fields we care about.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
	UserID      string `json:"user_id"`
}

// LoginWithPassword performs a Matrix password login.
func LoginWithPassword(homeserver, user, password, deviceDisplayName string) (*LoginResponse, error) {
	url := homeserver + "/_matrix/client/v3/login"

	body := map[string]any{
		"type": "m.login.password",
		"identifier": map[string]string{
			"type": "m.id.user",
			"user": user,
		},
		"password":                password,
		"initial_device_display_name": deviceDisplayName,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal login body: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("login failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result LoginResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse login response: %w", err)
	}
	return &result, nil
}

// IsTokenValid checks if an access token is still valid via /account/whoami.
func IsTokenValid(homeserver, accessToken string) (bool, string) {
	url := homeserver + "/_matrix/client/v3/account/whoami"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, ""
	}

	var result struct {
		UserID string `json:"user_id"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return false, ""
	}
	return true, result.UserID
}
