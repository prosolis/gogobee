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
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

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

		jobs = flag.Int("jobs", 0, "matrix mode — concurrent worker count (each worker is a subprocess so it gets its own sqlite). 0 = runtime.NumCPU()")
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
		runMatrix(*classes, *levels, *zones, *runs, *bank, *cap, *days, includeLog, *jobs, *trace, *petLevel)
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

// matrixJob is one (class, level, zone, replicate-index) cell of the
// matrix sweep. Each job is run by a worker as a single-run subprocess so
// it gets its own SQLite handle — the plugin package's db.* globals
// preclude in-process parallelism.
type matrixJob struct {
	class string
	level int
	zone  string
	rep   int
}

func runMatrix(classes, levels, zones string, runs int, bank float64, cap, days int, includeLog bool, jobs int, trace bool, petLevel int) {
	cs := splitNonEmpty(classes)
	ls := parseLevels(levels)
	zs := splitNonEmpty(zones)
	if len(cs) == 0 || len(ls) == 0 || len(zs) == 0 || runs <= 0 {
		fail("matrix mode requires non-empty -classes, -levels, -zones and runs > 0")
	}
	if jobs <= 0 {
		jobs = runtime.NumCPU()
	}
	exe, err := os.Executable()
	if err != nil {
		fail("os.Executable:", err)
	}

	work := make([]matrixJob, 0, len(cs)*len(ls)*len(zs)*runs)
	for _, c := range cs {
		for _, lv := range ls {
			for _, z := range zs {
				for r := 0; r < runs; r++ {
					work = append(work, matrixJob{class: c, level: lv, zone: z, rep: r})
				}
			}
		}
	}

	workCh := make(chan matrixJob)
	resCh := make(chan *plugin.SimResult, len(work))
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go matrixWorker(exe, workCh, resCh, &wg, bank, cap, days, includeLog, trace, petLevel)
	}
	go func() {
		for _, j := range work {
			workCh <- j
		}
		close(workCh)
	}()
	go func() {
		wg.Wait()
		close(resCh)
	}()

	enc := json.NewEncoder(os.Stdout)
	for r := range resCh {
		_ = enc.Encode(r)
	}
}

func matrixWorker(exe string, in <-chan matrixJob, out chan<- *plugin.SimResult, wg *sync.WaitGroup, bank float64, cap, days int, includeLog, trace bool, petLevel int) {
	defer wg.Done()
	for j := range in {
		uid := fmt.Sprintf("@sim:%s-l%d-%s-%d", j.class, j.level, j.zone, j.rep)
		dir, err := os.MkdirTemp("", "expedition-sim-")
		if err != nil {
			out <- &plugin.SimResult{UserID: uid, Class: j.class, Level: j.level, Zone: j.zone, Outcome: "halted"}
			continue
		}
		args := []string{
			"-class", j.class,
			"-level", strconv.Itoa(j.level),
			"-zone", j.zone,
			"-bank", strconv.FormatFloat(bank, 'f', -1, 64),
			"-cap", strconv.Itoa(cap),
			"-days", strconv.Itoa(days),
			"-data", dir,
			"-user", uid,
			fmt.Sprintf("-log=%t", includeLog),
			fmt.Sprintf("-pet-level=%d", petLevel),
		}
		if trace {
			args = append(args, "-trace")
		}
		cmd := exec.Command(exe, args...)
		stdout, runErr := cmd.Output()
		var res plugin.SimResult
		if jerr := json.Unmarshal(stdout, &res); jerr != nil {
			res = plugin.SimResult{UserID: uid, Class: j.class, Level: j.level, Zone: j.zone, Outcome: "halted"}
		}
		if runErr != nil && res.Outcome == "" {
			res.Outcome = "halted"
		}
		if !includeLog {
			res.Log = nil
		}
		out <- &res
		_ = os.RemoveAll(dir)
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
