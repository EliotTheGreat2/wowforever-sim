package shaman

import (
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever shaman talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/shaman.json).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents. Forever's Lava Burst,
// Lightning Overload and Maelstrom Weapon reuse the SoD rune implementations
// (GrantForeverRune) with Forever's numbers.
//
// Estimates (no public data in the beta tooltips):
//   - Lava Burst: rank 1 (143-185) at 40, rank 3 (192-248) at 60, rank 2 interpolated (167-216)
//     at 50; cast time, cooldown, mana cost and coefficient as SoD's rune (2 sec, 8 sec, 10% of
//     base mana, 0.571).
//   - Stormstrike: Classic's 20 sec cooldown (the engine's 6 sec is SoD's).
//   - Maelstrom Weapon: SoD's base 10 procs per minute.
//   - Rage of the Farseer: 3 min cooldown, no mana cost, off the GCD.
//
// Not modeled (no effect on a single-target DPS sim, or not enough data):
// Elemental Warding, Eye of the Storm, Elemental Reach, Earthbound, Improved Fire Nova (Forever's
// Fire Nova spell isn't in the engine), Earth's Grasp, Guardian Totems, Improved Ghost Wolf,
// Improved Reincarnation, Healing Focus, Ancestral Healing, Healing Way (mapped onto Classic's
// stacking version), Water Shield, Mana Tide Totem, Riptide, Restorative Totems' Healing Stream
// part (Classic's 5%/rank is used).
// Forever has no Elemental Mastery, Lightning Mastery, Shamanistic Rage or Weapon Mastery.

const (
	foreverRageOfTheFarseerSpellID = 1290200 // no public spell ID yet
)

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto, and grants the SoD runes that implement Forever talents. Called from NewShaman.
func (shaman *Shaman) useForeverTalentLayout() {
	ft := shaman.ForeverTalents
	if ft == nil {
		return
	}
	shaman.Talents = &proto.ShamanTalents{
		// Elemental
		Convection:           ft.Rank("Convection"),
		ElementalWarding:     ft.Rank("Elemental Warding"),
		Reverberation:        ft.Rank("Reverberation"),
		ElementalDevastation: ft.Rank("Elemental Devastation"),
		ElementalFocus:       ft.Has("Elemental Focus"),
		EyeOfTheStorm:        ft.Rank("Eye of the Storm"),
		// Forever's single-rank Call of Thunder (+3%) = Classic rank 3.
		CallOfThunder: 3 * ft.Rank("Call of Thunder"),
		// Enhancement
		EarthsGrasp:             ft.Rank("Earth's Grasp"),
		GuardianTotems:          ft.Rank("Guardian Totems"),
		ImprovedGhostWolf:       ft.Rank("Improved Ghost Wolf"),
		ImprovedLightningShield: ft.Rank("Improved Lightning Shield"),
		ElementalWeapons:        ft.Rank("Elemental Weapons"),
		// Anticipation: 2% dodge per rank (Classic: 1%).
		Anticipation: 2 * ft.Rank("Anticipation"),
		Flurry:       ft.Rank("Flurry"), // Forever's values are set in makeFlurryAura
		Stormstrike:  ft.Has("Stormstrike"),
		Parry:        ft.Has("Spirit Weapons"),
		// Restoration
		ImprovedHealingWave:   ft.Rank("Improved Healing Wave"),
		TotemicFocus:          ft.Rank("Totemic Focus"),
		TidalFocus:            ft.Rank("Tidal Focus"),
		NaturesGuidance:       ft.Rank("Tidal Focus"), // Forever's Tidal Focus also gives 1% hit per rank
		ImprovedReincarnation: ft.Rank("Improved Reincarnation"),
		AncestralHealing:      ft.Rank("Ancestral Healing"),
		HealingFocus:          ft.Rank("Healing Focus"),
		RestorativeTotems:     ft.Rank("Restorative Totems"),
		ManaTideTotem:         ft.Has("Mana Tide Totem"),
		HealingWay:            ft.Rank("Healing Way"),
		NaturesSwiftness:      ft.Has("Nature's Swiftness"),
		Purification:          ft.Rank("Purification"),
	}

	if ft.Has("Lava Burst") {
		shaman.GrantForeverRune(int32(proto.ShamanRune_RuneHandsLavaBurst))
	}
	if ft.Has("Lightning Overload") {
		shaman.GrantForeverRune(int32(proto.ShamanRune_RuneChestOverload))
	}
	if ft.Has("Maelstrom Weapon") {
		shaman.GrantForeverRune(int32(proto.ShamanRune_RuneWaistMaelstromWeapon))
		shaman.foreverMaelstromPerStack = 0.04 * float64(ft.Rank("Maelstrom Weapon"))
	}
}

