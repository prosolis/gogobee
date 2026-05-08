# GogoBee — Subclass System
> **Companion to:** `gogobee_dnd_design_doc.md`
> **Version:** 1.0
> **Trigger:** Level 5 subclass selection prompt

---

## 1. Selection

At Level 5, players receive a subclass selection prompt. Choice is permanent unless `!respec` is used (cooldown: 30 days; cost: 500 coins).

```
⚔️ You have reached Level 5.
   Choose your path:

   [Fighter]
   [1] Champion    — Raw power, brutal efficiency
   [2] Battle Master — Tactical superiority, maneuver control
   [3] Berserker   — Rage-fueled destruction, high risk/reward

   Reply with a number or name.
```

---

## 2. Fighter Subclasses

### Champion
*Raw power. Higher crit chance. Wins by hitting harder and surviving longer.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Improved Critical** | Crit on natural 19 or 20 (instead of 20 only) |
| 7 | **Remarkable Athlete** | +proficiency bonus to STR/DEX/CON checks not already proficient |
| 10 | **Additional Fighting Style** | Choose a second fighting style: Dueling (+2 melee damage one-handed), Great Weapon (reroll 1s and 2s on damage), Archery (+2 ranged attack rolls), Defense (+1 AC in armor) |
| 15 | **Superior Critical** | Crit on 18, 19, or 20 |

### Battle Master
*Tactical control. Maneuver dice fuel special attacks that debuff, reposition, or devastate enemies.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Maneuver Dice** | Gain 4d8 Superiority Dice per short rest. Fuel maneuvers. |
| 5 | **Maneuvers (pick 3)** | See maneuver list below |
| 7 | **Know Your Enemy** | After 1 min observation: learn 2 of target's stats vs. yours |
| 10 | **Improved Maneuvers** | Superiority Dice become d10; learn 2 more maneuvers |
| 15 | **Relentless** | On initiative if no Superiority Dice: regain 1 |

**Battle Master Maneuvers (choose 3 at Level 5, +2 at Level 10):**

| Maneuver | Cost | Effect |
|---|---|---|
| Disarming Attack | 1 die | On hit: +die damage; STR save or drop weapon (disadvantage next attack) |
| Tripping Attack | 1 die | On hit: +die damage; STR save or Prone |
| Pushing Attack | 1 die | On hit: +die damage; STR save or pushed 15 ft |
| Menacing Attack | 1 die | On hit: +die damage; WIS save or Frightened until end of your next turn |
| Precision Attack | 1 die | Add die to one attack roll (before or after rolling) |
| Parry | 1 die | Reaction: reduce damage taken by die + DEX mod |
| Rally | 1 die | Bonus action: give ally temp HP equal to die + CHA mod |
| Goading Attack | 1 die | On hit: +die damage; WIS save or disadvantage on attacks against anyone except you |
| Riposte | 1 die | Reaction when missed: make one melee attack; add die to damage |
| Commander's Strike | 1 die | Bonus action: ally makes one attack using Reaction; add die to their damage |

### Berserker
*Rage. More rage. Everything hurts including you but you hit harder.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Frenzy** | While Raging: make one bonus melee attack per turn; after rage: gain 1 Exhaustion level |
| 5 | **Rage** | Bonus action: +2 melee damage; resistance to physical damage; advantage on STR checks/saves; 1 min; 3 uses per long rest |
| 7 | **Mindless Rage** | While Raging: immune to Charmed and Frightened |
| 10 | **Intimidating Presence** | Action: WIS DC (8+prof+CHA) or target Frightened 1 min |
| 15 | **Retaliation** | Reaction: when taking damage, make one melee attack against attacker |

---

## 3. Rogue Subclasses

### Thief
*Speed and opportunism. Best at looting, pickpocketing, and using items faster than anyone.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Fast Hands** | Bonus action: Sleight of Hand check, use thieves' tools, or Use Object |
| 5 | **Second Story Work** | Climb speed = full speed; running jump distance +DEX mod ft |
| 7 | **Supreme Sneak** | Advantage on Stealth if moving at half speed |
| 10 | **Use Magic Device** | Ignore class/race restrictions on magic items |
| 15 | **Thief's Reflexes** | Take two turns in first combat round (normal initiative + initiative -10) |

