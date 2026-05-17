#!/usr/bin/env bash
# Aggregate a sim corpus JSONL into a per-(class,level,zone) table.
# Columns: class, level, zone, n, p50 yield (all), p50 yield among cleared,
# mean yield, mean rooms, %cleared, %boss-reached, %tpk, %fled, %extracted.
#
# boss-reached counts runs whose StopCode is "boss" or "complete" — i.e.,
# the autopilot entered the boss room, whether the run died there or
# finished the zone.
set -euo pipefail

infile=${1:-baseline_j0.jsonl}
cd "$(dirname "$0")"

jq -rs '
  def p50(xs): if (xs|length)==0 then 0 else (xs|sort|.[(length/2|floor)]) end;

  group_by([.Class, .Level, .Zone])
  | map({
      class:    .[0].Class,
      level:    .[0].Level,
      zone:     .[0].Zone,
      n:        length,
      yields:   [.[] | .YieldCount],
      yields_cleared: [.[] | select(.Outcome=="cleared") | .YieldCount],
      rooms:    [.[] | .Rooms],
      outcomes: [.[] | .Outcome],
      stops:    [.[] | .StopCode]
    })
  | map(. + {
      p50_yield:         p50(.yields),
      p50_yield_cleared: p50(.yields_cleared),
      mean_yield:        ((.yields | add) / .n),
      mean_rooms:        ((.rooms  | add) / .n),
      pct_cleared:       (([.outcomes[] | select(. == "cleared")]   | length) * 100 / .n),
      pct_boss_reached:  (([.stops[]    | select(. == "boss" or . == "complete")] | length) * 100 / .n),
      pct_tpk:           (([.outcomes[] | select(. == "tpk")]       | length) * 100 / .n),
      pct_fled:          (([.outcomes[] | select(. == "fled")]      | length) * 100 / .n),
      pct_extracted:     (([.outcomes[] | select(. == "extracted")] | length) * 100 / .n)
    })
  | sort_by(.zone, .class, .level)
  | (["class","level","zone","n","p50_yld","p50_yld_clr","mean_yld","mean_rms","%clr","%boss","%tpk","%fled","%ext"] | @tsv),
    (.[] | [.class, .level, .zone, .n,
            .p50_yield, .p50_yield_cleared,
            (.mean_yield * 10 | round / 10),
            (.mean_rooms * 10 | round / 10),
            (.pct_cleared      | round),
            (.pct_boss_reached | round),
            (.pct_tpk          | round),
            (.pct_fled         | round),
            (.pct_extracted    | round)] | @tsv)
' "$infile" | column -t -s $'\t'
