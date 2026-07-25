package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"gogobee/internal/peteclient"
)

// LLM-authored adventure dispatches. gogobee owns the raw model compute; this is
// where a structured fact becomes warm-reporter prose for Pete to publish. Pete
// is still the editor and the safety boundary: it runs its own prose-guard over
// whatever we send and falls back to its templates on anything it does not like,
// so authoring here is best-effort by design — every failure path returns an
// empty pair and Pete templates the fact.
//
// The voice must live somewhere, and with no route for Pete to call back into
// this box (see roster.go in the Pete repo) it lives in the prompt below. Keep
// it faithful to pete_adventure_news_voice.md; Pete's persona, not gogobee's.

// dispatchLLMTimeout bounds the authoring call. emitFact runs on game-event
// chokepoints (a party wipe fires one per member), so this is deliberately far
// tighter than the interactive 120s tip budget: if the model cannot turn a
// handful of facts into two sentences this fast, it is effectively down, and a
// template dispatch now beats a voiced one late.
const dispatchLLMTimeout = 15 * time.Second

// Length ceilings, mirrored from Pete's proseGuard so we never ship prose Pete
// will reject for length alone. Byte counts, matching Pete's len() check.
const (
	maxDispatchHeadline = 200
	maxDispatchLede     = 800
)

var dispatchHTTP = &http.Client{Timeout: dispatchLLMTimeout}

// authorDispatch turns a fact into a headline+lede in Pete's voice, or returns
// two empty strings if the model is unconfigured, errors, times out, or produces
// anything malformed. The fact must already have its FINAL Actors set (post
// opt-out anonymisation) — that list is the only set of names the prose may use,
// and it is what Pete's guard checks the output against.
func authorDispatch(f peteclient.Fact) (headline, lede string) {
	host := os.Getenv("OLLAMA_HOST")
	model := os.Getenv("OLLAMA_MODEL")
	if host == "" || model == "" {
		return "", ""
	}

	prompt := buildDispatchPrompt(f)
	raw, err := callOllamaDispatch(dispatchHTTP, host, model, prompt)
	if err != nil {
		slog.Warn("pete dispatch: LLM authoring failed, Pete will template", "guid", f.GUID, "err", err)
		return "", ""
	}

	h, l, ok := parseDispatch(raw)
	if !ok {
		slog.Warn("pete dispatch: unparseable LLM output, Pete will template", "guid", f.GUID)
		return "", ""
	}
	// Ship only a complete, in-bounds pair. A half-authored dispatch or an
	// over-long one is exactly what Pete would reject anyway; catch it here so a
	// bad generation costs nothing on the wire.
	if h == "" || l == "" || len(h) > maxDispatchHeadline || len(l) > maxDispatchLede {
		slog.Warn("pete dispatch: LLM output empty or over length, Pete will template",
			"guid", f.GUID, "headline_len", len(h), "lede_len", len(l))
		return "", ""
	}
	return h, l
}

// buildDispatchPrompt renders the persona, the strict rules, and this fact's
// structured facts into a single prompt. The facts block lists only the fields
// that are set, each labelled, so the model has the who/what/where and no room
// to invent the rest.
func buildDispatchPrompt(f peteclient.Fact) string {
	var facts strings.Builder
	add := func(label, val string) {
		if val != "" {
			fmt.Fprintf(&facts, "- %s: %s\n", label, val)
		}
	}
	addN := func(label string, n int) {
		if n != 0 {
			fmt.Fprintf(&facts, "- %s: %d\n", label, n)
		}
	}
	add("event", f.EventType)
	add("who this is about (the subject)", f.Subject)
	add("the other person named", f.Opponent)
	add("monster or boss", f.Boss)
	add("dungeon or zone", f.Zone)
	add("region", f.Region)
	addN("character level", f.Level)
	addN("count", f.Count)
	add("outcome", f.Outcome)
	add("stakes or item", f.Stakes)
	add("class and race", f.ClassRace)
	add("milestone", f.Milestone)

	names := "(none — this is a realm-level event with no named adventurer)"
	if len(f.Actors) > 0 {
		names = strings.Join(f.Actors, ", ")
	}

	return fmt.Sprintf(`You are Pete, a warm, friendly local news reporter for a fantasy adventuring town. Think a beloved local newscaster who genuinely knows everyone and is glad to see them. You have journalistic bones — a clear headline and a who/what/where lede that gets the facts right — delivered with personable, first-person warmth. You root for the community, celebrate wins, mourn losses gently, welcome newcomers. Conversational, never snarky, never a caps-lock hype-man. Warmth carries the register, not exclamation marks.

Write a short news dispatch about the event below.

STRICT RULES — do not violate these:
- Use ONLY these adventurer names, exactly as written: %s. Never invent a name, never use any other person's name, never use an @-handle.
- Use ONLY the facts listed. Do not invent numbers, outcomes, items, or events that are not below.
- The monster/boss, zone, region and item names are game names — you may use them as given.
- Do not address the reader as "you" unless the event is Pete's own duel.
- No markdown, no emoji, no quotation marks around the whole thing.

Respond with ONLY a JSON object, no other text:
{"headline": "one short sentence, a real headline", "lede": "one to three warm sentences with the who/what/where"}

The event:
%s`, names, facts.String())
}

// callOllamaDispatch posts a single non-streaming generation and returns the raw
// completion (think-tags stripped). The client is a parameter because the two
// callers have genuinely different patience: a dispatch is authored on a game
// chokepoint and must not stall it, while a run summary rides a background
// ticker and can afford to wait for a bigger model. See runSummaryHTTP.
func callOllamaDispatch(client *http.Client, host, model, prompt string) (string, error) {
	payload := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"stream": false,
		"think":  false,
		"options": map[string]interface{}{
			"num_ctx": 4096,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	apiURL := strings.TrimRight(host, "/") + "/api/generate"
	resp, err := client.Post(apiURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return result.Response, nil
}

// parseDispatch pulls {headline, lede} out of the model's completion, tolerating
// the usual noise (think blocks, markdown fences, prose around the JSON). ok is
// false when no JSON object with a headline can be recovered.
func parseDispatch(raw string) (headline, lede string, ok bool) {
	s := raw
	// Drop a Qwen-style reasoning block if present.
	if i := strings.Index(s, "<think>"); i != -1 {
		if j := strings.Index(s, "</think>"); j != -1 {
			s = s[:i] + s[j+len("</think>"):]
		}
	}
	// Isolate the first {...} object so surrounding prose or fences don't break
	// the decode.
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return "", "", false
	}
	var out struct {
		Headline string `json:"headline"`
		Lede     string `json:"lede"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return "", "", false
	}
	headline = strings.TrimSpace(out.Headline)
	lede = strings.TrimSpace(out.Lede)
	if headline == "" {
		return "", "", false
	}
	return headline, lede, true
}
