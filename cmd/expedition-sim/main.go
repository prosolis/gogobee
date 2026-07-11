// expedition-sim drives synthetic players through expeditions for batch
// analysis. Re-uses production plugin paths against a fresh sqlite DB so
// outcomes mirror what live players hit.
//
// Single run:
//
//	expedition-sim [-class fighter] [-level 5] [-zone goblin_warrens]
//	               [-bank 1000] [-cap 50] [-log] [-data DIR]
//
// Party run (N3/P7 — the leader is -class; followers clone it unless named):
//
//	expedition-sim -party 3 -level 15 -zone dragons_lair
//	expedition-sim -party 2 -class cleric -party-classes fighter -level 16 ...
//
// Matrix mode (cartesian sweep over classes × levels × zones × N runs,
// one JSON object per stdout line, log suppressed by default):
//
//	expedition-sim -matrix -classes fighter,mage -levels 5,10 \
//	               -zones goblin_warrens,wolf_den -runs 20
package main

import (
	"bytes"
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

		realUser = flag.String("real-user", "", "run an EXISTING character loaded from -data's DB instead of building a synthetic one. Pass the real mxid (e.g. @holymachina:parodia.dev). Requires -data to point at a (copy of a) populated gogobee.db dir. Heals the char to full + tops up the bankroll; leaves race/class/level/gear/spells as-is.")
		logFlag = flag.Bool("log", true, "include per-row expedition log in output (single-run default true; matrix default false)")

		matrix  = flag.Bool("matrix", false, "matrix mode — sweep over classes × levels × zones × runs")
		classes = flag.String("classes", "", "comma-separated class ids (matrix mode)")
		levels  = flag.String("levels", "", "comma-separated levels (matrix mode)")
		zones   = flag.String("zones", "", "comma-separated zone ids (matrix mode)")
		runs    = flag.Int("runs", 1, "replicates per (class,level,zone) cell (matrix mode)")

		trace = flag.Bool("trace", false, "include raw per-round CombatEvent stream on the LAST combat of each expedition (boss room) — for J2 diagnostic sweeps")

		petLevel = flag.Int("pet-level", 0, "attach a base housing pet at this level (1-10) to every sim character; 0 = no pet (default, matches prod char-creation)")

		party        = flag.Int("party", 1, "seat a party of N (1 = solo, the historical path). Followers are invited on Day 1 and buy their own supplies")
		partyClasses = flag.String("party-classes", "", "comma-separated classes for the N-1 followers (default: clones of -class)")

		jobs = flag.Int("jobs", 0, "matrix mode — concurrent worker count (each worker is a subprocess so it gets its own sqlite). 0 = runtime.NumCPU()")
	)
	flag.Parse()

	if *petLevel < 0 || *petLevel > 10 {
		fail("pet-level must be 0-10, got", *petLevel)
	}
	if *party < 1 || *party > plugin.ExpeditionPartyMax {
		fail("party must be 1-", plugin.ExpeditionPartyMax, ", got", *party)
	}
	followers := followerClasses(*class, *party, *partyClasses)

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
		runMatrix(*classes, *levels, *zones, *runs, *bank, *cap, *days, includeLog, *jobs, *trace, *petLevel, *party, *partyClasses)
		return
	}

	if *realUser != "" {
		if *dataDir == "" {
			fail("-real-user requires -data pointing at a dir containing a populated gogobee.db")
		}
		if *party != 1 {
			fail("-real-user does not support -party yet (would need every seat to be a real, tier-eligible char)")
		}
		runReal(*realUser, *zone, *dataDir, *bank, *cap, *days, *logFlag)
		return
	}

	runSingle(*class, *level, *zone, *userTag, *dataDir, *bank, *cap, *days, *logFlag, followers)
}

// runReal drives an existing character loaded from dataDir's gogobee.db through
// a real expedition. dataDir should be a COPY of prod — db.Init runs additive
// migrations against it, and the run mutates HP/coin/inventory. Never point this
// at the live prod file or at ./data.
func runReal(userTag, zone, dataDir string, bank float64, cap, days int, includeLog bool) {
	runner, err := plugin.NewSimRunner(dataDir)
	if err != nil {
		fail("init runner:", err)
	}
	defer runner.Close()

	uid := id.UserID(userTag)
	c, err := runner.PrepareRealCharacter(uid, bank)
	if err != nil {
		fail("prepare real character:", err)
	}
	res, err := runner.RunExpedition(uid, plugin.ZoneID(zone), cap, days)
	if res != nil {
		res.Class = string(c.Class) // real subclass/race aren't in SimResult; class is the useful key
		if !includeLog {
			res.Log = nil
		}
		emitIndented(res)
	}
	if err != nil {
		fail("run:", err)
	}
}