func (shaman *Shaman) applyForeverTalents() {
	ft := shaman.ForeverTalents
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

	// ---------------- Elemental Combat ----------------
	if r := rank("Convection"); r > 0 {
		// The other spells get Convection from the mapped Classic talent.
		shaman.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Pct, ClassMask: ClassSpellMask_ShamanLavaBurst, IntValue: -int64(2 * r)})
	}
	if r := rank("Concussion"); r > 0 {
		shaman.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_DamageDone_Flat,
			ClassMask: ClassSpellMask_ShamanLightningBolt | ClassSpellMask_ShamanChainLightning | ClassSpellMask_ShamanEarthShock,
			IntValue:  int64(r),
		})
	}
	if r := rank("Call of Flame"); r > 0 {
		shaman.AddStaticMod(core.SpellModConfig{
			Kind: core.SpellMod_DamageDone_Flat,
			ClassMask: ClassSpellMask_ShamanSearingTotemAttack | ClassSpellMask_ShamanMagmaTotemAttack | ClassSpellMask_ShamanFireNovaTotemAttack |
				ClassSpellMask_ShamanFlameShock | ClassSpellMask_ShamanFireNova | ClassSpellMask_ShamanLavaBurst,
			IntValue: int64(5 * r),
		})
	}
	if v := byRank("Elemental Alacrity", 170, 330, 500); v > 0 {
		shaman.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_CastTime_Flat,
			ClassMask: ClassSpellMask_ShamanLightningBolt | ClassSpellMask_ShamanChainLightning | ClassSpellMask_ShamanLavaBurst,
			TimeValue: -time.Duration(v) * time.Millisecond,
		})
	}
	if v := byRank("Lightning Overload", 0.03, 0.07, 0.10); v > 0 {
		shaman.overloadProcChance = v
	}
	if r := rank("Elemental Fury"); r > 0 {
		shaman.AddStaticMod(core.SpellModConfig{
			Kind:        core.SpellMod_CritDamageBonus_Flat,
			ClassMask:   ClassSpellMask_ShamanAll | ClassSpellMask_ShamanTotems,
			DefenseType: core.DefenseTypeMagic,
			FloatValue:  0.2 * r,
		})
	}
	if ft.Has("Lava Burst") {
		low := shaman.foreverRankValue(map[int32]float64{40: 143, 50: 167, 60: 192})
		high := shaman.foreverRankValue(map[int32]float64{40: 185, 50: 216, 60: 248})
		shaman.foreverLavaBurstDamage = [2]float64{low, high}
	}

	// ---------------- Enhancement ----------------
	if r := rank("Thundering Strikes"); r > 0 {
		shaman.AddStat(stats.MeleeCrit, r*core.CritRatingPerCritChance)
		shaman.AddStat(stats.SpellCrit, r*core.SpellCritRatingPerCritChance)
	}
	if r := rank("Ancestral Knowledge"); r > 0 {
		shaman.MultiplyStat(stats.Intellect, 1+0.02*r)
	}
	if v := byRank("Mental Dexterity", 0.33, 0.67, 1); v > 0 {
		shaman.AddStatDependency(stats.Intellect, stats.AttackPower, v)
	}
	if ft.Has("Shamanistic Focus") {
		shaman.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_PowerCost_Pct,
			ClassMask: ClassSpellMask_ShamanShocks | ClassSpellMask_ShamanLightningShield,
			IntValue:  -45,
		})
	}
	if r := rank("Toughness"); r > 0 {
		shaman.MultiplyStat(stats.Stamina, 1+0.02*r)
	}
	if ft.Has("Stormstrike") {
		shaman.applyForeverStormstrike()
	}
	if ft.Has("Spirit Weapons") {
		// Parry comes from the mapped Classic talent.
		if shaman.Consumes.MainHandImbue == proto.WeaponImbue_RockbiterWeapon {
			shaman.PseudoStats.ThreatMultiplier *= 1.3
		} else {
			shaman.PseudoStats.ThreatMultiplier *= 0.7
		}
	}
	if v := byRank("Mental Quickness", 0.15, 0.30); v > 0 {
		shaman.AddStatDependency(stats.Intellect, stats.SpellPower, v)
	}
	if r := rank("Improved Stormstrike"); r > 0 {
		shaman.applyForeverImprovedStormstrike(0.5 * r)
	}
	if ft.Has("Rage of the Farseer") {
		shaman.registerForeverRageOfTheFarseer()
	}

	// ---------------- Restoration ----------------
	if v := byRank("Mindfulness", 0.17, 0.33, 0.50); v > 0 {
		shaman.PseudoStats.SpiritRegenRateCasting += v
	}
	if r := rank("Natural Grace"); r > 0 {
		shaman.AddStaticMod(core.SpellModConfig{
			Kind:       core.SpellMod_Threat_Flat,
			ClassMask:  ClassSpellMask_ShamanAll,
			ProcMask:   core.ProcMaskSpellDamage | core.ProcMaskSpellHealing,
			FloatValue: -0.05 * r,
		})
	}
	if r := rank("Tidal Mastery"); r > 0 {
		// Healing spells only (Classic's also covered Lightning spells).
		critBonus := r * core.SpellCritRatingPerCritChance
		shaman.OnSpellRegistered(func(spell *core.Spell) {
			if spell.Matches(ClassSpellMask_ShamanHealingSpell) {
				spell.BonusCritRating += critBonus
			}
		})
	}
}

