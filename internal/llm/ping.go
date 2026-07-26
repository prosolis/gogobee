package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// pingTimeout keeps a liveness probe short — /botinfo renders synchronously and
// a hung endpoint must not hold the reply.
const pingTimeout = 5 * time.Second

func getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
