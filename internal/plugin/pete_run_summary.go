package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"
)

// The run summary — three sentences over forty beats.
//
// Every other line in the liveblog is assembled by Pete out of a beat's own
// nouns and numbers, and that is the right split: a log has to be exactly what
// happened, in order, and prose in the middle of it would be the more convincing
// of the two accounts and the less true. But a *report* is read afterwards, by
// somebody who wasn't watching, and the question it answers is not "what
// happened" — the log already answers that — it is "what was that run". That is
// a judgement, and no template makes judgements.
//
// So this is the one piece of prose on the channel, and it earns the model far
// better than a dispatch headline does. authorDispatch turns four fields into a
// sentence a template could nearly have written; this reads a whole expedition
// and picks out what mattered.
//
// Three rules, and the first one is why this file exists at all:
//
//   - **Off the hot path.** It runs on the roster ticker, not at the moment the
//     run ends. A run ending is already a player-facing beat with a dispatch
//     being authored against it; adding a second bounded-but-real LLM call to
//     that chokepoint would stall the command that killed the boss.
//   - **One per tick.** A backlog after an outage drains over minutes rather
//     than spooling a hundred generations at once.
//   - **Best effort, exactly once.** A run that can't be summarised is filed
//     with an empty summary beat rather than retried forever — the row is what
//     stops the sweep picking it up again next tick, and a report with no
//     summary is still the log and the numbers, which is most of it.

// runSummaryMaxBeats bounds what goes into the prompt. Far more than a normal
// run produces; the cap is for the multi-day expedition that beat out hundreds,
// where the last chunk is the part with the ending in it.
const runSummaryMaxBeats = 120

// maxRunSummary mirrors Pete's cap so we never ship prose Pete will reject on
// length alone. Byte count, matching Pete's len() check.
const maxRunSummary = 1200

// runSummaryTimeout has to cover a COLD model, not just a generation.
//
// dispatchLLMTimeout is tight because authoring runs on a game chokepoint and a
// template dispatch now beats a voiced one late. Nothing here is waiting on this:
// it is a background sweep, the run ended minutes ago, and the page it feeds is
// already serving without it. The cost of being impatient is the opposite of
// there — a timeout files an empty summary beat, and that run never gets another
// chance at one.
//
// And impatience is the live risk, because this call is almost always the one
// that pays the load. Ollama evicts an idle model after about five minutes, and
// runs end far further apart than that, so the steady state is: model on disk,
// nothing resident, weights to page in before the first token. A budget sized
// for generation alone would expire during the load on every single run and file
// an empty beat that says the box is down when the box is fine. So this is sized
// for load-then-generate, and a timeout here really does mean the box is down.
const runSummaryTimeout = 5 * time.Minute

// runSummaryBusy is the whole concurrency story: at most one sweep in flight,
// ever. The ticker starts one and moves on, so a cold model loading for minutes
// costs the board nothing, and the ticks that fire meanwhile find the flag set
// and skip rather than queue.
var runSummaryBusy atomic.Bool

// sweepRunSummariesAsync starts a sweep off the caller's goroutine if one isn't
// already running.
//
// It has to be off the ticker: runSummaryTimeout is minutes and the tick is two,
// so a synchronous call would hold the roster, details, siege and beat pushes
// behind a model load and put the live board permanently a tick or more behind
// the game. Ordering against those pushes is not lost by going async — the beat
// this files is written to the local buffer with the next seq, and the pusher
// ships it by seq on whichever tick comes after, still behind the run's own log.
func (p *AdventurePlugin) sweepRunSummariesAsync() {
	if !runSummaryBusy.CompareAndSwap(false, true) {
		return // one still working; the next tick will find it done or still busy
	}
	go func() {
		defer runSummaryBusy.Store(false)
		p.sweepRunSummaries()
	}()
}

// sweepRunSummaries authors the summary for at most one finished run per call.
// Called from the roster ticker, after the beats themselves have been pushed —
// the summary is the last beat of a run's story and there is no rush to have it
// overtake the log it is about.
func (p *AdventurePlugin) sweepRunSummaries() {
	if !peteclient.Enabled() || !newsEmissionOn() {
		return
	}
	if !llmConfigured() {
		return // no model, no summary, no wasted queries asking which run needs one
	}
	runID := nextRunNeedingSummary()
	if runID == "" {
		return
	}
	// File the beat whatever happens below. An empty one carries no prose and Pete
	// stores nothing from it — its entire job is to be the row that stops this run
	// coming back round every two minutes for the rest of the week.
	summary, name := authorRunSummary(runID)
	if summary == "" {
		slog.Debug("run summary: nothing authored, filing an empty beat to close it out", "run", runID)
	}
	recordRunBeat(runID, peteclient.RunBeat{
		Kind:  "summary",
		Name:  name,
		Prose: summary,
	})
}

