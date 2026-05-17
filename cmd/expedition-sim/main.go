// expedition-sim drives synthetic players through expeditions for batch
// analysis. Re-uses production plugin paths against a fresh sqlite DB so
// outcomes mirror what live players hit.
//
// Usage: expedition-sim [-class fighter] [-level 5] [-zone goblin_warrens]
//                       [-bank 1000] [-cap 200] [-data DIR]
//
// Defaults run one L5 fighter through goblin_warrens and print a JSON
// SimResult to stdout. Per-room logs and matrix-mode batches land in a
// follow-up; this binary is the scaffold the analysis stack hangs off.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"gogobee/internal/plugin"

	"maunium.net/go/mautrix/id"
)

func main() {
	var (
		class   = flag.String("class", "fighter", "DnD class id (fighter, mage, cleric, …)")
		level   = flag.Int("level", 5, "character level")
		zone    = flag.String("zone", "goblin_warrens", "zone id (see internal/plugin/dnd_zone_registry.go)")
		bank    = flag.Float64("bank", 1000, "starting coin balance — must cover outfitting")
		cap     = flag.Int("cap", 200, "max !expedition run invocations per expedition")
		dataDir = flag.String("data", "", "data dir for the temp sqlite db (default: OS tempdir)")
		userTag = flag.String("user", "@sim:expedition", "synthetic user id")
	)
	flag.Parse()

	dir := *dataDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "expedition-sim-")
		if err != nil {
			fail("mkdir temp:", err)
		}
		defer os.RemoveAll(dir)
	}

	runner, err := plugin.NewSimRunner(dir)
	if err != nil {
		fail("init runner:", err)
	}
	defer runner.Close()

	uid := id.UserID(*userTag)
	if _, err := runner.BuildCharacter(uid, plugin.DnDClass(*class), *level); err != nil {
		fail("build character:", err)
	}

	runner.Euro.Credit(uid, *bank, "expedition-sim bankroll")

	res, err := runner.RunExpedition(uid, plugin.ZoneID(*zone), *cap)
	if err != nil {
		// RunExpedition returns a partial result alongside the error so the
		// outcome of a partial run is still observable.
		if res != nil {
			emit(res)
		}
		fail("run expedition:", err)
	}
	emit(res)
}

func emit(res *plugin.SimResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
}

func fail(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}
