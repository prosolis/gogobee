// Command char-migrate mints a confirmed D&D character for one or more users,
// for the "stuck adventurer" cases the boredom ticker can't reach: veteran
// legacy players who never triggered auto-migration, and players who abandoned
// !setup at race pick. It reuses the game's own constructors (stats, HP/AC,
// resources, spells) via plugin.AdminBuildConfirmedCharacter.
//
// It opens the SAME gogobee.db the live bot uses (WAL + busy_timeout), so it is
// safe to run alongside the running process — but snapshot the db dir first.
//
// Usage:
//
//	char-migrate -data /home/reala/gogobee/data \
//	    '@nonk:parodia.dev:human:rogue:4' \
//	    '@knightstar:matrix.org:half_elf:warlock:1'
//
// Each spec is mxid:race:class:level (mxid contains colons, so we split from
// the right for the last three fields).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gogobee/internal/db"
	"gogobee/internal/plugin"

	"maunium.net/go/mautrix/id"
)

func main() {
	dataDir := flag.String("data", "", "data dir containing gogobee.db (e.g. /home/reala/gogobee/data)")
	flag.Parse()

	if *dataDir == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: char-migrate -data <dir> '<mxid>:<race>:<class>:<level>' ...")
		os.Exit(2)
	}

	if err := db.Init(*dataDir); err != nil {
		log.Fatalf("db init: %v", err)
	}

	for _, spec := range flag.Args() {
		uid, race, class, level, err := parseSpec(spec)
		if err != nil {
			log.Fatalf("bad spec %q: %v", spec, err)
		}
		c, err := plugin.AdminBuildConfirmedCharacter(uid, race, class, level)
		if err != nil {
			log.Fatalf("build %s: %v", uid, err)
		}
		fmt.Printf("OK  %-28s %s %s L%d  HP=%d AC=%d  STR%d DEX%d CON%d INT%d WIS%d CHA%d\n",
			uid, c.Race, c.Class, c.Level, c.HPMax, c.ArmorClass,
			c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA)
	}
}

// parseSpec splits mxid:race:class:level. The mxid itself contains a colon
// (@user:server), so split the last three fields off the right.
func parseSpec(spec string) (id.UserID, plugin.DnDRace, plugin.DnDClass, int, error) {
	i := strings.LastIndex(spec, ":")
	if i < 0 {
		return "", "", "", 0, fmt.Errorf("missing :level")
	}
	level, err := strconv.Atoi(spec[i+1:])
	if err != nil {
		return "", "", "", 0, fmt.Errorf("level: %w", err)
	}
	rest := spec[:i]
	j := strings.LastIndex(rest, ":")
	if j < 0 {
		return "", "", "", 0, fmt.Errorf("missing :class")
	}
	class := rest[j+1:]
	rest = rest[:j]
	k := strings.LastIndex(rest, ":")
	if k < 0 {
		return "", "", "", 0, fmt.Errorf("missing :race")
	}
	race := rest[k+1:]
	mxid := rest[:k]
	if mxid == "" || race == "" || class == "" {
		return "", "", "", 0, fmt.Errorf("empty field")
	}
	return id.UserID(mxid), plugin.DnDRace(race), plugin.DnDClass(class), level, nil
}