// followerClasses expands -party / -party-classes into the class of each
// follower seat. An explicit list must name every seat: a party of 3 whose
// second follower silently defaulted to the leader's class would quietly
// answer a different question than the one asked.
func followerClasses(leaderClass string, party int, spec string) []string {
	n := party - 1
	if n == 0 {
		if strings.TrimSpace(spec) != "" {
			fail("-party-classes given but -party is 1")
		}
		return nil
	}
	if strings.TrimSpace(spec) == "" {
		out := make([]string, n)
		for i := range out {
			out[i] = leaderClass
		}
		return out
	}
	out := splitNonEmpty(spec)
	if len(out) != n {
		fail(fmt.Sprintf("-party %d needs %d follower classes, got %d", party, n, len(out)))
	}
	return out
}

func runSingle(class string, level int, zone, userTag, dataDir string, bank float64, cap, days int, includeLog bool, followers []string) {
	dir := dataDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "expedition-sim-")
		if err != nil {
			fail("mkdir temp:", err)
		}
		defer os.RemoveAll(dir)
	}

	res, err := runOne(dir, id.UserID(userTag), plugin.DnDClass(class), level, plugin.ZoneID(zone), bank, cap, days, followers)
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

func runMatrix(classes, levels, zones string, runs int, bank float64, cap, days int, includeLog bool, jobs int, trace bool, petLevel, party int, partyClasses string) {
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
		go matrixWorker(exe, workCh, resCh, &wg, bank, cap, days, includeLog, trace, petLevel, party, partyClasses)
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

func matrixWorker(exe string, in <-chan matrixJob, out chan<- *plugin.SimResult, wg *sync.WaitGroup, bank float64, cap, days int, includeLog, trace bool, petLevel, party int, partyClasses string) {
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
			fmt.Sprintf("-party=%d", party),
		}
		// Left empty, each cell's followers clone that cell's own -class.
		if partyClasses != "" {
			args = append(args, "-party-classes", partyClasses)
		}
		if trace {
			args = append(args, "-trace")
		}
		cmd := exec.Command(exe, args...)
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
		stdout, runErr := cmd.Output()
		var res plugin.SimResult
		jerr := json.Unmarshal(stdout, &res)
		if jerr != nil {
			res = plugin.SimResult{UserID: uid, Class: j.class, Level: j.level, Zone: j.zone, Outcome: "halted"}
		}
		if runErr != nil && res.Outcome == "" {
			res.Outcome = "halted"
		}
		// Surface subprocess failures: parse error with non-empty stdout
		// (corrupted JSON) and non-zero exits both get a stderr dump so the
		// user sees the underlying cause instead of just a halted row.
		if jerr != nil || runErr != nil {
			fmt.Fprintf(os.Stderr, "sim cell %s halted: runErr=%v jerr=%v\n", uid, runErr, jerr)
			if stderrBuf.Len() > 0 {
				fmt.Fprintf(os.Stderr, "  child stderr:\n%s\n", stderrBuf.String())
			}
			if jerr != nil && len(stdout) > 0 {
				snip := stdout
				if len(snip) > 200 {
					snip = snip[:200]
				}
				fmt.Fprintf(os.Stderr, "  child stdout (first 200B): %q\n", snip)
			}
		}
		if !includeLog {
			res.Log = nil
		}
		out <- &res
		_ = os.RemoveAll(dir)
	}
}

func runOne(dataDir string, uid id.UserID, class plugin.DnDClass, level int, zone plugin.ZoneID, bank float64, cap, days int, followers []string) (*plugin.SimResult, error) {
	runner, err := plugin.NewSimRunner(dataDir)
	if err != nil {
		return nil, fmt.Errorf("init runner: %w", err)
	}
	defer runner.Close()

	if _, err := runner.BuildCharacter(uid, class, level); err != nil {
		return nil, fmt.Errorf("build character: %w", err)
	}
	runner.Euro.Credit(uid, bank, "expedition-sim bankroll")

	// Followers share the leader's level — a party is not a carry (the tier
	// gate refuses an under-levelled invitee anyway) — and each buys their own
	// loadout, so each needs their own bankroll.
	var members []id.UserID
	for i, fc := range followers {
		muid := followerUserID(uid, i)
		if _, err := runner.BuildCharacter(muid, plugin.DnDClass(fc), level); err != nil {
			return nil, fmt.Errorf("build follower %d (%s): %w", i+1, fc, err)
		}
		runner.Euro.Credit(muid, bank, "expedition-sim bankroll")
		members = append(members, muid)
	}
	return runner.RunPartyExpedition(uid, members, zone, cap, days)
}

// followerUserID derives a follower's Matrix id from the leader's. It must keep
// the ":" — ResolveUser treats a colon as "this is already a full mxid" and
// short-circuits the room-membership lookup the sim has no client for.
func followerUserID(leader id.UserID, i int) id.UserID {
	s := string(leader)
	local, server, ok := strings.Cut(strings.TrimPrefix(s, "@"), ":")
	if !ok {
		local, server = strings.TrimPrefix(s, "@"), "sim"
	}
	return id.UserID(fmt.Sprintf("@%s-m%d:%s", local, i+1, server))
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
