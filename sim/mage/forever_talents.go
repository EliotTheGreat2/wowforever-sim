package mage

import (
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever mage talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/mage.json).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents. Arcane Blast, Ice Lance,
// Missile Barrage and Fingers of Frost reuse the SoD rune implementations (GrantForeverRune)
// with Forever's numbers; Frostfire Bolt is a trained Forever spell (level 40+) that reuses
// the SoD rune spell with Forever's numbers.
//
// Not modeled (no effect on a single-target DPS sim, or not enough data):
// Improved Channeling, Arcane Geometry, Arcane Shielding, Improved Counterspell's silence,
// Arcane Resilience, Magic Absorption's mana return, Wake of Fire's on-kill crit,
// Flame Throwing, Impact, Improved Fire Ward, Frost Warding, Permafrost (slow only; not mapped
// because the engine's SoD Frostfire Bolt would gain a tick from it), Improved Frost Nova,
// Arctic Reach, Ice Block, Improved Blizzard's slow, and Frostbite (bosses are immune to Freeze; it is not mapped onto the engine's
// Frostbite either, because that one carries a SoD-only "always grants Fingers of Frost").

var (
	// Spells whose damage Forever's Arcane Blast stacks increase ("all your other spells").
	// Ignite is left out because it is already a share of the buffed spell's crit.
	foreverArcaneBlastBuffedSpells = ClassSpellMask_MageAll &^ (ClassSpellMask_MageArcaneBlast | ClassSpellMask_MageIgnite)
	// Spells that consume the stacks ("until any other damage spell is cast"). Arcane Missiles
	// consumes them when its channel ends (arcane_missiles.go), so all its missiles benefit.
	foreverArcaneBlastConsumingSpells = ClassSpellMask_MageHarmfulGCDSpells &^ (ClassSpellMask_MageArcaneBlast | ClassSpellMask_MageArcaneMissiles)
)

// Frostfire Bolt: 3.0 sec cast (as the SoD rune); 3.0/3.5 coefficient is an estimate.
const foreverFrostfireBoltCoeff = 0.857

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto, and grants the SoD rune implementations Forever talents/spells reuse.
// Called from NewMage.
func (mage *Mage) useForeverTalentLayout() {
	if mage.Forever && mage.Level >= 40 {
		// Frostfire Bolt is a trained spell in Forever (ranks at 40/50/60).
		mage.GrantForeverRune(int32(proto.MageRune_RuneBeltFrostfireBolt))
	}

	ft := mage.ForeverTalents
	if ft == nil {
		return
	}
	mage.Talents = &proto.MageTalents{
		// Arcane
		WandSpecialization:   ft.Rank("Wand Specialization"),
		ArcaneConcentration:  ft.Rank("Arcane Concentration"),
		ImprovedCounterspell: ft.Rank("Improved Counterspell"),
		PresenceOfMind:       ft.Has("Presence of Mind"),
		ArcaneInstability:    ft.Rank("Arcane Instability"),
		ArcanePower:          ft.Has("Arcane Power"),
		// Fire
		ImprovedFireball:    ft.Rank("Improved Fireball"),
		Ignite:              ft.Rank("Ignite"),
		FlameThrowing:       ft.Rank("Flame Throwing"),
		Impact:              ft.Rank("Impact"),
		ImprovedFlamestrike: ft.Rank("Improved Flamestrike"),
		Pyroblast:           ft.Has("Pyroblast"),
		ImprovedScorch:      ft.Rank("Improved Scorch"),
		ImprovedFireWard:    ft.Rank("Improved Fire Ward"),
		MasterOfElements:    ft.Rank("Master of Elements"),
		CriticalMass:        ft.Rank("Critical Mass"),
		BlastWave:           ft.Has("Blast Wave"),
		FirePower:           ft.Rank("Fire Power"),
		Combustion:          ft.Has("Combustion"),
		// Frost
		FrostWarding:      ft.Rank("Frost Warding"),
		ImprovedFrostbolt: ft.Rank("Improved Frostbolt"),
		IceShards:         ft.Rank("Ice Shards"),
		ImprovedFrostNova: ft.Rank("Improved Frost Nova"),
		PiercingIce:       ft.Rank("Piercing Ice"),
		FrostChanneling:   ft.Rank("Frost Channeling"),
		ImprovedBlizzard:  ft.Rank("Improved Blizzard"),
		ArcticReach:       ft.Rank("Arctic Reach"),
		IceBlock:          ft.Has("Ice Block"),
		ColdSnap:          ft.Has("Cold Snap"),
		IceBarrier:        ft.Has("Ice Barrier"),
	}

	if ft.Has("Arcane Blast") {
		mage.GrantForeverRune(int32(proto.MageRune_RuneHandsArcaneBlast))
	}
	if ft.Has("Missile Barrage") {
		mage.GrantForeverRune(int32(proto.MageRune_RuneBeltMissileBarrage))
	}
	if ft.Has("Ice Lance") {
		mage.GrantForeverRune(int32(proto.MageRune_RuneHandsIceLance))
	}
	if r := ft.Rank("Fingers of Frost"); r > 0 {
		mage.GrantForeverRune(int32(proto.MageRune_RuneChestFingersOfFrost))
		mage.foreverFingersOfFrostStacks = r
	}
}