// nextRunNeedingSummary picks the most recently finished run that has an `end`
// beat and no `summary` beat yet.
//
// Newest first, deliberately. If the sweep is behind — an outage, a busy
// evening — the run somebody is most likely to be looking at right now is the
// one that just ended, not the one from four hours ago. The old ones still get
// their turn on later ticks; they just don't get to hold up the fresh one.
func nextRunNeedingSummary() string {
	var runID string
	err := db.Get().QueryRow(`
		SELECT e.run_id
		  FROM pete_run_beat e
		 WHERE e.kind = 'end'
		   AND NOT EXISTS (
		       SELECT 1 FROM pete_run_beat s
		        WHERE s.run_id = e.run_id AND s.kind = 'summary')
		 ORDER BY e.occurred_at DESC
		 LIMIT 1`).Scan(&runID)
	if err != nil {
		return "" // ErrNoRows is the common case: nothing to summarise
	}
	if !runBeatAllowed(runID) {
		// An opted-out player's beats never leave the box, so there is nothing for
		// a summary to be attached to. File the closing beat anyway (it will be
		// retired locally with the rest) so this run stops being picked.
		recordRunBeat(runID, peteclient.RunBeat{Kind: "summary"})
		return ""
	}
	return runID
}

// authorRunSummary reads a run's beats back and returns Pete's summary of it,
// plus the adventurer's name for the guard allow-list. Returns empty strings on
// any failure — the model being off, a timeout, an unparseable completion, an
// over-long generation — because every one of those is a report without a
// summary rather than a problem.
func authorRunSummary(runID string) (summary, name string) {
	beats, err := loadRunBeatsForSummary(runID)
	if err != nil || len(beats) == 0 {
		return "", ""
	}
	name = runSummarySubject(beats)
	if name == "" {
		// No name means no allow-list on Pete's side, which means the guard rejects
		// anything naming anyone. Don't spend a generation to have it thrown away.
		return "", ""
	}

	raw, err := callLLMDispatch(runSummaryTimeout, buildRunSummaryPrompt(name, beats))
	if err != nil {
		slog.Warn("run summary: LLM authoring failed", "run", runID, "err", err)
		return "", name
	}
	summary = parseRunSummary(raw)
	if summary == "" || len(summary) > maxRunSummary {
		slog.Warn("run summary: unusable output", "run", runID, "len", len(summary))
		return "", name
	}
	return summary, name
}

