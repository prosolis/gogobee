# Fix: Texas Hold'em LLM Tips

## What's broken

Two confirmed issues observed across multiple tip examples:

### 1. Position label is inverted in heads-up play

The tip says "positional advantage" when the player is acting first post-flop (out of position) and "out of position" when they're acting last. The position label reaching the LLM prompt is wrong.

**Root cause:** the `positionLabel()` function in `tips.go` derives position from `DealerIdx` using the general formula. In heads-up play the dealer posts the small blind and acts first pre-flop but **last** post-flop. The heads-up exception that exists in `PostBlinds()` in `betting.go` is not being reflected in position label calculation.

**Fix:** in `positionLabel()`, gate on `len(g.Players) == 2` before applying any label logic. In heads-up:
- Pre-flop: dealer = BTN/SB (acts first), other player = BB (acts last)
- Post-flop: dealer = BTN (acts last, positional advantage), other player = BB (acts first, out of position)

Check which street it is before assigning the label. `g.Street == PreFlop` needs different position semantics than all other streets in heads-up.

---

### 2. LLM is generating generic concepts instead of hand-specific advice

**Observed:** tips reference equity numbers but then ignore what those numbers mean for the specific hand. A player with 8♥ 7♥ on Q♥ K♠ 10♦ (gutshot + backdoor flush draw, 29% equity, free card available) received "not enough equity to bet" — which ignores the draw entirely and misapplies a made-hand concept to a drawing hand.

**Root cause:** the user prompt is not giving the LLM enough structured context to reason about hand *type*. It sees an equity number but doesn't know whether the hand is a draw, a made hand, a bluff catcher, or air. It pattern-matches on the number alone.

**Fix:** compute and inject the following additional fields into `TipContext` before building the prompt:

```go
type TipContext struct {
    // existing fields...

    // Add these:
    HandCategory    string   // from poker.RankString() on current 5-card best
    IsDraw          bool     // true if outs > 0 (see below)
    FlushDrawOuts   int      // suited cards matching board suit count
    StraightDrawOuts int     // connected card gaps to straight
    TotalOuts       int      // combined draw outs (deduped)
    IsFreeCard      bool     // ToCall == 0
    HeadsUp         bool     // len(ActivePlayers) == 2
}
```

Outs calculation (add to `equity.go` or a new `outs.go`):
- Flush draw: count hole cards matching dominant board suit; if 2 hole cards + 2 board cards same suit, FlushDrawOuts = 9
- Open-ended straight draw: 8 outs
- Gutshot: 4 outs
- Backdoor draws: count as 1-2 outs each
- TotalOuts = sum, capped at 15 (avoid double-counting straights and flushes)

---

### 3. System prompt needs to be more directive

**Current system prompt** (paraphrased from blueprint): "be a concise Hold'em coach, 2-4 sentences, cover hand strength, pot odds, position."

This is too open-ended. The LLM fills the space with whatever poker concepts come to mind. Replace with a prompt that forces it to reason about the specific situation before speaking.

**New system prompt:**

```
You are a Texas Hold'em coach giving advice to a single player via private message.
You will receive structured game context. Reason through it in this order:

1. What type of hand do I have — made hand, drawing hand, or air?
2. If drawing: how many outs, and do pot odds justify continuing?
3. If made hand: is it strong enough to bet for value, or weak enough to just pot control?
4. Does position affect what I should do here?
5. Is a free card available, and if so, is taking it correct?

Then write ONE piece of advice — 2 to 3 sentences maximum — that tells the player
what to do and why, using the specific cards and numbers provided.
Do not list concepts. Do not use generic poker vocabulary without connecting it to
this specific hand. If the correct play is obvious (e.g. free card with a draw),
say so plainly and briefly.
```

---

### 4. User prompt needs draw and hand type context injected

**Current user prompt structure** (from blueprint):
```
Street: <street>
Your hand: <cards>
Board: <cards>
Equity vs <n> opponents: Win x% | Tie y% | Loss z%
Pot odds to call: x%
SPR: x | Position: <pos> | Active players: <n>
```

**New user prompt structure** — add the computed fields:

```
Street: {street}
Your hand: {cards}  [{hand_category}]
Board: {cards}
Draw outs: {total_outs} ({draw_description})   <- omit line if IsDraw == false
Equity vs {n} opponent(s): Win {x}% | Tie {y}% | Loss {z}%
{if ToCall > 0}: Pot odds to call: {pct}% — equity {exceeds|falls short of} price
{if IsFreeCard}: Free card available — no bet to call
SPR: {spr} | Position: {position} | Heads-up: {yes|no} | Street: {street}
```