func (mage *Mage) applyForeverTalents() {
	ft := mage.ForeverTalents
	if ft == nil {
		return
	}
	rank := func(name string) float64 { return float64(ft.Rank(name)) }
	byRank := func(name string, values ...float64) float64 {
		r := ft.Rank(name)
		if r == 0 {
			return 0
		}
		return values[r-1]
	}

	// ---------------- Arcane ----------------
	if r := rank("Arcane Focus"); r > 0 {
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, ClassMask: ClassSpellMask_MageAll, School: core.SpellSchoolArcane, FloatValue: r * core.SpellHitRatingPerHitChance})
	}
	if v := byRank("Arcane Subtlety", 8, 15); v > 0 {
		mage.AddStat(stats.SpellPenetration, v)
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Threat_Pct, ClassMask: ClassSpellMask_MageAll, School: core.SpellSchoolArcane, FloatValue: 1 - 0.15*rank("Arcane Subtlety")})
	}
	if r := rank("Magic Absorption"); r > 0 {
		mage.AddResistances(5 * r)
	}
	if v := byRank("Arcane Meditation", 0.17, 0.33, 0.50); v > 0 {
		mage.PseudoStats.SpiritRegenRateCasting += v
	}
	if r := rank("Arcane Impact"); r > 0 {
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_MageAll, School: core.SpellSchoolArcane, FloatValue: 2 * r * core.SpellCritRatingPerCritChance})
	}
	if r := rank("Arcane Mind"); r > 0 {
		mage.MultiplyStat(stats.Intellect, 1+0.02*r)
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_CritDamageBonus_Flat, ClassMask: ClassSpellMask_MageAll, School: core.SpellSchoolArcane, FloatValue: 0.2 * r})
	}

	// ---------------- Fire ----------------
	if r := rank("Wake of Fire"); r > 0 {
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Cooldown_Flat, ClassMask: ClassSpellMask_MageFireBlast, TimeValue: -time.Second * time.Duration(r)})
	}
	if r := rank("Incineration"); r > 0 {
		mage.AddStaticMod(core.SpellModConfig{
			Kind:       core.SpellMod_BonusCrit_Flat,
			ClassMask:  ClassSpellMask_MageFireBlast | ClassSpellMask_MageIceLance | ClassSpellMask_MageArcaneBlast | ClassSpellMask_MageScorch,
			FloatValue: 2 * r * core.SpellCritRatingPerCritChance,
		})
	}
	if r := rank("Improved Fireball"); r > 0 {
		// Fireball's part is the mapped Classic talent; Forever adds Frostfire Bolt.
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_CastTime_Flat, ClassMask: ClassSpellMask_MageFrostfireBolt, TimeValue: -100 * time.Millisecond * time.Duration(r)})
	}
	if r := rank("Burning Soul"); r > 0 {
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Threat_Pct, ClassMask: ClassSpellMask_MageAll, School: core.SpellSchoolFire, FloatValue: 1 - 0.10*r})
	}
	if ft.Has("Heating Up") {
		mage.applyForeverHeatingUp()
	}

	// ---------------- Frost ----------------
	if r := rank("Elemental Precision"); r > 0 {
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, ClassMask: ClassSpellMask_MageAll, School: core.SpellSchoolFire | core.SpellSchoolFrost, FloatValue: r * core.SpellHitRatingPerHitChance})
	}
	if v := byRank("Improved Cone of Cold", 12, 23, 35); v > 0 {
		mage.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_MageConeOfCold, IntValue: int64(v)})
	}
	if v := byRank("Shatter", 17, 33, 50); v > 0 {
		mage.applyForeverCritBonus(ClassSpellMask_MageAll, func(target *core.Unit) float64 {
			return core.TernaryFloat64(mage.isTargetFrozen(target), v, 0)
		})
	}
	if r := ft.Rank("Winter's Chill"); r > 0 {
		mage.applyForeverWintersChill(r)
	}
}

