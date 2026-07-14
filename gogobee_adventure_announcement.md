# Adventure — the big update (2026-07-09 → 2026-07-11)

62 commits, ~31k lines, 7 phases. All merged to `main` and live in prod as of
2026-07-11. Working-tree doc; do not commit.

---

## The one-line version

Adventure stopped being a solo grind. You can now bring friends, fight each
other, fight the world together, prestige past the level cap, and there's an
actual story running underneath it — and Pete reports on all of it.

---

## 1. Co-op — you can bring people now (N3)

The headline. The combat engine was 1-versus-1 at its core; it was rewritten
into an N-player engine with real initiative, and every seat gets its own turn.

- `!expedition invite @user` — leader, Day 1 only
- `!expedition accept` / `decline` / `party` / `leave`
- Everyone buys their own supply pack. Everyone acts on their own turn.
- The enemy's action economy scales to the party, so four people don't trivialize
  a room.

## 2. Fight each other (N6)

- `!duel @user [stake]`, then `!duel accept` / `decline` / `status`.
  Staked, and **nobody dies** — it's a bout, not a mugging.
- `!rivals` for your record, `!rivals board` for room-wide standings.

## 3. Fight the world together (N6)

- `!adventure worldboss` — the Siege. One communal bout a day; `worldboss fight`
  to take your swing at it. Everybody's damage counts toward the same health bar.

## 4. The Shadow (N6)

- `!adventure shadow` — a rival adventurer who runs the dungeon on their own
  schedule whether you play or not, and you can check how your run stacks up
  against theirs.

## 5. The Town (N4)

Housing finally pays off, and the room has a civic life.

- **T3 payoffs** — trophy hall + workshop; a long rest at home now grants a
  **well-rested** buff that scales with your house tier.
- **T4 Estate vault** — `!adventure vault` / `vault store <item>` / `vault take
  <item>`. 10 protected slots. Safe from `!sell all`, safe from combat.
- **A second pet** at Tier 4 — it shows up on its own.
- **Gifting** — `!give <item> @user`. Small handling fee to the community pot.
- **Registries** — `!town` (civic pride, housing street, pet showcase),
  `!graveyard` (recent deaths), `!rivals`.

## 6. Prestige and a world that moves (N7)

- **Renown** — XP past L20 no longer evaporates. It accrues into a Renown rank
  that shows on your `!sheet` and gets announced when it ticks up.
- **The Omen** — one rotating world modifier per week. TwinBee tells you what's
  in the air in the morning DM.
- **Seasonal events** — holiday weeks re-skin the day's events and flavor.
- **Achievements** — a whole Renown wing added to `!achievements`.

## 7. The chase (N2)

- **Tempering** — `!adventure temper` pushes a magic item one rarity step higher
  at the blacksmith. Euros plus a T5 material (one per clear — *that's* the real
  gate).
- **Arena seasons** — `!arena leaderboard` is now the current season; champions
  get titles. `!arena stats` keeps your lifetime record.

## 8. Story (N5 — the Hollow King)

- **`!adventure journal`** — recover campaign pages through the zones and read
  them back. Bosses now have epilogues, and there's a real finale.
- NPC arcs land as DMs mid-run — Misty, Robbie, and Thom all have somewhere to
  go now.

## 9. Backtracking (C5)

- **`!revisit <N>`** — walk back one room from the Path strip in `!map` for +1
  threat. Blocked mid-fight, at a fork, and when it's too hot to double back.

## 10. Pete reports on it (Pete Adventure News)

Guild events — zone firsts, deaths, achievements — are now narrated into a live
news feed by Pete, the deadpan announcer.

- `news.parodia.dev/adventure` (feed, permalinks, daily digest)
- Live beats in the games room
- `!news` to read; `!news optout` if you'd rather not be named.

## 11. Restoration + plumbing (N1, unglamorous but real)

- The drop systems Phase R orphaned are wired back up; milestone rewards that
  were stubs actually pay out.
- Lottery ticket sales fund the community pot.
- Fixed outside Adventure but shipped alongside: URL previews stopped dropping
  thumbnails, and the bot stopped reminting its cross-signing identity on every
  restart.

---

## ⚠️ Do NOT put in the announcement

- **Secret rooms and the cross-zone keys** (Sunken Sigil, Underforge Seal). These
  are discovery content — no command, found by walking. Announcing them deletes
  the discovery.
- **The NPC buffs** (Misty/Arina). Hidden mechanics by design. Say the NPCs have
  arcs; do not say what befriending them *does*.
- Numbers, rolls, and rarity math generally — house style is verbs and outcomes,
  not crunch.

## Watch list (first live exercise in prod)

Renown, the Omen, seasonal events, the Hollow King arc, the world boss and duels
have **never fired against real players before today**. Expect the first firings
to be the real test.
