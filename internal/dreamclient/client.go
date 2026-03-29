package dreamclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is an HTTP client for the DreamDict dictionary service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a new DreamDict client.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// Definition is a single definition entry from DreamDict.
type Definition struct {
	POS      string `json:"pos"`
	Gloss    string `json:"gloss"`
	Source   string `json:"source"`
	Priority int    `json:"priority"`
}

// HealthResponse is the response from /health.
type HealthResponse struct {
	Status        string         `json:"status"`
	DBPath        string         `json:"db_path"`
	WordCounts    map[string]int `json:"word_counts"`
	DefCounts     map[string]int `json:"def_counts"`
	ImportedAt    string         `json:"imported_at"`
	SchemaVersion string         `json:"schema_version"`
}

// IsValidWord checks if a word exists in a language's word list.
func (c *Client) IsValidWord(word, lang string) (bool, error) {
	u := c.baseURL + "/valid?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return false, fmt.Errorf("dreamdict: valid: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("dreamdict: valid: status %d", resp.StatusCode)
	}

	var result struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("dreamdict: valid: decode: %w", err)
	}
	return result.Valid, nil
}

// RandomWord returns a random word for a language with optional filters.
// pos can be empty. min/max of 0 means no filter.
func (c *Client) RandomWord(lang, pos string, min, max int) (string, error) {
	params := url.Values{"lang": {lang}}
	if pos != "" {
		params.Set("pos", pos)
	}
	if min > 0 {
		params.Set("min", strconv.Itoa(min))
	}
	if max > 0 {
		params.Set("max", strconv.Itoa(max))
	}

	u := c.baseURL + "/random?" + params.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return "", fmt.Errorf("dreamdict: random: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("dreamdict: random: no match")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dreamdict: random: status %d", resp.StatusCode)
	}

	var result struct {
		Word string `json:"word"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("dreamdict: random: decode: %w", err)
	}
	return result.Word, nil
}

// Define returns all definitions for a word in a language.
func (c *Client) Define(word, lang string) ([]Definition, error) {
	u := c.baseURL + "/define?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: define: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: define: status %d", resp.StatusCode)
	}

	var result struct {
		Definitions []Definition `json:"definitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: define: decode: %w", err)
	}
	return result.Definitions, nil
}

// Synonyms returns synonyms for a word in a language.
func (c *Client) Synonyms(word, lang string) ([]string, error) {
	u := c.baseURL + "/synonyms?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: synonyms: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: synonyms: status %d", resp.StatusCode)
	}

	var result struct {
		Synonyms []string `json:"synonyms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: synonyms: decode: %w", err)
	}
	return result.Synonyms, nil
}

// Translate returns translations of a word from one language to another.
func (c *Client) Translate(word, from, to string) ([]string, error) {
	u := c.baseURL + "/translate?" + url.Values{"word": {word}, "from": {from}, "to": {to}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: translate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: translate: status %d", resp.StatusCode)
	}

	var result struct {
		Translations []string `json:"translations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: translate: decode: %w", err)
	}
	return result.Translations, nil
}

// Health checks if the DreamDict service is reachable and returns stats.
func (c *Client) Health() (*HealthResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return nil, fmt.Errorf("dreamdict: health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: health: status %d", resp.StatusCode)
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: health: decode: %w", err)
	}
	return &result, nil
}