// applyForeverCritBonus adds a target-dependent crit chance (in %) to the matching damage spells.
func (mage *Mage) applyForeverCritBonus(classMask uint64, critPct func(target *core.Unit) float64) {
	mage.OnSpellRegistered(func(spell *core.Spell) {
		if !spell.Matches(classMask) || !spell.ProcMask.Matches(core.ProcMaskSpellDamage) {
			return
		}
		oldApplyEffects := spell.ApplyEffects
		spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bonus := critPct(target) * core.SpellCritRatingPerCritChance
			spell.BonusCritRating += bonus
			oldApplyEffects(sim, target, spell)
			spell.BonusCritRating -= bonus
		}
	})
}

// applyForeverWintersChill: Frost damage spells have a 20% chance per rank to apply Winter's
// Chill, which raises the crit chance of the mage's own Ice Lance and Frostbolt on that target
// by 2% per stack (up to 1 stack per rank) for 15 sec. Unlike Classic it is not a raid debuff.
func (mage *Mage) applyForeverWintersChill(points int32) {
	auras := mage.NewEnemyAuraArray(func(target *core.Unit, _ int32) *core.Aura {
		return target.RegisterAura(core.Aura{
			Label:     "Winter's Chill (Forever)-" + mage.Label,
			ActionID:  core.ActionID{SpellID: 12579},
			Duration:  time.Second * 15,
			MaxStacks: points,
		})
	})

	core.MakeProcTriggerAura(&mage.Unit, core.ProcTrigger{
		Name:             "Winter's Chill Trigger (Forever)",
		Callback:         core.CallbackOnSpellHitDealt,
		Outcome:          core.OutcomeLanded,
		ClassSpellMask:   ClassSpellMask_MageAll,
		SpellSchool:      core.SpellSchoolFrost,
		ProcChance:       0.2 * float64(points),
		Harmful:          true,
		CanProcFromProcs: true,
		Handler: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			aura := auras.Get(result.Target)
			aura.Activate(sim)
			aura.AddStack(sim)
		},
	})

	mage.applyForeverCritBonus(ClassSpellMask_MageFrostbolt|ClassSpellMask_MageIceLance, func(target *core.Unit) float64 {
		if aura := auras.Get(target); aura.IsActive() {
			return 2 * float64(aura.GetStacks())
		}
		return 0
	})
}