`{draw_description}` examples:
- "flush draw (9 outs)"
- "gutshot straight draw (4 outs)"
- "open-ended straight draw (8 outs)"
- "flush draw + gutshot (11 outs)"
- "backdoor flush + backdoor straight (2 outs)"

`{hand_category}` examples from `poker.RankString()`:
- "High Card", "One Pair", "Two Pair", "Three of a Kind", "Straight", "Flush", "Full House", "Four of a Kind", "Straight Flush"

---

## Specific scenario the fix must handle correctly

**Hand:** 8♥ 7♥
**Board:** Q♥ K♠ 10♦
**Street:** Flop
**Equity:** 29%
**To call:** €0 (free card)
**Position:** dealer, heads-up, acting first post-flop (out of position)

Expected tip behaviour after fix:
- Identifies this as a drawing hand (gutshot + backdoor flush)
- Notes the free card is available
- Does NOT say "not enough equity to bet" without acknowledging the draw
- Does NOT say "positional advantage" — player is out of position post-flop heads-up
- Produces something like: "You have a gutshot straight draw with a backdoor flush. With a free card available you can check and see the turn without risk. If a 9 or a third heart comes, you'll be in a strong position — for now, take the free card."

---

## Reasoning mode (Qwen3 thinking)

Poker tips are the only task in GogoBee that should use reasoning mode. All other LLM calls (adventure narrative, etc.) run with thinking disabled. This needs to be a one-off configuration scoped entirely to `tips.go`.

### Why reasoning mode here

The tips failure pattern is not a knowledge gap — Qwen3-32B knows poker. The problem is that it jumps to pattern-matched conclusions without working through the situation in sequence. Reasoning mode forces the model to produce a `<think>...</think>` chain before the final response, which naturally surfaces: hand type, outs, position semantics, and the actual decision. The tip then follows from that chain rather than being assembled from disconnected concepts.

### Request changes in `tips.go`

Add a `enable_thinking` field to the request body and a `thinking_budget` cap to keep latency bounded:

```go
type llmRequest struct {
    Model          string       `json:"model"`
    Messages       []llmMessage `json:"messages"`
    MaxTokens      int          `json:"max_tokens"`
    Stream         bool         `json:"stream"`
    EnableThinking bool         `json:"enable_thinking,omitempty"`
    ThinkingBudget int          `json:"thinking_budget,omitempty"`
}
```

When building the tips request, set:

```go
body := llmRequest{
    Model:          cfg.Model,
    Messages:       []llmMessage{...},
    MaxTokens:      1000,   // increased to accommodate think block + response
    Stream:         false,
    EnableThinking: true,
    ThinkingBudget: 512,    // cap reasoning tokens; enough for poker, not runaway
}
```

`ThinkingBudget` of 512 tokens is sufficient for a poker hand analysis reasoning chain. Without a cap, complex board textures can produce very long think blocks. 512 keeps worst-case latency reasonable.

Note: the exact field names for Ollama's Qwen3 thinking mode may differ from the above. Check the Ollama API docs for the current `qwen3:32b` thinking parameters — it may be `/think` appended to the model name (`qwen3:32b/think`) rather than a request body field, depending on the Ollama version. Either way, the intent is the same — make this configurable in `TipsConfig` so it can be toggled without a code change:

```go
type TipsConfig struct {
    Endpoint       string
    Model          string
    APIKey         string
    Timeout        time.Duration
    EnableThinking bool          // default true for poker tips
    ThinkingBudget int           // default 512
}
```

### Strip the think block from the response

The `<think>...</think>` content must never reach the player DM. The current response parser takes `choices[0].message.content` directly. Update it to strip thinking content before returning:

```go
func extractTipFromResponse(raw string) string {
    // Strip <think>...</think> block if present
    // Qwen3 may use <think> or <!--think--> depending on version
    re := regexp.MustCompile(`(?s)<think>.*?</think>`)
    cleaned := re.ReplaceAllString(raw, "")

    // Also strip any leading/trailing whitespace left behind
    return strings.TrimSpace(cleaned)
}
```

Call `extractTipFromResponse()` on `llmResp.Choices[0].Message.Content` before returning the tip string. If the result is empty after stripping (model only produced a think block and nothing else), fall back to the rules-based tip.

### Latency expectations

With `ThinkingBudget: 512` and the structured context prompt, expect:
- Typical: 4-8 seconds total (within the existing 10s timeout)
- Complex boards: up to 10 seconds
- Increase `cfg.Timeout` to `12 * time.Second` for tips specifically to give reasoning room without affecting other LLM calls

Tip delivery via DM is already async (goroutine), so even a 10-12 second tip doesn't block the table view or the action loop. Players receive the table view immediately and the tip follows shortly after.

### Config addition