### Assassin
*Setup and execution. Pre-combat advantage pays off in devastating opening strikes.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Assassinate** | Advantage vs. creatures that haven't acted yet; crits vs. surprised targets |
| 5 | **Infiltration Expertise** | Craft a false identity (useful for NPC skill checks) |
| 7 | **Impostor** | Perfectly mimic voice/behavior after 3 hours study |
| 10 | **Death Strike** | On crit vs. surprised target: target must CON save (DC 8+prof+DEX) or take double damage |
| 15 | **Shadow Step (Assassin)** | Bonus action teleport between dim/dark areas up to 60 ft |

### Arcane Trickster
*Magic plus stealth. The most versatile Rogue build; some spellcasting.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Spellcasting** | Gain Mage cantrips (2) and 1st level spells (3 slots); INT-based |
| 5 | **Mage Hand Legerdemain** | Mage Hand is invisible; can pickpocket, disarm traps, retrieve items remotely |
| 7 | **Magical Ambush** | If hidden when casting: targets have disadvantage on the save |
| 10 | **Versatile Trickster** | Use Mage Hand to give yourself advantage on one creature's attack roll or ability check |
| 15 | **Spell Thief** | Reaction: steal a spell just cast against you (INT save to resist for caster) |

---

## 4. Mage Subclasses

### Evocation
*Pure damage output. The glass cannon perfected.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Sculpt Spells** | When casting AoE: choose up to (1 + spell level) creatures to auto-succeed the save |
| 5 | **Potent Cantrip** | Cantrip saves that succeed still deal half damage |
| 7 | **Empowered Evocation** | Add INT mod to damage of all Mage evocation spells |
| 10 | **Overchannel** | Maximize damage dice of a 1st–5th level spell (once per turn; above 1st level = take 2d12 necrotic per level above 1st) |
| 15 | **Sculptural Mastery** | Sculpt Spells now affects double the number of creatures |

### Abjuration
*Defensive Mage. Wards, shields, and counterspells.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Arcane Ward** | When casting abjuration spell: create ward with HP = 2× Mage level; absorbs damage before player does |
| 5 | **Projected Ward** | Use Reaction: extend Arcane Ward to protect one ally hit by attack |
| 7 | **Improved Abjuration** | Proficiency bonus to Arcane Ward HP; bonus to Counterspell/Dispel Magic checks |
| 10 | **Spell Resistance** | Advantage on saves vs. spells; resistance to spell damage |
| 15 | **Spell Reflection** | Counterspell can redirect the spell back at the caster |

### Necromancy
*Control undead and drain life. High synergy with Crypt of Valdris zone.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Grim Harvest** | When killing with a spell: heal HP equal to twice spell level (three times for necrotic) |
| 5 | **Undead Thralls** | Animate Dead creates one extra undead; undead gain +proficiency bonus to damage |
| 7 | **Inured to Undeath** | Resistance to necrotic damage; max HP can't be reduced |
| 10 | **Command Undead** | Attempt to take control of any undead (CHA save DC 8+prof+INT) |
| 15 | **Improved Undead Thralls** | Undead thralls gain +CON mod to HP and attack bonus |

---

## 5. Cleric Subclasses

### Life Domain
*Maximum healing output. Party support anchor.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Disciple of Life** | Healing spells restore additional HP equal to 2 + spell level |
| 5 | **Preserve Life** | Channel Divinity: restore HP to any creatures within 30 ft up to half max HP; total pool = 5× Cleric level |
| 7 | **Blessed Healer** | When casting healing spell on ally: heal yourself for 2–5 HP |
| 10 | **Divine Strike** | Once per turn: +1d8 radiant on weapon hit (+2d8 at Level 14) |
| 15 | **Supreme Healing** | Instead of rolling healing spell dice: use maximum value |

### War Domain
*Combat Cleric. Extra attacks, martial weapons, armor.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **War Priest** | Bonus action: make one weapon attack; usable WIS mod times per long rest |
| 5 | **Guided Strike** | Channel Divinity: +10 to one attack roll (after rolling, before resolving) |
| 7 | **War God's Blessing** | Reaction: grant ally +10 to their attack roll |
| 10 | **Divine Strike** | +1d8 weapon-type damage on weapon hit (+2d8 at Level 14) |
| 15 | **Avatar of Battle** | Resistance to bludgeoning, piercing, slashing from non-magical weapons |

