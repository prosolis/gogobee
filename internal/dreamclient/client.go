package dreamclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
// pos can be empty. min/max of 0 means no filter. minFreq of 0 means no
// frequency filter; values > 0 pass min_freq to the server and also veto
// results that come back below the threshold (belt-and-suspenders).
func (c *Client) RandomWord(lang, pos string, min, max, minFreq int) (string, error) {
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
	if minFreq > 0 {
		params.Set("min_freq", strconv.Itoa(minFreq))
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
		Word      string `json:"word"`
		Frequency int    `json:"frequency"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("dreamdict: random: decode: %w", err)
	}

	// Client-side veto in case the server doesn't support min_freq yet.
	if minFreq > 0 && result.Frequency > 0 && result.Frequency < minFreq {
		return "", fmt.Errorf("dreamdict: random: word below frequency threshold")
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

// Frequency returns the frequency score for a word in a language.
// Higher values indicate more common words. Returns 0 if unknown.
func (c *Client) Frequency(word, lang string) (int, error) {
	u := c.baseURL + "/frequency?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return 0, fmt.Errorf("dreamdict: frequency: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("dreamdict: frequency: status %d", resp.StatusCode)
	}

	var result struct {
		Frequency int `json:"frequency"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("dreamdict: frequency: decode: %w", err)
	}
	return result.Frequency, nil
}

// FrequencyBatch returns frequency scores for multiple words in a single request.
func (c *Client) FrequencyBatch(words []string, lang string) (map[string]int, error) {
	u := c.baseURL + "/frequency/batch?" + url.Values{
		"words": {strings.Join(words, ",")},
		"lang":  {lang},
	}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: frequency batch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: frequency batch: status %d", resp.StatusCode)
	}

	var result struct {
		Frequencies map[string]int `json:"frequencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: frequency batch: decode: %w", err)
	}
	return result.Frequencies, nil
}

// Antonyms returns antonyms for a word in a language.
func (c *Client) Antonyms(word, lang string) ([]string, error) {
	u := c.baseURL + "/antonyms?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: antonyms: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: antonyms: status %d", resp.StatusCode)
	}

	var result struct {
		Antonyms []string `json:"antonyms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: antonyms: decode: %w", err)
	}
	return result.Antonyms, nil
}

// Pronunciation holds pronunciation data for a word.
type Pronunciation struct {
	Format string `json:"format"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Pronunciations returns pronunciation data for a word in a language.
func (c *Client) Pronunciations(word, lang string) ([]Pronunciation, error) {
	u := c.baseURL + "/pronunciation?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: pronunciation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: pronunciation: status %d", resp.StatusCode)
	}

	var result struct {
		Pronunciations []Pronunciation `json:"pronunciations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: pronunciation: decode: %w", err)
	}
	return result.Pronunciations, nil
}

// Etymology returns the etymology text for a word in a language.
func (c *Client) Etymology(word, lang string) (string, error) {
	u := c.baseURL + "/etymology?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return "", fmt.Errorf("dreamdict: etymology: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dreamdict: etymology: status %d", resp.StatusCode)
	}

	var result struct {
		Etymology string `json:"etymology"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("dreamdict: etymology: decode: %w", err)
	}
	return result.Etymology, nil
}

// Difficulty returns the difficulty score (0.0-1.0) for a word in a language.
// Returns -1 if the word has no difficulty score.
func (c *Client) Difficulty(word, lang string) (float64, error) {
	u := c.baseURL + "/difficulty?" + url.Values{"word": {word}, "lang": {lang}}.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return 0, fmt.Errorf("dreamdict: difficulty: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("dreamdict: difficulty: status %d", resp.StatusCode)
	}

	var result struct {
		Difficulty float64 `json:"difficulty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("dreamdict: difficulty: decode: %w", err)
	}
	return result.Difficulty, nil
}

// Rhymes returns words that rhyme with the given word (English only).
func (c *Client) Rhymes(word string, limit int) ([]string, error) {
	params := url.Values{"word": {word}}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	u := c.baseURL + "/rhyme?" + params.Encode()
	resp, err := c.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("dreamdict: rhyme: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dreamdict: rhyme: status %d", resp.StatusCode)
	}

	var result struct {
		Rhymes []string `json:"rhymes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dreamdict: rhyme: decode: %w", err)
	}
	return result.Rhymes, nil
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