// loadRunBeatsForSummary reads a run's own beats back out of the outbound
// buffer. It reads the TAIL and re-sorts, so a run long enough to hit the cap
// contributes the part with its ending in it rather than its first morning.
func loadRunBeatsForSummary(runID string) ([]peteclient.RunBeat, error) {
	rows, err := db.Get().Query(`
		SELECT seq, payload FROM pete_run_beat
		 WHERE run_id = ? ORDER BY seq DESC LIMIT ?`, runID, runSummaryMaxBeats)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []peteclient.RunBeat
	for rows.Next() {
		var seq int64
		var payload string
		if err := rows.Scan(&seq, &payload); err != nil {
			return nil, err
		}
		var b peteclient.RunBeat
		if err := json.Unmarshal([]byte(payload), &b); err != nil {
			continue // a row the pusher will retire on its own; not this sweep's problem
		}
		b.RunID, b.Seq = runID, seq
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// runSummarySubject finds the one name a summary is allowed to use. Only the
// `start` beat carries identity, by design — so a run whose start beat was
// dropped has no subject here, and gets no summary rather than an anonymous one.
func runSummarySubject(beats []peteclient.RunBeat) string {
	for _, b := range beats {
		if b.Name != "" {
			return b.Name
		}
	}
	return ""
}

// buildRunSummaryPrompt renders the run as a plain numbered log and asks for
// three sentences over it.
//
// The beats go in as facts, not as Pete's rendered lines: Pete's phrasing is
// Pete's, and feeding a model its own output back would have it summarising a
// summary. The rules are the dispatch prompt's, tightened in the one place that
// matters here — a run log is full of monster names and a model asked to write
// about a party is very willing to invent a second member of it.
func buildRunSummaryPrompt(name string, beats []peteclient.RunBeat) string {
	var log strings.Builder
	zone, outcome := "", ""
	n := 0
	for _, b := range beats {
		if b.Zone != "" && zone == "" {
			zone = b.Zone
		}
		line := describeBeatForPrompt(b)
		if line == "" {
			continue
		}
		if b.Kind == "end" {
			outcome = b.Outcome
		}
		n++
		fmt.Fprintf(&log, "%d. %s\n", n, line)
	}
	if zone == "" {
		zone = "a dungeon"
	}
	ending := "the run ended"
	switch outcome {
	case "cleared":
		ending = "they cleared it"
	case "died":
		ending = "they died down there"
	case "retreated":
		ending = "they walked out alive but beaten"
	}

	return fmt.Sprintf(`You are Pete, a warm, friendly local news reporter for a fantasy adventuring town. Think a beloved local newscaster who genuinely knows everyone and is glad to see them. Conversational, never snarky, never a caps-lock hype-man. Warmth carries the register, not exclamation marks.

Below is the log of one expedition, room by room, exactly as it was recorded. Write a SHORT summary of how the run went: what it cost them, the moment it turned, and how it ended.

STRICT RULES — do not violate these:
- The ONLY adventurer name you may use is: %s. Never invent another adventurer, companion, party member or friend. If the log does not say someone was there, they were not there.
- Monster, zone and item names in the log are game names — use them as given.
- Use ONLY what the log says. Do not invent numbers, fights, items or outcomes.
- Do NOT add numbers together and do NOT state any total. The exact totals are printed next to your summary and a total you worked out yourself will contradict them. Quote a number only if that exact number appears on one line of the log.
- Three sentences at most. No markdown, no emoji, no headline, no bullet points.
- Past tense, third person. Do not address the reader as "you".

Respond with ONLY a JSON object, no other text:
{"summary": "at most three sentences about how the run went"}

The expedition: %s went into %s, and %s.

The log:
%s`, name, name, zone, ending, log.String())
}

// describeBeatForPrompt renders one beat as a flat fact line for the prompt.
// Returns "" for a beat with nothing in it worth a sentence — a room with no
// identity, an empty haul — so the model isn't handed forty lines of "walked
// into the next room" to find three sentences in.
func describeBeatForPrompt(b peteclient.RunBeat) string {
	switch b.Kind {
	case "start":
		if b.TotalRooms > 0 {
			return fmt.Sprintf("set out into %s, %d rooms deep", orSomething(b.Zone), b.TotalRooms)
		}
		return "set out into " + orSomething(b.Zone)
	case "combat":
		what := orSomething(b.Target)
		switch b.RoomKind {
		case "boss":
			what = "the boss, " + what
		case "elite":
			what = "an elite, " + what
		}
		switch b.Outcome {
		case "won":
			s := fmt.Sprintf("killed %s, taking %d damage", what, b.Amount)
			if b.HPMax > 0 {
				s += fmt.Sprintf(" (left on %d of %d health)", b.HP, b.HPMax)
			}
			if b.Crits > 0 {
				s += fmt.Sprintf(", %d critical hit(s)", b.Crits)
			}
			return s
		case "retreat":
			return "could not finish " + what + " in time and withdrew"
		default:
			return "was beaten by " + what
		}
	case "trap":
		if b.Amount <= 0 {
			return "spotted a trap and stepped over it"
		}
		s := fmt.Sprintf("sprung a trap for %d damage", b.Amount)
		if b.HPMax > 0 {
			s += fmt.Sprintf(" (left on %d of %d health)", b.HP, b.HPMax)
		}
		return s
	case "treasure":
		return "found " + orSomething(b.Target)
	case "lock":
		if b.Outcome == "picked" {
			return "picked a locked door"
		}
		return "found every way on sealed and doubled back"
	case "region":
		return "crossed out of " + orSomething(b.Region) + " into " + orSomething(b.Target)
	case "haul":
		if b.Amount <= 0 {
			return ""
		}
		return fmt.Sprintf("gathered %d supplies along the way", b.Amount)
	case "end":
		switch b.Outcome {
		case "cleared":
			return "finished the run and got out"
		case "died":
			return "did not come home"
		case "retreated":
			return "withdrew, wounded but alive"
		}
		return "the run ended"
	}
	return ""
}

func orSomething(s string) string {
	if s == "" {
		return "something"
	}
	return s
}

// parseRunSummary pulls {"summary": ...} out of the completion, tolerating the
// same noise parseDispatch does: reasoning blocks, fences, prose around the JSON.
func parseRunSummary(raw string) string {
	s := raw
	if i := strings.Index(s, "<think>"); i != -1 {
		if j := strings.Index(s, "</think>"); j != -1 {
			s = s[:i] + s[j+len("</think>"):]
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &out); err != nil {
		return ""
	}
	return strings.TrimSpace(out.Summary)
}
