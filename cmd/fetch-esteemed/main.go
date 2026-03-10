package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Entry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ImageURL    string `json:"image_url"`
	SearchQuery string `json:"search_query,omitempty"`
}

type WikiResponse struct {
	Query struct {
		Pages map[string]struct {
			Thumbnail struct {
				Source string `json:"source"`
			} `json:"thumbnail"`
		} `json:"pages"`
	} `json:"query"`
}

type SerpAPIResponse struct {
	ImagesResults []struct {
		Original  string `json:"original"`
		Thumbnail string `json:"thumbnail"`
	} `json:"images_results"`
}

func main() {
	const (
		esteemedPath = "esteemed.json"
		outputDir    = "data/esteemed"
		requestDelay = 500 * time.Millisecond
	)

	_ = godotenv.Load()
	serpAPIKey := os.Getenv("SERPAPI_KEY")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Read esteemed.json
	data, err := os.ReadFile(esteemedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read %s: %v\n", esteemedPath, err)
		os.Exit(1)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse JSON: %v\n", err)
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	if serpAPIKey != "" {
		fmt.Println("SerpAPI key found — will use as fallback for missing Wikipedia images")
	} else {
		fmt.Println("No SERPAPI_KEY set — Wikipedia only")
	}

	fmt.Printf("Processing %d entries...\n\n", len(entries))

	downloaded := 0
	skipped := 0
	failed := 0

	for i, entry := range entries {
		outPath := filepath.Join(outputDir, entry.ID+".jpg")

		// Skip if already downloaded
		if _, err := os.Stat(outPath); err == nil {
			fmt.Printf("[%d/%d] SKIP (exists): %s\n", i+1, len(entries), entry.Name)
			skipped++
			continue
		}

		if entry.ImageURL == "" {
			fmt.Printf("[%d/%d] SKIP (no URL): %s\n", i+1, len(entries), entry.Name)
			skipped++
			continue
		}

		var imageURL string

		// Try Wikipedia first
		if isWikipediaURL(entry.ImageURL) {
			pageName := extractPageName(entry.ImageURL)
			if pageName != "" {
				var err error
				imageURL, err = fetchWikipediaImage(client, pageName)
				if err != nil {
					fmt.Printf("[%d/%d] WARN (wiki API): %s - %v\n", i+1, len(entries), entry.Name, err)
				}
			}
		} else {
			imageURL = entry.ImageURL
		}

		// Fallback to SerpAPI if no Wikipedia image
		if imageURL == "" && serpAPIKey != "" {
			// Use explicit search_query if provided, else derive from Wikipedia page name
			searchQuery := entry.Name
			if entry.SearchQuery != "" {
				searchQuery = entry.SearchQuery
			} else if isWikipediaURL(entry.ImageURL) {
				if pn := extractPageName(entry.ImageURL); pn != "" {
					q := strings.ReplaceAll(pn, "_", " ")
					q = strings.ReplaceAll(q, "(", "")
					q = strings.ReplaceAll(q, ")", "")
					q = strings.TrimSpace(q)
					if q != "" {
						searchQuery = q
					}
				}
			}
			var err error
			imageURL, err = fetchSerpAPIImage(client, serpAPIKey, searchQuery)
			if err != nil {
				fmt.Printf("[%d/%d] WARN (serpapi): %s - %v\n", i+1, len(entries), entry.Name, err)
			}
			if imageURL != "" {
				fmt.Printf("[%d/%d] SerpAPI fallback: %s\n", i+1, len(entries), entry.Name)
			}
		}

		if imageURL == "" {
			fmt.Printf("[%d/%d] FAIL (no image found): %s\n", i+1, len(entries), entry.Name)
			failed++
			time.Sleep(requestDelay)
			continue
		}

		if err := downloadImage(client, imageURL, outPath); err != nil {
			fmt.Printf("[%d/%d] FAIL (download): %s - %v\n", i+1, len(entries), entry.Name, err)
			failed++
		} else {
			// Convert non-JPEG files (WebP, PNG, etc.) to JPEG via ImageMagick
			convertToJPEG(outPath)
			fmt.Printf("[%d/%d] OK: %s\n", i+1, len(entries), entry.Name)
			downloaded++
		}

		time.Sleep(requestDelay)
	}

	fmt.Printf("\nDone. Downloaded: %d, Skipped: %d, Failed: %d\n", downloaded, skipped, failed)
}

func isWikipediaURL(rawURL string) bool {
	return strings.Contains(rawURL, "wikipedia.org/wiki/")
}

func extractPageName(wikiURL string) string {
	parsed, err := url.Parse(wikiURL)
	if err != nil {
		return ""
	}
	const prefix = "/wiki/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return ""
	}
	return parsed.Path[len(prefix):]
}

func fetchWikipediaImage(client *http.Client, pageName string) (string, error) {
	apiURL := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&titles=%s&prop=pageimages&format=json&pithumbsize=400",
		url.PathEscape(pageName),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "gogobee-fetch-esteemed/1.0 (bot; educational project)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var wikiResp WikiResponse
	if err := json.NewDecoder(resp.Body).Decode(&wikiResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	for _, page := range wikiResp.Query.Pages {
		if page.Thumbnail.Source != "" {
			return page.Thumbnail.Source, nil
		}
	}

	return "", nil
}

func fetchSerpAPIImage(client *http.Client, apiKey, query string) (string, error) {
	params := url.Values{
		"engine":  {"google_images"},
		"q":       {query},
		"api_key": {apiKey},
		"num":     {"1"},
	}
	apiURL := "https://serpapi.com/search.json?" + params.Encode()

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var serpResp SerpAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&serpResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if len(serpResp.ImagesResults) == 0 {
		return "", nil
	}

	// Prefer the original full-size image, fall back to thumbnail
	result := serpResp.ImagesResults[0]
	if result.Original != "" {
		return result.Original, nil
	}
	return result.Thumbnail, nil
}

func downloadImage(client *http.Client, imageURL, outPath string) error {
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "gogobee-fetch-esteemed/1.0 (bot; educational project)")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Write to a temp file first, then rename for atomicity
	tmpPath := outPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing file: %w", err)
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming file: %w", err)
	}

	return nil
}

// convertToJPEG checks if the file is actually a JPEG; if not (e.g. WebP, PNG),
// it converts it to JPEG via ImageMagick, flattening any alpha channel.
func convertToJPEG(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}

	// Read first 3 bytes to check for JPEG magic bytes (FF D8 FF)
	header := make([]byte, 3)
	n, _ := f.Read(header)
	f.Close()

	if n >= 3 && header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return // already JPEG
	}

	// Convert via ImageMagick
	tmpPath := path + ".conv.jpg"
	cmd := exec.Command("convert", path, "-background", "white", "-flatten", tmpPath)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  convert failed for %s: %v\n", filepath.Base(path), err)
		os.Remove(tmpPath)
		return
	}

	if err := os.Rename(tmpPath, path); err != nil {
		fmt.Fprintf(os.Stderr, "  rename failed for %s: %v\n", filepath.Base(path), err)
		os.Remove(tmpPath)
		return
	}

	fmt.Printf("  converted %s to JPEG\n", filepath.Base(path))
}
