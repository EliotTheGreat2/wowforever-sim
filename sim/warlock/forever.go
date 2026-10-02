package warlock

import "github.com/wowsims/sod/sim/core/proto"

// WoW Forever warlock rules.
//
// Confirmed (BlizzCon 2026 talent preview / beta notes):
//   - Damage-over-time effects can critically strike.
//
// NOT yet modeled (needs datamined values from the Forever client):
//   - Bane of Agony replacing Curse of Agony outside the curse slot
//   - New talents: Pandemic, Improved Corruption (cast time + damage), Malediction,
//     Shadow and Flame, Improved Drains, Soul Siphon, Decimation, Demonic Pact, ...
//   - Removed talents (Devastation, Emberstorm, Dark Pact, ...)
// Until those land, Forever mode runs vanilla level-60 talents with Forever-wide rules.

// DotsCanCrit reports whether this warlock's periodic damage can crit:
// under Forever's combat rules, or with the SoD Pandemic rune.
func (warlock *Warlock) DotsCanCrit() bool {
	return warlock.ForeverCombatRules || warlock.HasRune(proto.WarlockRune_RuneHelmPandemic)
}