### Trickery Domain
*Illusion, misdirection, and chaos. High-risk high-reward Cleric.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Blessing of the Trickster** | Give one creature advantage on Stealth for 1 hr (1/rest) |
| 5 | **Invoke Duplicity** | Channel Divinity: create illusory duplicate; cast spells from its position; +advantage on attacks vs. creatures adjacent to it |
| 7 | **Cloak of Shadows** | Become Invisible as action (until attack, spell, or end of next turn) |
| 10 | **Divine Strike** | +1d8 poison damage on weapon hit |
| 15 | **Improved Duplicity** | Spawn up to 4 duplicates; each ally adjacent to a duplicate has advantage on attacks |

---

## 6. Ranger Subclasses

### Hunter
*Zone specialist. Bonuses against specific enemy types stack with zone familiarity.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Hunter's Prey** | Choose one: Colossus Slayer (+1d8 vs. target below max HP), Giant Killer (Reaction attack when Large+ misses you), Horde Breaker (attack second adjacent creature once per turn) |
| 7 | **Defensive Tactics** | Choose one: Escape the Horde (OA against you have disadvantage), Multiattack Defense (+4 AC after being hit by multiattack), Steel Will (advantage vs. Frightened) |
| 10 | **Multiattack** | Choose one: Volley (ranged attack all creatures in 10 ft radius) or Whirlwind Attack (melee attack all within 5 ft) |
| 15 | **Superior Hunter's Defense** | Choose one: Evasion, Stand Against the Tide (on OA hit: redirect to different adjacent creature), Uncanny Dodge |

### Beast Master
*Deep Ranger–pet bond. Pet becomes a true combat partner.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Ranger's Companion** | Pet gains: +proficiency to attack/saves; acts on your initiative; follows tactical commands |
| 5 | **Exceptional Training** | Bonus action: command pet to Dash, Disengage, Dodge, or Help |
| 7 | **Bestial Fury** | Pet makes 2 attacks when commanded to attack |
| 10 | **Share Spells** | Spells targeting self also affect pet if within 30 ft |
| 15 | **Superior Bond** | Pet can't be charmed or frightened; pet survives death (returns at next long rest at 1 HP) |

### Gloom Stalker
*Darkness specialist. Exceptional in underground zones and night phases.*

| Level | Ability | Effect |
|---|---|---|
| 5 | **Dread Ambusher** | +initiative bonus equal to WIS mod; +1 attack in first combat round; +1d8 damage on that extra attack |
| 5 | **Umbral Sight** | Darkvision 60 ft; creatures with darkvision can't see you in darkness |
| 7 | **Iron Mind** | Proficiency in WIS saves (or +1 if already proficient) |
| 10 | **Stalker's Flurry** | When you miss: immediately make another attack against same target |
| 15 | **Shadowy Dodge** | Reaction when attacked: impose disadvantage on the attack |

---

## 7. Subclass Flavor Prompts

Each subclass selection triggers a TwinBee narration line and a one-line flavor acknowledgment in the player's character sheet. Examples:

- **Champion:** *"You've chosen the old way. No tricks, no systems. Just you, and the blade, and the next hit. TwinBee approves of this directness."*
- **Arcane Trickster:** *"Magic in the hands of a Rogue. TwinBee considers this the universe's way of apologizing for giving you low HP."*
- **Necromancer:** *"Undeath as a tool rather than a state. TwinBee has opinions about this. TwinBee is keeping them to itself."*
- **Beast Master:** *"You and your pet, all the way down. TwinBee finds this genuinely moving and declines to be embarrassed about that."*
- **Gloom Stalker:** *"You were already good in the dark. Now you're better. TwinBee notes: the Underdark is waiting."*

---

## 8. Implementation Phases

### Phase SUB1 — Selection System
- [ ] Level 5 subclass prompt (per class)
- [ ] Subclass record stored on character sheet
- [ ] `!respec` command (cooldown + cost)

### Phase SUB2 — Ability Implementation
- [ ] All Level 5 abilities per subclass
- [ ] Level 7 abilities per subclass
- [ ] Berserker Rage mechanic and Exhaustion tracking

### Phase SUB3 — Advanced Abilities
- [ ] Level 10 and 15 abilities per subclass
- [ ] Battle Master maneuver dice system
- [ ] Arcane Trickster spellcasting integration
- [ ] Beast Master full combat pet participation

---
*End of Subclass System.*
