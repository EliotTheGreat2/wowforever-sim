package core

// WoW Forever: support for every character level, not just the 25/40/50/60 brackets
// Season of Discovery uses. Measured per-level data is generated into
// base_stats_forever_gen.go; this file holds the helpers and the level blending used
// for tables that only exist at the brackets (buffs, pets, forms).

import (
	"math"

	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// ForeverBrackets are the levels the engine has hand-measured data for.
var ForeverBrackets = []int32{25, 40, 50, 60}

func foreverFillBaseStats(class proto.Class, levels map[int]stats.Stats) {
	for level, st := range levels {
		if _, ok := ClassBaseStats[class][level]; !ok {
			ClassBaseStats[class][level] = st
		}
	}
}

func foreverFillRatios(table map[proto.Class]map[int]float64, class proto.Class, levels map[int]float64) {
	for level, v := range levels {
		if _, ok := table[class][level]; !ok {
			table[class][level] = v
		}
	}
}

// LevelBlend returns the two measured brackets around level and how far level sits
// between them (0 = lo, 1 = hi). Below 25 it extrapolates from the 25-40 slope;
// callers clamp the result.
func LevelBlend(level int32) (lo, hi int32, w float64) {
	switch {
	case level >= 60:
		return 60, 60, 0
	case level <= 25:
		return 25, 40, float64(level-25) / 15
	}
	for i := 0; i+1 < len(ForeverBrackets); i++ {
		lo, hi = ForeverBrackets[i], ForeverBrackets[i+1]
		if level >= lo && level <= hi {
			return lo, hi, float64(level-lo) / float64(hi-lo)
		}
	}
	return 60, 60, 0
}

// BlendStats linearly blends bracket stats for an arbitrary level, never letting a
// value fall below 20% of its lowest-bracket value (so extrapolation below 25 stays sane).
func BlendStats(level int32, at func(bracket int32) stats.Stats) stats.Stats {
	lo, hi, w := LevelBlend(level)
	if w == 0 {
		return at(lo)
	} else if w == 1 {
		return at(hi)
	}
	a, b := at(lo), at(hi)
	var out stats.Stats
	for i := range out {
		v := a[i] + (b[i]-a[i])*w
		if floor := 0.2 * a[i]; a[i] > 0 && v < floor {
			v = floor
		}
		out[i] = v
	}
	return out
}

// BlendFloat is BlendStats for a single number.
func BlendFloat(level int32, at func(bracket int32) float64) float64 {
	lo, hi, w := LevelBlend(level)
	if w == 0 {
		return at(lo)
	} else if w == 1 {
		return at(hi)
	}
	a, b := at(lo), at(hi)
	v := a + (b-a)*w
	if a > 0 && v < 0.2*a {
		v = 0.2 * a
	}
	return v
}

// FloorBracket returns the highest measured bracket at or below level (25 for anything lower).
// Used for rank-based lookups: a level 47 character has the ranks it had at 40.
func FloorBracket(level int32) int32 {
	best := ForeverBrackets[0]
	for _, b := range ForeverBrackets {
		if b <= level {
			best = b
		}
	}
	return best
}

func init() {
	// Buff values exist only at the brackets. Blend them for every level 1-60
	// (below 25 they scale down toward zero with level, since early ranks are weak).
	for name, byLevel := range BuffSpellByLevel {
		if len(byLevel) == 0 {
			continue
		}
		for level := int32(1); level <= 60; level++ {
			if _, ok := byLevel[level]; ok {
				continue
			}
			lo, hi, w := LevelBlend(level)
			a, okA := byLevel[lo]
			b, okB := byLevel[hi]
			switch {
			case okA && okB && level > 25:
				var st stats.Stats
				for i := range st {
					st[i] = math.Floor(a[i] + (b[i]-a[i])*w)
				}
				byLevel[level] = st
			case okA && level < 25:
				byLevel[level] = a.Multiply(float64(level) / 25).Floor()
			default:
				// Only some brackets defined (e.g. a buff first learned at 50): use the highest defined below.
				for l := level; l >= 1; l-- {
					if st, ok := byLevel[l]; ok {
						byLevel[level] = st
						break
					}
				}
			}
		}
		BuffSpellByLevel[name] = byLevel
	}
	for _, byLevel := range LevelToBuffRank {
		lowest := int32(0)
		for l := int32(1); l <= 60 && lowest == 0; l++ {
			if _, ok := byLevel[l]; ok {
				lowest = l
			}
		}
		for level := int32(1); level <= 60; level++ {
			if _, ok := byLevel[level]; ok {
				continue
			}
			// The rank known at the highest measured level at or below this one. Below the first
			// entry, use rank 1 of that table: callers already gate on the spell's learn level
			// (e.g. Windfury Totem at 32 while its table starts at 40).
			byLevel[level] = byLevel[lowest]
			for l := level; l >= 1; l-- {
				if r, ok := byLevel[l]; ok && l != level {
					byLevel[level] = r
					break
				}
			}
		}
	}
}

// AtLevel reads a table measured only at the 25/40/50/60 brackets for any level.
//   - Whole-number values (spell IDs, ranks) come from the highest bracket at or below level,
//     i.e. the rank the character already had.
//   - Decimal values (damage, threat) blend between the two nearest brackets. Below 25,
//     values that grow strongly with level (damage-like) scale down with level; flat values
//     (costs, coefficients) keep their level-25 number.
func AtLevel[V float64 | int32 | int](level int32, byBracket map[int32]V) V {
	if v, ok := byBracket[level]; ok {
		return v
	}
	floor := func() V {
		var best V
		found := false
		for _, b := range ForeverBrackets {
			if v, ok := byBracket[b]; ok && (b <= level || !found) {
				best, found = v, true
			}
		}
		return best
	}
	var zero V
	if _, isFloat := any(zero).(float64); !isFloat {
		v := floor()
		// Spell-ID tables: return the exact rank this level has learned, when rank data exists.
		if id, isID := any(v).(int32); isID && id > 0 {
			if exact := ForeverRankForLevel(id, level); exact != 0 {
				return any(exact).(V)
			}
		}
		return v
	}
	get := func(b int32) (float64, bool) {
		v, ok := byBracket[b]
		return float64(v), ok
	}
	v25, ok25 := get(25)
	v40, ok40 := get(40)
	if level < 25 && ok25 {
		if ok40 && v25 > 0 && v40 >= 1.3*v25 {
			return V(v25 * float64(level) / 25)
		}
		return V(v25)
	}
	lo, hi, w := LevelBlend(level)
	a, okA := get(lo)
	b, okB := get(hi)
	if !okA || !okB {
		return floor()
	}
	return V(a + (b-a)*w)
}

var (
	foreverRankIndex map[int32][]int32 // spell ID -> family IDs, highest rank first
	foreverLearnAt   map[int32]int32   // spell ID -> level the rank is learned
)

func foreverRanksInit() {
	if foreverRankIndex != nil {
		return
	}
	foreverRankIndex = make(map[int32][]int32)
	foreverLearnAt = make(map[int32]int32)
	for _, family := range foreverSpellRanks {
		ids := make([]int32, len(family))
		for i, entry := range family {
			ids[i] = entry[0]
			foreverLearnAt[entry[0]] = entry[2]
		}
		for _, id := range ids {
			foreverRankIndex[id] = ids
		}
	}
}

// ForeverRankAlternatives returns the other ranks of a spell, highest first.
func ForeverRankAlternatives(spellID int32) []int32 {
	foreverRanksInit()
	return foreverRankIndex[spellID]
}

// ForeverRankForLevel returns the highest rank of spellID's family learned by level,
// or 0 if the spell has no rank data or no rank is learned yet.
func ForeverRankForLevel(spellID int32, level int32) int32 {
	foreverRanksInit()
	for _, id := range foreverRankIndex[spellID] {
		if foreverLearnAt[id] <= level {
			return id
		}
	}
	return 0
}

// isPlayerCastable reports whether a spell is something a player presses (flagged for
// rotations, or it has a GCD, cast time or cost), as opposed to a proc or judgement effect
// sharing its name.
func (spell *Spell) isPlayerCastable() bool {
	return spell.Flags.Matches(SpellFlagAPL) || spell.DefaultCast.GCD > 0 || spell.DefaultCast.CastTime > 0 || spell.Cost != nil
}

// ForeverBossTarget returns a generic dungeon/raid boss for a player of the given level:
// two levels higher (three at 60), with armor and melee damage blended from the generic
// "SoD/Level N" boss presets (25: 1104 / 40: 2053 / 50: 3137 / 60: 3731 armor).
func ForeverBossTarget(level int32) *proto.Target {
	armor := map[int32]float64{25: 1104, 40: 2053, 50: 3137, 60: 3731}
	damage := map[int32]float64{25: 400, 40: 1000, 50: 2000, 60: 3000}
	ap := map[int32]float64{25: 574, 40: 574, 50: 574, 60: 805}
	scaled := func(m map[int32]float64) float64 {
		if level < 25 {
			return m[25] * float64(level) / 25
		}
		return AtLevel(level, m)
	}
	targetLevel := level + 2
	if level >= 60 {
		targetLevel = 63
	}
	return &proto.Target{
		Level: targetLevel,
		Stats: stats.Stats{
			stats.Armor:       math.Round(scaled(armor)),
			stats.AttackPower: AtLevel(level, ap),
			stats.Health:      127393,
		}.ToFloatArray(),
		MobType:       proto.MobType_MobTypeDemon,
		SwingSpeed:    2,
		MinBaseDamage: math.Round(scaled(damage)),
		ParryHaste:    true,
		DamageSpread:  0.3333,
	}
}
