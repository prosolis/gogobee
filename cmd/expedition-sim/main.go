// expedition-sim drives synthetic players through expeditions for batch
// analysis. Re-uses production plugin paths against a fresh sqlite DB so
// outcomes mirror what live players hit.
//
// Single run:
//   expedition-sim [-class fighter] [-level 5] [-zone goblin_warrens]
//                  [-bank 1000] [-cap 50] [-log] [-data DIR]
//
// Matrix mode (cartesian sweep over classes × levels × zones × N runs,
// one JSON object per stdout line, log suppressed by default):
//   expedition-sim -matrix -classes fighter,mage -levels 5,10 \
//                  -zones goblin_warrens,wolf_den -runs 20
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gogobee/internal/plugin"

	"maunium.net/go/mautrix/id"
)

func main() {
	var (
		class   = flag.String("class", "fighter", "DnD class id (single-run mode)")
		level   = flag.Int("level", 5, "character level (single-run mode)")
		zone    = flag.String("zone", "goblin_warrens", "zone id (single-run mode)")
		bank    = flag.Float64("bank", 1000, "starting coin balance — must cover outfitting")
		cap     = flag.Int("cap", 50, "max autopilot bursts per expedition (each = up to autopilotRoomCap rooms)")
		days    = flag.Int("days", 0, "stop after N synthetic day rollovers (0 = unbounded; the -cap safety net still applies)")
		dataDir = flag.String("data", "", "data dir for the temp sqlite db (default: OS tempdir; ignored in matrix mode)")
		userTag = flag.String("user", "@sim:expedition", "synthetic user id (single-run mode)")
		logFlag = flag.Bool("log", true, "include per-row expedition log in output (single-run default true; matrix default false)")

		matrix  = flag.Bool("matrix", false, "matrix mode — sweep over classes × levels × zones × runs")
		classes = flag.String("classes", "", "comma-separated class ids (matrix mode)")
		levels  = flag.String("levels", "", "comma-separated levels (matrix mode)")
		zones   = flag.String("zones", "", "comma-separated zone ids (matrix mode)")
		runs    = flag.Int("runs", 1, "replicates per (class,level,zone) cell (matrix mode)")

		trace = flag.Bool("trace", false, "include raw per-round CombatEvent stream on the LAST combat of each expedition (boss room) — for J2 diagnostic sweeps")

		petLevel = flag.Int("pet-level", 0, "attach a base housing pet at this level (1-10) to every sim character; 0 = no pet (default, matches prod char-creation)")
	)
	flag.Parse()

	if *petLevel < 0 || *petLevel > 10 {
		fail("pet-level must be 0-10, got", *petLevel)
	}

	plugin.SetSimIncludeTrace(*trace)
	plugin.SetSimPetLevel(*petLevel)

	if *matrix {
		// Matrix default: drop log to keep stdout manageable; explicit
		// -log=true overrides.
		includeLog := false
		// Flag.Lookup tells us whether the user explicitly set -log.
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "log" {
				includeLog = *logFlag
			}
		})
		runMatrix(*classes, *levels, *zones, *runs, *bank, *cap, *days, includeLog)
		return
	}

	runSingle(*class, *level, *zone, *userTag, *dataDir, *bank, *cap, *days, *logFlag)
}

func runSingle(class string, level int, zone, userTag, dataDir string, bank float64, cap, days int, includeLog bool) {
	dir := dataDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "expedition-sim-")
		if err != nil {
			fail("mkdir temp:", err)
		}
		defer os.RemoveAll(dir)
	}

	res, err := runOne(dir, id.UserID(userTag), plugin.DnDClass(class), level, plugin.ZoneID(zone), bank, cap, days)
	if err != nil {
		if res != nil {
			if !includeLog {
				res.Log = nil
			}
			emitIndented(res)
		}
		fail("run:", err)
	}
	if !includeLog {
		res.Log = nil
	}
	emitIndented(res)
}

func runMatrix(classes, levels, zones string, runs int, bank float64, cap, days int, includeLog bool) {
	cs := splitNonEmpty(classes)
	ls := parseLevels(levels)
	zs := splitNonEmpty(zones)
	if len(cs) == 0 || len(ls) == 0 || len(zs) == 0 || runs <= 0 {
		fail("matrix mode requires non-empty -classes, -levels, -zones and runs > 0")
	}
	enc := json.NewEncoder(os.Stdout)
	for _, c := range cs {
		for _, lv := range ls {
			for _, z := range zs {
				for r := 0; r < runs; r++ {
					dir, err := os.MkdirTemp("", "expedition-sim-")
					if err != nil {
						fail("mkdir temp:", err)
					}
					uid := id.UserID(fmt.Sprintf("@sim:%s-l%d-%s-%d", c, lv, z, r))
					res, runErr := runOne(dir, uid, plugin.DnDClass(c), lv, plugin.ZoneID(z), bank, cap, days)
					if res != nil && !includeLog {
						res.Log = nil
					}
					if runErr != nil && res == nil {
						// Synthesize a row so the corpus has one line per
						// cell regardless of init failures.
						res = &plugin.SimResult{
							UserID:  string(uid),
							Class:   c,
							Level:   lv,
							Zone:    z,
							Outcome: "halted",
						}
					}
					_ = enc.Encode(res)
					_ = os.RemoveAll(dir)
				}
			}
		}
	}
}

func runOne(dataDir string, uid id.UserID, class plugin.DnDClass, level int, zone plugin.ZoneID, bank float64, cap, days int) (*plugin.SimResult, error) {
	runner, err := plugin.NewSimRunner(dataDir)
	if err != nil {
		return nil, fmt.Errorf("init runner: %w", err)
	}
	defer runner.Close()

	if _, err := runner.BuildCharacter(uid, class, level); err != nil {
		return nil, fmt.Errorf("build character: %w", err)
	}
	runner.Euro.Credit(uid, bank, "expedition-sim bankroll")
	return runner.RunExpedition(uid, zone, cap, days)
}

func emitIndented(res *plugin.SimResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}

func splitNonEmpty(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseLevels(s string) []int {
	var out []int
	for _, p := range splitNonEmpty(s) {
		n, err := strconv.Atoi(p)
		if err != nil {
			fail("bad level:", p)
		}
		out = append(out, n)
	}
	return out
}

func fail(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}
