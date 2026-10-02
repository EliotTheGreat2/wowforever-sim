# WoW Forever Sim (fork of wowsims/sod)

Built on [WoWSims Season of Discovery](https://github.com/wowsims/sod) (MIT). Original project: https://wowsims.github.io/sod

## Status

| Piece | State |
|---|---|
| Engine builds, SoD regression tests pass | Done |
| `forever_ruleset` player flag (proto + engine) | Done |
| Forever disables all SoD runes (every class) | Done |
| Warlock: DoTs can crit (confirmed Forever change) | Done, verified by test |
| Warlock Forever rotations (affliction, destruction) | Done, vanilla spell ranks |
| Placeholder pre-raid gear set | Done, auto-picked dungeon/crafted gear |
| Forever talent trees (Pandemic, Malediction, Improved Drains, ...) | Blocked: needs datamined values |
| Bane of Agony (off the curse slot) | Blocked: unconfirmed |
| Forever items (new dungeons/raids) | Blocked: needs datamined item data |
| Web UI: "WoW Forever Rules" toggle (Settings > Other), Forever presets as defaults | Done, verified in browser |
| Generic level-60 boss as default target | Done |
| Windows launcher (`wowforever-sim-windows.exe`) | Done |
| Character import addon, Top Gear, hosted site | Not started |

## Run it

Windows: run `wowforever-sim-windows.exe`. It serves the sim locally and opens the warlock page in your browser.

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
