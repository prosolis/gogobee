#!/usr/bin/env bash
# Parallel driver for cmd/expedition-sim matrix sweeps.
#
# Spawns one expedition-sim process per (class, level, zone) cell, so each
# worker owns its own global sqlite handle (NewSimRunner closes+reinits the
# package-level db.Get() — workers in the same process would clobber each
# other). All worker stdouts are concatenated into a single JSONL.
#
# Usage:
#   run_matrix.sh OUTFILE RUNS CLASSES LEVELS ZONES [PARALLEL]
#
# Example (1350 rows, 14-way parallel):
#   run_matrix.sh baseline_j0.jsonl 30 \
#     fighter,mage,rogue 3,7,12 \
#     goblin_warrens,forest_shadows,manor_blackspire,underdark,dragons_lair
set -euo pipefail

outfile=${1:?outfile required}
runs=${2:?runs required}
classes=${3:?classes required}
levels=${4:?levels required}
zones=${5:?zones required}
parallel=${6:-$(nproc)}

repo=$(cd "$(dirname "$0")/.." && pwd)
bin=$repo/expedition-sim
[[ -x "$bin" ]] || { echo "missing $bin — run 'go build ./cmd/expedition-sim' first" >&2; exit 1; }

cd "$repo/sim_results"
tmpdir=$(mktemp -d -t simrun-XXXXXX)
trap 'rm -rf "$tmpdir"' EXIT

errfile=${outfile%.jsonl}.err
: > "$outfile"
: > "$errfile"

# Enumerate cells one per line: "class level zone".
cells=$tmpdir/cells.txt
IFS=, read -ra cls <<< "$classes"
IFS=, read -ra lvs <<< "$levels"
IFS=, read -ra zns <<< "$zones"
for c in "${cls[@]}"; do
  for l in "${lvs[@]}"; do
    for z in "${zns[@]}"; do
      printf "%s\t%s\t%s\n" "$c" "$l" "$z"
    done
  done
done > "$cells"

ncells=$(wc -l < "$cells")
echo "matrix: $ncells cells × $runs runs = $((ncells * runs)) rows, $parallel workers" >&2

# Fan out: one process per cell. Per-cell stdout goes to its own shard,
# stderr is collected to the shared errfile.
worker() {
  local class=$1 level=$2 zone=$3
  local shard=$tmpdir/$class-$level-$zone.jsonl
  "$bin" -matrix \
    -classes "$class" -levels "$level" -zones "$zone" \
    -runs "$runs" \
    > "$shard" 2>> "$errfile"
}
export -f worker
export bin tmpdir runs errfile

xargs -P "$parallel" -L 1 -a "$cells" bash -c 'worker "$@"' _

# Stitch shards in deterministic order (zone, class, level — matches
# summarize.sh's sort_by) so diff-friendliness survives parallel arrival.
for c in "${cls[@]}"; do
  for l in "${lvs[@]}"; do
    for z in "${zns[@]}"; do
      shard=$tmpdir/$c-$l-$z.jsonl
      [[ -f "$shard" ]] && cat "$shard" >> "$outfile"
    done
  done
done

rows=$(wc -l < "$outfile")
echo "wrote $rows rows → $outfile (errors in $errfile)" >&2
