# WoW Forever Sim (fork of wowsims/sod)

Built on [WoWSims Season of Discovery](https://github.com/wowsims/sod) (MIT). Original project: https://wowsims.github.io/sod

## Status

| Piece | State |
|---|---|
| Engine builds, SoD regression tests pass | Done |
| `forever_ruleset` player flag (proto + engine) | Done |
| Forever disables all SoD runes (every class) | Done |
| Warlock: DoTs (and now drains) can crit (confirmed Forever change) | Done, verified by test |
| Warlock Forever rotations (affliction, destruction) | Done, vanilla spell ranks |
| Placeholder pre-raid gear set | Done, auto-picked dungeon/crafted gear |
| Forever talent trees, all 9 classes (beta build 1.60.1.70170) | Captured: `assets/db_inputs/forever/talents/` |
| Warlock Forever talents in the engine (Pandemic, Malediction, Soul Siphon, Wrack, Incinerate, Agonizing Flames, Shadow and Flame, ...) | Done; other classes still sim with Classic talents |
| Forever spell numbers (base damage cut, new DoT coefficients) for ~60 class spells | Done: `assets/db_inputs/forever/spell_tuning.csv` |
| Bane of Agony (off the curse slot) | Blocked: unconfirmed |
| Forever items (new dungeons/raids) | Waiting on launch (Blizzard encrypts them); import tool ready: `tools/forever_items` |
| Web UI: "WoW Forever Rules" toggle (Settings > Other), Forever presets as defaults | Done, verified in browser |
| Generic level-60 boss as default target | Done |
| Windows launcher (`wowforever-sim-windows.exe`) | Done |
| Every class: 14 DPS/tank specs with Forever rotations, talents, pre-raid gear (`tools/forever/gen_specs.py`) | Done, engine audit passes |
| 1,876 vanilla items merged from wowsims/classic; Manual Crowd Pummeler + Rivenspike effects ported | Done |
| Quick Sim page (`ui/forever/`): class -> spec -> Sim, addon paste import, ability breakdown, Forever-vs-Classic delta, best-stat finder | Done, browser-tested every spec |
| Healer sims (Resto Druid/Shaman, Holy Paladin, Holy Priest) | Blocked: healing spells not implemented in the engine |
| Every level 10-60 (Quick Sim level picker, per-level talents/gear/boss) | Done, every spec audited at 10-60 |
| Per-level base stats, crit/dodge ratios (1-60) | Done: cmangos classic-db + vmangos sniffed data, anchored to engine values (`tools/forever/gen_levels.py`) |
| Spell ranks by level + rotation rank fallback | Done: vanilla spell table ranks (`tools/forever/gen_spell_ranks.py`) |
| Top Gear, hosted site | Not started |

## Run it

Windows: run `wowforever-sim-windows.exe`. It serves the sim locally and opens the Quick Sim page in your browser.

From source:

```sh
make wowsimsod && ./wowsimsod   # opens http://localhost:3333/sod/warlock
```

Engine comparison test:

```sh
go test --tags=with_db ./sim/warlock/dps -run TestForever -v
```

Prints each spec's DPS under vanilla rules vs Forever rules on the same character,
and fails if DoT crits ever appear without the Forever flag (or vanish with it).

## Where things live

- `proto/api.proto` - `Player.forever_ruleset`
- `sim/core/character.go` - `Character.Forever`; runes return false in Forever
- `sim/warlock/forever.go` - Forever warlock rules and the TODO list of unmodeled changes
- `ui/warlock/apls/forever/` - rotations
- `ui/warlock/gear_sets/forever/` - placeholder gear
- `sim/warlock/dps/forever_test.go` - comparison harness

## Local build notes

The `replace` block at the bottom of `go.mod` points Go modules at their GitHub
mirrors because the sandbox this was built in blocks golang.org / gopkg.in.
Delete it on a normal machine.

## Regenerating spec presets

```sh
FOREVER_DUMP=1 go test --tags=with_db ./sim -run TestForeverDumpKnownSpells   # spells the engine knows per spec
python3 tools/forever/gen_specs.py /path/to/wowsims-classic                    # rotations, gear, ui/core/forever/specs.json
go test --tags=with_db ./sim -run TestForeverAllSpecs -v                      # audit every spec
```

## Leveling support (levels 10-60)

The SoD engine only had data for levels 25/40/50/60. For WoW Forever:

- `sim/core/base_stats_forever_gen.go` (generated): base attributes/health/mana per level from
  cmangos classic-db, agility-per-crit/dodge from vmangos' sniffed tables, intellect-per-spell-crit
  from the client game table. Engine values at 25/40/50/60 are reproduced exactly.
- `sim/core/forever_levels.go`: `AtLevel` for the ~130 bracket-keyed tables (spell IDs pick the exact
  rank learned at that level; damage values blend between brackets), buff tables for every level,
  pet/form stat blending, the rotation rank fallback, and `ForeverBossTarget`.
- Fixed along the way: paladin seal/Holy Shield loop-variable capture (crashes at in-between levels),
  primary-seal action when the seal isn't learned yet, Windfury Totem buff ID in the Enhancement
  rotation (it re-dropped the totem ~57 times per fight, at every level), no-op buff totem casts,
  Shadowfiend (SoD skill book) appearing in Forever, invalid 71-point talent strings for Bear/healers.
- `forever_classic_combat_rules` keeps the Forever class kit but turns off Forever's combat changes,
  so the page's "worth X% vs Classic" compares rules only.

```sh
python3 tools/forever/gen_levels.py        # per-level stats
python3 tools/forever/gen_spell_ranks.py   # spell rank families
go test --tags=with_db ./sim -run TestForeverAllLevels -v   # every spec, levels 10-60
```

## Forever talents and spell numbers

- `assets/db_inputs/forever/talents/raw/<class>.txt` - the talent calculator (wow-forever.gg, beta build
  1.60.1.70170) as text: tree, tier, column, name, max rank, prerequisite, tooltip.
  `tools/forever/gen_talents.py` turns these into per-class JSON, `ui/core/forever/talents.json`
  and `sim/core/forever_talents_gen.go`.
- `Player.forever_talents_string` (proto) carries a talent string in Forever's trees. The engine
  parses it by talent name (`sim/core/forever_talents.go`); a class opts in by mapping names to
  effects (`sim/warlock/forever_talents.go`). Without it, the Classic `talents_string` is used.
- `tools/forever/gen_forever_builds.py` holds each spec's Forever pick order and writes the level
  10-60 builds into `ui/core/forever/specs.json`. Only Warlock has one so far.
- `assets/db_inputs/forever/spell_tuning.csv` - Forever's max-rank spell numbers vs Classic
  (foreverchanges.pro spellbooks, Blizzard beta forum measurements). `tools/forever/gen_spell_tuning.py`
  scales every rank by the ratio and sets measured coefficients (`sim/core/forever_spell_tuning*.go`).
  Applies only with the Forever ruleset.

Known gaps: per-rank numbers below max rank are scaled, not measured; Forever's new spells
(Incinerate ranks 1-2, Wrack mana cost) use beta tooltips and estimates; Bane of Agony still
shares the curse slot; Curse of Shadow is still castable though Forever removed it.

## Updating when a new build or the live game ships (Nov 4)

On your own computer (needs normal internet):

```sh
python3 tools/forever/update_forever_data.py talents     # re-pull trees, prints what changed
python3 tools/forever/update_forever_data.py spells      # saves spellbook pages (send to Claude to update spell_tuning.csv)
python3 tools/forever/update_forever_data.py items --branch <wowhead branch>   # after launch, item stats
python3 tools/forever/update_forever_data.py rebuild     # regenerate, test, build
```

Every download is saved under `assets/db_inputs/forever/pages/<date>/`. If a site changed its
layout and a parser can't read it, the script says which pages to send over.
For items, find Wowhead's branch name from any Forever item URL (`wowhead.com/<branch>/item=...`).