```toml
[holdem]
# ... existing fields ...
tips_enable_thinking = true
tips_thinking_budget = 512
tips_timeout         = "12s"   # longer than default to accommodate reasoning
```

---

## Files to change

- `tips.go` — `TipContext` struct, `BuildTipContext()`, `buildPrompt()`, `positionLabel()`, `llmRequest` struct, `GenerateTip()`, new `extractTipFromResponse()` function
- `equity.go` — add outs calculation function
- No schema changes required
- No changes to `game.go`, `betting.go`, or `render.go`

## Test cases to verify before shipping

Write a table-driven test in `tips_test.go` covering:

| Hand | Board | Street | Expected position (HU) | Expected IsDraw | Expected outs |
|------|-------|--------|------------------------|-----------------|---------------|
| 8♥ 7♥ | Q♥ K♠ 10♦ | Flop | Out of position | true | 4 (gutshot) + backdoor |
| A♠ K♠ | — | Pre-Flop | BTN (dealer, acts first) | false | 0 |
| 5♥ 6♥ | 7♥ 8♣ 2♥ | Flop | varies | true | 15 (OESD + flush) |
| Q♣ Q♦ | Q♥ 2♠ 7♣ | Flop | varies | false | 0 |

The position test for heads-up pre-flop vs post-flop is the most important one. Get that right first.

---

## Validation pipeline (shipped 2026-04-13)

The "is the tip actually good?" question is now answered by a two-layer
automated test harness rather than vibes.

**Layer 1 — hand-authored scenarios** (`internal/plugin/holdem_tip_scenarios.go`)

20 canonical spots covering preflop tier/facing-bet branches and postflop
equity tiers × board textures × SPR depths. Each scenario declares an
expected action verb, required theme keywords, and forbidden substrings.
`TestTipScenarios_Layer1` runs the full rules-engine pipeline
(equity MC, draw detection, hand category, board texture, preflop
classification) against each scenario and asserts the tip contains the
expected action + themes. Fast, cheap, green.

**Layer 2 — solver-derived scenarios** (same scenarios, populated via `cmd/gensolver`)

11 of the 14 postflop scenarios carry real TexasSolver GTO frequencies
committed as a fixture at `internal/plugin/testdata/solver_freqs.json`.
`TestTipScenarios_Layer2` treats any action with solver frequency ≥ 15% as
"significant" and asserts the rules engine's recommended action matches one
of the significant actions — tolerating GTO's legitimately mixed spots
while catching genuinely-wrong recommendations.

**cmd/gensolver**

Offline pipeline that iterates `plugin.TipScenarios()`, shells out to
`console_solver` (TexasSolver CLI), parses the JSON strategy tree,
navigates to hero's decision node (IP/OOP × facing-check/facing-bet ×
check-bet line), extracts hero's action frequencies for their exact hole
combo, and merges them into the fixture file.

Key solver-side knobs worked out the hard way:

- **Scale normalization** to `pot=50, stack=8×pot` (SPR cap 8). TexasSolver
  segfaults on deep stacks and on some textures at larger chip counts;
  strategic equivalence is preserved because GTO frequencies are
  scale-invariant.
- **Bet tree**: 50% + 100% pot sizings, plus allin. Narrower trees build
  faster and still give solvable decision points.
- **`set_accuracy 1.0`, `set_max_iteration 100`** — converges in ~2 min
  per flop instead of the ~24 min the solver's defaults demanded. 1%
  exploitability is plenty for our assertion type.
- **Range syntax**: TexasSolver rejects shorthand like `22+` / `A2s+` —
  ranges must be explicitly enumerated. Using the solver's own
  sample-input ranges verbatim as HU defaults.

Invocation:

```bash
GOGOBEE_SOLVER=/path/to/console_solver \
GOGOBEE_SOLVER_RESOURCES=/path/to/TexasSolver/resources \
go run ./cmd/gensolver [scenario-name-substring]
```

Results merge into the fixture, so regenerating one scenario doesn't wipe
the others.

**Known gaps** — 3 scenarios have no solver frequencies:

- `flop/monster set on paired board facing bet` — TexasSolver segfaults on
  paired-board textures (upstream bug, not fixable from our side).
- `turn/weak top pair facing overbet` — hero's hole (63o) isn't in any
  reasonable HU range, so the solver never allocates strategy for it.
  Scenario still validated by Layer 1.
- Occasional flake on `flop/bottom pair facing big bet` at full-batch time
  (succeeds when retried solo). Current fixture entry came from a solo
  retry and is valid; if regeneration fails, just re-run that one
  scenario with the name filter.

Adding new scenarios: append to `tipScenarios` in
`holdem_tip_scenarios.go`, run `cmd/gensolver` with the name filter,
commit both the code and fixture changes together.
