package plugin

import (
	"strings"
	"testing"

	"gogobee/internal/peteclient"
)

func TestParseDispatch(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantOK      bool
		wantHeadPre string
	}{
		{
			name:        "clean json",
			raw:         `{"headline": "Josie cleared the Ossuary.", "lede": "Alone, no less."}`,
			wantOK:      true,
			wantHeadPre: "Josie cleared",
		},
		{
			name:        "wrapped in prose and fences",
			raw:         "Sure! Here you go:\n```json\n{\"headline\":\"A win.\",\"lede\":\"Nice one.\"}\n```",
			wantOK:      true,
			wantHeadPre: "A win.",
		},
		{
			name:        "think block stripped",
			raw:         "<think>let me consider the tone</think>\n{\"headline\":\"Held the line.\",\"lede\":\"Proud of you all.\"}",
			wantOK:      true,
			wantHeadPre: "Held the line.",
		},
		{name: "no json", raw: "I could not write that.", wantOK: false},
		{name: "empty headline", raw: `{"headline":"","lede":"body"}`, wantOK: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, l, ok := parseDispatch(c.raw)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (h=%q l=%q)", ok, c.wantOK, h, l)
			}
			if ok && !strings.HasPrefix(h, c.wantHeadPre) {
				t.Errorf("headline = %q, want prefix %q", h, c.wantHeadPre)
			}
		})
	}
}

// TestBuildDispatchPrompt pins the two properties the prose-guard depends on:
// the allowed names are stated verbatim, and only the set facts appear (no empty
// labels for the model to fill in with invention).
func TestBuildDispatchPrompt(t *testing.T) {
	f := peteclient.Fact{
		EventType: "boss_kill", Subject: "Josie", Boss: "the Bone Warden",
		Zone: "the Ossuary", Level: 14, Actors: []string{"Josie"},
	}
	p := buildDispatchPrompt(f)

	if !strings.Contains(p, "ONLY these adventurer names, exactly as written: Josie") {
		t.Errorf("prompt does not constrain names to Actors:\n%s", p)
	}
	if !strings.Contains(p, "the Bone Warden") || !strings.Contains(p, "the Ossuary") {
		t.Error("prompt dropped a supplied fact")
	}
	// Unset fields must not appear as empty labels.
	if strings.Contains(p, "region:") || strings.Contains(p, "milestone:") {
		t.Errorf("prompt lists an unset fact:\n%s", p)
	}
}

// TestBuildDispatchPromptRealmEvent: a realm-level fact with no named adventurer
// still produces a usable prompt that tells the model there is no name to use.
func TestBuildDispatchPromptRealmEvent(t *testing.T) {
	f := peteclient.Fact{EventType: "siege_start", Boss: "the Horde", Stakes: "the whole town"}
	p := buildDispatchPrompt(f)
	if !strings.Contains(p, "no named adventurer") {
		t.Errorf("realm event prompt missing the no-name note:\n%s", p)
	}
}
