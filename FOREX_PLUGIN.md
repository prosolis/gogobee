# GogoBee — `forex` Plugin

Design document for Claude Code integration.

---

## Purpose

Tracks USD exchange rates against EUR and JPY using the [Frankfurter API](https://www.frankfurter.app) (ECB-sourced, no API key required). Stores daily history in bbolt, computes buy signals, and exposes Matrix commands for rate checks, full reports, and threshold alerts.

---

## File Layout

Place all files under `plugins/forex/` in the GogoBee module tree.

```
plugins/forex/
├── forex.go       # Plugin struct, Start/Stop, HandleCommand, all Matrix command handlers
├── store.go       # bbolt persistence (rates + alerts)
├── analyzer.go    # Signal computation (30/90d moving averages, 52w range, buy score)
├── scheduler.go   # Frankfurter HTTP client, backfill, daily poller goroutine
```

---

## Matrix Commands

All commands use the `!fx` prefix. Currency arguments are optional where noted and accept any currency in `TrackedCurrencies`.

| Command | Args | Behavior |
|---|---|---|
| `!fx rate` | `[currencies...]` | Current rate + one-line quick signal. No args = all tracked currencies. |
| `!fx report` | `[currencies...]` | Full analysis block per currency: 30/90d averages, 52w range bar, buy score. |
| `!fx setalert` | `<currency> <rate>` | Save a threshold alert for this room. Fires when `1 USD >= rate`. |
| `!fx alerts` | — | List all active alerts in the current room, with last-fired time if applicable. |
| `!fx delalert` | `<currency> <rate>` | Remove a specific alert. |

**Example usage:**
```
!fx rate                  → EUR and JPY quick signals
!fx rate JPY              → JPY only
!fx report EUR JPY        → full report for both
!fx setalert JPY 155      → alert when USD/JPY ≥ 155
!fx delalert JPY 155
```

---

## Data Source

**Frankfurter.app** — wraps ECB reference rates. No authentication. Two endpoints used:

- `GET /latest?from=USD&to=EUR,JPY` — current rates (called on-demand for `!fx rate/report` and daily by the poller)
- `GET /YYYY-MM-DD..YYYY-MM-DD?from=USD&to=EUR,JPY` — date range (used for the one-time backfill)

Both return JSON. All currencies for a given date arrive in a single response, so there is only ever one HTTP call per operation regardless of how many currencies are tracked.

---

## Storage (`store.go`)

**Database:** bbolt, single file, path configurable at `New()` call time (e.g. `./data/forex.db`).

**Two buckets:**

### `rates`

Key: `"EUR:20240315"` (currency + date in `YYYYMMDD`, lexicographically sortable per currency)  
Value: JSON-encoded `RateRecord{Date, Currency, Rate}`

Range queries use bbolt cursor `Seek` + forward scan bounded by the end key. Each currency's history is naturally isolated by key prefix.

### `alerts`

Key: `"<roomID>:<userID>:<currency>:<threshold>"` (float formatted to 6 decimal places)  
Value: JSON-encoded `AlertRecord{RoomID, UserID, Currency, Threshold, FiredAt}`

`FiredAt` is a Unix timestamp (int64). Zero means never fired. Alerts are reset (set back to zero) if `FiredAt` is older than 24 hours — called automatically after each daily poll.

---

## Signal Computation (`analyzer.go`)

`Compute(store, currency, currentRate)` returns a `Signal` struct.

**Inputs gathered from store:**
- 30-day rate slice (stored records + today's live rate appended)
- 90-day rate slice (same)
- 52-week rate slice (same)

**Derived values:**

| Field | Calculation |
|---|---|
| `Avg30` / `Avg90` | Arithmetic mean of respective slice |
| `High52w` / `Low52w` | Min/max of 52-week slice |
| `Percentile` | `(current - low) / (high - low) * 100` — where current sits in the 52w range |
| `Score` (1–10) | `percentile/100 * 10 * 0.6 + devScore * 0.4` |

`devScore` is derived from the average deviation above the 30d and 90d moving averages, mapped to a 0–10 scale where +5% above average = 10, −5% = 0.

**Score labels:**

| Score | Label | Emoji |
|---|---|---|
| ≥ 8 | Excellent | 🟢 |
| ≥ 6 | Good | 🟡 |
| ≥ 4 | Fair | 🟠 |
| < 4 | Poor | 🔴 |

**Note on score direction:** Higher score = dollar is *stronger* relative to history = good time to convert USD to a foreign currency. This is correct for both EUR (Portugal move) and JPY (Japan purchases).

**Rate formatting:** JPY prints to 2 decimal places (`154.32`); all other currencies to 4 (`1.0842`). Controlled by `formatRate(currency, rate)` in `analyzer.go`.

---

## Daily Poller (`scheduler.go`)

`StartDailyPoller(ctx, store, pollHourUTC, alertCallback)` launches a goroutine that:

1. Sleeps until `pollHourUTC:01 UTC` (default: `17` — ECB publishes ~16:00 CET)
2. Calls `FetchCurrentRates` for all `TrackedCurrencies`
3. Saves each rate to the store
4. Calls `store.ResetFiredAlerts()` (clears alerts fired >24h ago)
5. Calls `alertCallback(rates)` so `forex.go` can check thresholds and send Matrix notifications
6. Sleeps until next day

**Backfill** (`Backfill(ctx, store)`) is called once at `Start()`. It fetches the full trailing year in one request and saves all records. Safe to call repeatedly — bbolt upserts are idempotent.

---

## Plugin Lifecycle

```go
// Initialization (in GogoBee main/registry)
p, err := forex.New(matrixClient, "./data/forex.db")
p.Start()   // triggers backfill + launches poller goroutine
defer p.Stop()

// Command dispatch (in message event handler)
// parts = strings.Fields(messageBody), e.g. ["!fx", "rate", "JPY"]
if parts[0] == "!fx" {
    p.HandleCommand(evt, parts[1:])
}
```

`New` takes the mautrix `*mautrix.Client` and the bbolt file path.  
`HandleCommand` takes the `*event.Event` and the argument slice (everything after `!fx`).

---

## Adding Currencies

One change required: add the ISO 4217 code to `TrackedCurrencies` and a `CurrencyMeta` entry in `store.go`.

```go
var TrackedCurrencies = []string{"EUR", "JPY", "GBP"}

var CurrencyMeta = map[string]struct {
    Emoji string
    Label string
}{
    "EUR": {Emoji: "🇪🇺", Label: "Euro"},
    "JPY": {Emoji: "🇯🇵", Label: "Japanese Yen"},
    "GBP": {Emoji: "🇬🇧", Label: "British Pound"},
}
```

Frankfurter supports all major ISO currencies. The backfill and daily poll will automatically include any new entry on next run.

---

## Dependencies

```
go.etcd.io/bbolt v1.3.9
maunium.net/go/mautrix v0.18.1   // already in GogoBee
gopkg.in/yaml.v3                 // only if config file support is added later
```

No CGO. Builds cleanly on Linux/ARM (Hetzner CAX11 compatible).

---

## Notes for Integration

- The `mdToHTML` function in `forex.go` is a minimal stub that handles `**bold**` and `` `code` `` only. Replace with GogoBee's existing markdown renderer if one is available — the function signature is `mdToHTML(s string) string`.
- Alert `FiredAt` uses a 24-hour cooldown. If the daily poll fires while an alert's threshold is still met the next day, it will re-notify. This is intentional — persistent high rates should keep notifying.
- Live rate fetch has a 10-second timeout. On failure it falls back to the most recent stored rate per currency and logs a warning. Commands will still work, but the response will note staleness via the log (not surfaced to Matrix — add if desired).