// foreverRankValue returns the value of the highest rank learned at the shaman's level,
// from {learn level: value} (the lowest rank if none is learned yet).
func (shaman *Shaman) foreverRankValue(ranks map[int32]float64) float64 {
	best, bestLevel := 0.0, int32(-1)
	lowest, lowestLevel := 0.0, int32(1000)
	for level, v := range ranks {
		if level <= shaman.Level && level > bestLevel {
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

// applyForeverStormstrike: Forever's Stormstrike strikes for normal weapon damage and makes the
// shaman's next Lightning Bolt, Chain Lightning or Earth Shock on the target deal 20% more damage
// (12 sec). Modeled as a 20% Nature damage taken debuff (like Classic's, so it doesn't stack with
// it) that the shaman's next Lightning Bolt, Chain Lightning or Earth Shock on the target consumes.
func (shaman *Shaman) applyForeverStormstrike() {
	shaman.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Cooldown_Flat, ClassMask: ClassSpellMask_ShamanStormstrike, TimeValue: time.Second * 14})

	shaman.foreverStormstrikeAuras = shaman.NewEnemyAuraArray(func(target *core.Unit, _ int32) *core.Aura {
		aura := target.GetOrRegisterAura(core.Aura{
			Label:    "Stormstrike (Forever)-" + shaman.Label,
			ActionID: core.ActionID{SpellID: 17364},
			Duration: time.Second * 12,
		})
		aura.NewExclusiveEffect("NatureDamageTaken", false, core.ExclusiveEffect{
			Priority: 20,
			OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				aura.Unit.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexNature] *= 1.2
			},
			OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				aura.Unit.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexNature] /= 1.2
			},
		})
		return aura
	})

	consumers := ClassSpellMask_ShamanLightningBolt | ClassSpellMask_ShamanChainLightning | ClassSpellMask_ShamanEarthShock
	core.MakePermanent(shaman.RegisterAura(core.Aura{
		Label: "Stormstrike Consume (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.Matches(consumers) && !spell.ProcMask.Matches(core.ProcMaskSpellProc) {
				shaman.foreverStormstrikeAuras.Get(result.Target).Deactivate(sim)
			}
		},
	}))
}

// applyForeverImprovedStormstrike: Stormstrike has a chance to allow 50% mana regeneration while
// casting for 15 sec, and its cooldown has a chance to reset when the shaman dodges or parries.
func (shaman *Shaman) applyForeverImprovedStormstrike(chance float64) {
	regen := shaman.RegisterAura(core.Aura{
		Label:    "Improved Stormstrike (Forever)",
		ActionID: core.ActionID{SpellID: 17364, Tag: 3},
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			shaman.PseudoStats.SpiritRegenRateCasting += 0.5
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			shaman.PseudoStats.SpiritRegenRateCasting -= 0.5
		},
	})

	core.MakePermanent(shaman.RegisterAura(core.Aura{
		Label: "Improved Stormstrike Trigger (Forever)",
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell.Matches(ClassSpellMask_ShamanStormstrike) && sim.Proc(chance, "Improved Stormstrike") {
				regen.Activate(sim)
			}
		},
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if shaman.Stormstrike != nil && result.Outcome.Matches(core.OutcomeDodge|core.OutcomeParry) && sim.Proc(chance, "Improved Stormstrike Reset") {
				shaman.Stormstrike.CD.Reset()
			}
		},
	}))
}

// registerForeverRageOfTheFarseer: Enhancement capstone, +30% attack speed for 25 sec.
func (shaman *Shaman) registerForeverRageOfTheFarseer() {
	actionID := core.ActionID{SpellID: foreverRageOfTheFarseerSpellID}
	aura := shaman.RegisterAura(core.Aura{
		Label:    "Rage of the Farseer",
		ActionID: actionID,
		Duration: time.Second * 25,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			shaman.MultiplyMeleeSpeed(sim, 1.3)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			shaman.MultiplyMeleeSpeed(sim, 1/1.3)
		},
	})

	spell := shaman.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    shaman.NewTimer(),
				Duration: time.Minute * 3,
			},
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			aura.Activate(sim)
		},
	})

	shaman.AddMajorCooldown(core.MajorCooldown{
		Spell: spell,
		Type:  core.CooldownTypeDPS,
	})
}