// applyForeverHeatingUp: non-periodic crits with Fireball, Frostfire Bolt, Fire Blast and
// Scorch reduce the cast time of the next Pyroblast within 20 sec by 25%, stacking 3 times.
func (mage *Mage) applyForeverHeatingUp() {
	triggers := ClassSpellMask_MageFireball | ClassSpellMask_MageFrostfireBolt | ClassSpellMask_MageFireBlast | ClassSpellMask_MageScorch

	castTimeMod := mage.AddDynamicMod(core.SpellModConfig{
		Kind:      core.SpellMod_CastTime_Pct,
		ClassMask: ClassSpellMask_MagePyroblast,
	})

	aura := mage.RegisterAura(core.Aura{
		Label:     "Heating Up (Forever)",
		ActionID:  core.ActionID{SpellID: 48107},
		Duration:  time.Second * 20,
		MaxStacks: 3,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			castTimeMod.Activate()
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			castTimeMod.Deactivate()
		},
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			castTimeMod.UpdateFloatValue(-0.25 * float64(newStacks))
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell.Matches(ClassSpellMask_MagePyroblast) {
				aura.Deactivate(sim)
			}
		},
	})

	core.MakePermanent(mage.RegisterAura(core.Aura{
		Label: "Heating Up Trigger (Forever)",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.Matches(triggers) && result.DidCrit() {
				aura.Activate(sim)
				aura.AddStack(sim)
			}
		},
	}))
}

// foreverRankValue returns the value of the highest rank learned at the mage's level, from
// {learn level: value} (the lowest rank if none is learned yet).
func (mage *Mage) foreverRankValue(ranks map[int32]float64) float64 {
	best, bestLevel := 0.0, int32(-1)
	lowest, lowestLevel := 0.0, int32(1000)
	for level, v := range ranks {
		if level <= mage.Level && level > bestLevel {
			best, bestLevel = v, level
		}
		if level < lowestLevel {
			lowest, lowestLevel = v, level
		}
	}
	if bestLevel < 0 {
		return lowest
	}
	return best
}

// foreverArcaneBlastDamage: Forever's Arcane Blast has 5 ranks: rank 1 deals 50-58 (talent
// tooltip) and rank 5 (level 60) 364-424 (spellbook). The learn levels of ranks 2-4 and
// their damage aren't published; ranks every 10 levels from 20 with linear damage are an
// estimate. Cast time, coefficient (0.714) and mana cost are the SoD rune's.
func (mage *Mage) foreverArcaneBlastDamage() (float64, float64) {
	low := mage.foreverRankValue(map[int32]float64{20: 50, 30: 129, 40: 207, 50: 286, 60: 364})
	high := mage.foreverRankValue(map[int32]float64{20: 58, 30: 150, 40: 241, 50: 333, 60: 424})
	return low, high
}

// foreverIceLanceDamage: Forever's Ice Lance has 6 ranks: rank 1 deals 26-30 (talent tooltip)
// and rank 6 (level 56) 137-161 (spellbook). Ranks 2-5 are estimated (levels 28/36/44/50,
// linear damage). 300% more damage to Frozen targets, coefficient and mana as the SoD rune.
func (mage *Mage) foreverIceLanceDamage() (float64, float64) {
	low := mage.foreverRankValue(map[int32]float64{20: 26, 28: 48, 36: 70, 44: 93, 50: 115, 56: 137})
	high := mage.foreverRankValue(map[int32]float64{20: 30, 28: 56, 36: 82, 44: 108, 50: 135, 56: 161})
	return low, high
}

// foreverFrostfireBoltDamage: Forever's Frostfire Bolt has 3 ranks (levels 40/50/60); rank 3
// deals 270-314 and 57 over 9 sec (spellbook). Ranks 1-2 are estimated from Frostbolt's
// rank scaling at the same levels. Returns the low/high hit and the per-tick damage.
func (mage *Mage) foreverFrostfireBoltDamage() (float64, float64, float64) {
	low := mage.foreverRankValue(map[int32]float64{40: 162, 50: 224, 60: 270})
	high := mage.foreverRankValue(map[int32]float64{40: 188, 50: 261, 60: 314})
	dot := mage.foreverRankValue(map[int32]float64{40: 34, 50: 47, 60: 57})
	return low, high, dot / 3
}
