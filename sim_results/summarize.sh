#!/usr/bin/env bash
# Aggregate h4_baseline.jsonl into a per-(class,level,zone) table.
# Columns: class, level, zone, n, p50 yield, mean yield, mean rooms, %cleared, %tpk, %fled, %extracted.
set -euo pipefail

infile=${1:-h4_baseline.jsonl}
cd "$(dirname "$0")"

jq -rs '
  group_by([.Class, .Level, .Zone])
  | map({
      class:  .[0].Class,
      level:  .[0].Level,
      zone:   .[0].Zone,
      n:      length,
      yields: [.[] | .YieldCount],
      rooms:  [.[] | .Rooms],
      outcomes: [.[] | .Outcome]
    })
  | map(. + {
      p50_yield:  (.yields | sort | .[(length/2|floor)]),
      mean_yield: ((.yields | add) / length),
      mean_rooms: ((.rooms  | add) / length),
      pct_cleared:   (([.outcomes[] | select(. == "cleared")]   | length) * 100 / .n),
      pct_tpk:       (([.outcomes[] | select(. == "tpk")]       | length) * 100 / .n),
      pct_fled:      (([.outcomes[] | select(. == "fled")]      | length) * 100 / .n),
      pct_extracted: (([.outcomes[] | select(. == "extracted")] | length) * 100 / .n)
    })
  | sort_by(.zone, .class, .level)
  | (["class","level","zone","n","p50_yld","mean_yld","mean_rms","%clr","%tpk","%fled","%ext"] | @tsv),
    (.[] | [.class, .level, .zone, .n, .p50_yield,
            (.mean_yield * 10 | round / 10),
            (.mean_rooms * 10 | round / 10),
            (.pct_cleared   | round),
            (.pct_tpk       | round),
            (.pct_fled      | round),
            (.pct_extracted | round)] | @tsv)
' "$infile" | column -t -s $'\t'
