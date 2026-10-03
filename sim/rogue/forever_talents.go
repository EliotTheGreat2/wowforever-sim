package rogue

import (
	"slices"
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever rogue talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/rogue.json; full tooltips from wow-forever.gg).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents.
//
// Not modeled (no effect on a single-target boss DPS sim, or not enough data):
// Improved Gouge, Remorseless Attacks (needs kills), Improved Expose Armor (Forever's version
// refunds combo points; Expose Armor isn't in the simmed rotations), Improved Kidney Shot
// (bosses are stun immune), Improved Sprint, Improved Kick, Camouflage, Master of Deception,
// Setup, Dirty Tricks, Improved Distract, Heightened Senses, Cutthroat (Ambush outside Stealth;
// the SoD Cutthroat rune also changes Backstab/Garrote facing, so it isn't reused),
// Improved Poisons' "no charge consumed" clause (poison charges aren't modeled).
//
// Estimates (no Forever data published yet):
//   - Mutilate keeps the SoD rune's 40 Energy cost and dagger requirement; its flat damage per
//     weapon is 17.2 (talent tooltip, rank 1) to 50 (spellbook, rank 4). The rank learn levels
//     aren't published; ranks 2-4 are assumed at 40/50/60 like other talent abilities.
//   - Venom costs 25 Energy (like Slice and Dice) and lasts 6 sec + 3 sec per combo point
//     (9-21 sec from the tooltip).
//   - Serrated Blades and Hack and Slash (Mace) ignore a percentage of the primary target's
//     armor before debuffs; Hack and Slash's Mace part applies if either hand has a Mace.
//   - Murder: Humanoid and Giant targets only (Forever tooltip), 2% per rank.

const (
	foreverVenomSpellID        = 1296001 // Forever's Venom has no public spell ID yet
	foreverThousandCutsSpellID = 1296002
	foreverHemorrhageDebuffTag = 99
)

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from NewRogue.
func (rogue *Rogue) useForeverTalentLayout() {
	ft := rogue.ForeverTalents
	if ft == nil {
		return
	}
	rogue.Talents = &proto.RogueTalents{
		// Assassination
		Malice:               ft.Rank("Malice"),
		Ruthlessness:         ft.Rank("Ruthlessness"),
		ImprovedSliceAndDice: ft.Rank("Improved Slice and Dice"),
		RelentlessStrikes:    ft.Has("Relentless Strikes"),
		VilePoisons:          ft.Rank("Vile Poisons"),
		ColdBlood:            ft.Has("Cold Blood"),
		ImprovedPoisons:      ft.Rank("Improved Poisons"),
		SealFate:             ft.Rank("Seal Fate"),
		// Combat
		ImprovedSinisterStrike: ft.Rank("Improved Sinister Strike"),
		LightningReflexes:      ft.Rank("Lightning Reflexes"),
		Deflection:             ft.Rank("Deflection"),
		Precision:              ft.Rank("Precision"),
		Endurance:              ft.Rank("Endurance"),
		Riposte:                ft.Has("Riposte"),
		BladeFlurry:            ft.Has("Blade Flurry"),
		AdrenalineRush:         ft.Has("Adrenaline Rush"),
		// Hack and Slash with Daggers/Fist Weapons: +1% crit per rank, like Classic's Dagger
		// and Fist Weapon Specialization. Swords/Axes and Maces are applied in applyForeverTalents.
		DaggerSpecialization:     ft.Rank("Hack and Slash"),
		FistWeaponSpecialization: ft.Rank("Hack and Slash"),
		// Subtlety
		Elusiveness:    ft.Rank("Elusiveness"),
		ImprovedAmbush: ft.Rank("Improved Ambush"),
		GhostlyStrike:  ft.Has("Ghostly Strike"),
		Premeditation:  ft.Has("Premeditation"),
		DirtyDeeds:     ft.Rank("Dirty Deeds"),
		Preparation:    ft.Has("Preparation"),
		Hemorrhage:     ft.Has("Hemorrhage"),
	}

	// Forever's Mutilate talent is the SoD rune with Forever's numbers (mutilate.go).
	if ft.Has("Mutilate") {
		rogue.GrantForeverRune(int32(proto.RogueRune_RuneMutilate))
	}
}

// foreverVigorEnergy: Forever's Vigor has 2 ranks of +5 maximum Energy.
func (rogue *Rogue) foreverVigorEnergy() float64 {
	if rogue.ForeverTalents == nil {
		return 0
	}
	return 5 * float64(rogue.ForeverTalents.Rank("Vigor"))
}

func (rogue *Rogue) applyForeverTalents() {
	ft := rogue.ForeverTalents
	if ft == nil {
		return
	}
	rank := func(name string) float64 { return float64(ft.Rank(name)) }

	// ---------------- Assassination ----------------
	if r := rank("Malice"); r > 0 {
		// Melee crit is the mapped Classic talent; Forever's Malice also covers Poisons.
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_RogueInstantPoison | ClassSpellMask_RogueDeadlyPoisonTick, FloatValue: r * core.SpellCritRatingPerCritChance})
	}
	if r := rank("Murder"); r > 0 {
		rogue.applyForeverMurder(1 + 0.02*r)
	}
	if ft.Has("Venom") {
		rogue.registerForeverVenom()
	}

	// ---------------- Combat ----------------
	if v := []int64{0, 7, 13, 20}[ft.Rank("Improved Eviscerate")]; v > 0 {
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_RogueEviscerate, IntValue: v})
	}
	if r := rank("Puncturing Wounds"); r > 0 {
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_RogueBackstab, FloatValue: 10 * r * core.CritRatingPerCritChance})
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_RogueMutilateHit, FloatValue: 5 * r * core.CritRatingPerCritChance})
		rogue.applyForeverPuncturingWounds(0.15 * r)
	}
	if ft.Has("Flawless Execution") {
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Flat, ClassMask: ClassSpellMask_RogueEviscerate, IntValue: -10})
	}
	if r := rank("Hack and Slash"); r > 0 {
		if mask := rogue.GetProcMaskForTypes(proto.WeaponType_WeaponTypeSword, proto.WeaponType_WeaponTypeAxe); mask != core.ProcMaskUnknown {
			rogue.registerExtraAttackProc("Hack and Slash", mask, 0.01*r)
		}
	}
	if r := rank("Weapon Expertise"); r > 0 {
		// Expertise reduces the target's dodge and parry chance by 1% per point.
		rogue.AddStat(stats.Expertise, r)
	}
	if r := rank("Aggression"); r > 0 {
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_RogueSinisterStrike | ClassSpellMask_RogueBackstab | ClassSpellMask_RogueEviscerate, IntValue: int64(2 * r)})
	}

	// ---------------- Subtlety ----------------
	if r := rank("Opportunity"); r > 0 {
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_RogueBackstab | ClassSpellMask_RogueGarrote | ClassSpellMask_RogueAmbush | ClassSpellMask_RogueMutilateHit, IntValue: int64(5 * r)})
	}
	if v := []float64{0, 0.33, 0.67, 1}[ft.Rank("Initiative")]; v > 0 {
		rogue.applyForeverInitiative(v)
	}
	if r := rank("Serrated Blades"); r > 0 {
		rogue.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PeriodicDamageDone_Flat, ClassMask: ClassSpellMask_RogueRupture, IntValue: int64(10 * r)})
	}
	armorIgnore := 0.03 * rank("Serrated Blades")
	if rogue.GetProcMaskForTypes(proto.WeaponType_WeaponTypeMace) != core.ProcMaskUnknown {
		armorIgnore += 0.03 * rank("Hack and Slash")
	}
	if armorIgnore > 0 && len(rogue.Env.Encounter.TargetUnits) > 0 {
		rogue.AddStat(stats.ArmorPenetration, armorIgnore*rogue.Env.Encounter.TargetUnits[0].GetStat(stats.Armor))
	}
	if ft.Has("Hemorrhage") {
		rogue.foreverHemorrhageDebuffs = rogue.NewEnemyAuraArray(func(target *core.Unit, _ int32) *core.Aura {
			return target.RegisterAura(core.Aura{
				Label:    "Hemorrhage (Forever)-" + rogue.Label,
				ActionID: core.ActionID{SpellID: 16511, Tag: foreverHemorrhageDebuffTag},
				Duration: time.Second * 15,
			})
		})
	}
	if r := rank("Quietus"); r > 0 {
		rogue.applyForeverQuietus(2 * r)
	}
	if ft.Has("Thousand Cuts") {
		rogue.applyForeverThousandCuts()
	}
}

// Murder: +2/4% damage against Humanoid and Giant targets.
func (rogue *Rogue) applyForeverMurder(multiplier float64) {
	mobTypes := []proto.MobType{proto.MobType_MobTypeHumanoid, proto.MobType_MobTypeGiant}
	targets := core.FilterSlice(rogue.Env.Encounter.Targets, func(t *core.Target) bool { return slices.Contains(mobTypes, t.MobType) })
	if len(targets) == 0 {
		return
	}
	rogue.Env.RegisterPostFinalizeEffect(func() {
		for _, t := range targets {
			for _, at := range rogue.AttackTables[t.UnitIndex] {
				at.DamageDealtMultiplier *= multiplier
				at.CritMultiplier *= multiplier
			}
		}
	})
}

// Puncturing Wounds: Backstab has a 15/30/45% chance to add an additional Combo Point.
func (rogue *Rogue) applyForeverPuncturingWounds(procChance float64) {
	cpMetrics := rogue.NewComboPointMetrics(core.ActionID{SpellID: 13733})
	core.MakePermanent(rogue.RegisterAura(core.Aura{
		Label: "Puncturing Wounds (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.Matches(ClassSpellMask_RogueBackstab) && sim.Proc(procChance, "Puncturing Wounds") {
				rogue.AddComboPoints(sim, 1, result.Target, cpMetrics)
			}
		},
	}))
}

// Initiative: 33/67/100% chance for an extra Combo Point from Ambush and Garrote.
func (rogue *Rogue) applyForeverInitiative(procChance float64) {
	cpMetrics := rogue.NewComboPointMetrics(core.ActionID{SpellID: 13980})
	core.MakePermanent(rogue.RegisterAura(core.Aura{
		Label: "Initiative (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.Matches(ClassSpellMask_RogueAmbush|ClassSpellMask_RogueGarrote) && sim.Proc(procChance, "Initiative") {
				rogue.AddComboPoints(sim, 1, result.Target, cpMetrics)
			}
		},
	}))
}

// Quietus: Sinister Strike, Ghostly Strike and Hemorrhage deal 2-10% more damage to targets
// below 35% health.
func (rogue *Rogue) applyForeverQuietus(percent float64) {
	mask := ClassSpellMask_RogueSinisterStrike | ClassSpellMask_RogueGhostlyStrike | ClassSpellMask_RogueHemorrhage
	mod := rogue.AddDynamicMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: mask, IntValue: int64(percent)})
	core.MakePermanent(rogue.RegisterAura(core.Aura{
		Label: "Quietus (Forever)",
		OnApplyEffects: func(aura *core.Aura, sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if !spell.Matches(mask) {
				return
			}
			if sim.IsExecutePhase35() {
				mod.Activate()
			} else {
				mod.Deactivate()
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			mod.Deactivate()
		},
	}))
}

// Thousand Cuts: each Rupture tick reduces the Energy cost of the next Hemorrhage or Backstab
// within 10 sec by 3, stacking up to 5 times.
func (rogue *Rogue) applyForeverThousandCuts() {
	mask := ClassSpellMask_RogueBackstab | ClassSpellMask_RogueHemorrhage
	mod := rogue.AddDynamicMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Flat, ClassMask: mask, IntValue: -3})
	aura := rogue.RegisterAura(core.Aura{
		Label:     "Thousand Cuts",
		ActionID:  core.ActionID{SpellID: foreverThousandCutsSpellID},
		Duration:  time.Second * 10,
		MaxStacks: 5,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks, newStacks int32) {
			mod.UpdateIntValue(-3 * int64(newStacks))
			if newStacks > 0 {
				mod.Activate()
			} else {
				mod.Deactivate()
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			mod.Deactivate()
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell.Matches(mask) {
				aura.Deactivate(sim)
			}
		},
	})
	core.MakePermanent(rogue.RegisterAura(core.Aura{
		Label: "Thousand Cuts Trigger",
		OnPeriodicDamageDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.Matches(ClassSpellMask_RogueRupture) {
				aura.Activate(sim)
				aura.AddStack(sim)
			}
		},
	}))
}

// foreverGhostlyStrikeMultiplier: Forever's Ghostly Strike deals 180% weapon damage with a
// main-hand Dagger (125% otherwise, as in Classic).
func (rogue *Rogue) foreverGhostlyStrikeMultiplier() float64 {
	if rogue.ForeverTalents != nil && rogue.HasDagger(core.MainHand) {
		return 1.8
	}
	return 1.25
}

// foreverHemorrhageMultiplier: Forever's Hemorrhage deals 100% weapon damage, 145% with a Dagger.
func (rogue *Rogue) foreverHemorrhageMultiplier() float64 {
	if rogue.ForeverTalents != nil && rogue.HasDagger(core.MainHand) {
		return 1.45
	}
	return 1
}

// Forever's Hemorrhage: the target takes 15% more Rupture damage from the rogue for 15 sec.
func (rogue *Rogue) applyForeverHemorrhageDebuff(sim *core.Simulation, target *core.Unit) {
	if rogue.foreverHemorrhageDebuffs != nil {
		rogue.foreverHemorrhageDebuffs.Get(target).Activate(sim)
	}
}

func (rogue *Rogue) foreverRuptureTakenMultiplier(target *core.Unit) float64 {
	if rogue.foreverHemorrhageDebuffs != nil && rogue.foreverHemorrhageDebuffs.Get(target).IsActive() {
		return 1.15
	}
	return 1
}

// foreverMutilateFlatDamage: flat damage per weapon of the highest Mutilate rank known.
func (rogue *Rogue) foreverMutilateFlatDamage() float64 {
	switch {
	case rogue.Level >= 60:
		return 50
	case rogue.Level >= 50:
		return 39
	case rogue.Level >= 40:
		return 28
	}
	return 17.2
}

// registerForeverVenom: Assassination capstone finisher. Poisons deal 30% more damage and are
// 10% more likely to apply while it lasts (9/12/15/18/21 sec by combo points).
func (rogue *Rogue) registerForeverVenom() {
	actionID := core.ActionID{SpellID: foreverVenomSpellID}
	poisonMod := rogue.AddDynamicMod(core.SpellModConfig{
		Kind:       core.SpellMod_DamageDone_Pct,
		ClassMask:  ClassSpellMask_RogueInstantPoison | ClassSpellMask_RogueDeadlyPoisonTick | ClassSpellMask_RogueOccultPoisonTick,
		FloatValue: 1.3,
	})

	rogue.foreverVenomAura = rogue.RegisterAura(core.Aura{
		Label:    "Venom",
		ActionID: actionID,
		Duration: time.Second * 21,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			poisonMod.Activate()
			rogue.additivePoisonBonusChance += 0.10
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			poisonMod.Deactivate()
			rogue.additivePoisonBonusChance -= 0.10
		},
	})

	rogue.foreverVenom = rogue.RegisterSpell(core.SpellConfig{
		ActionID:     actionID,
		Flags:        core.SpellFlagAPL,
		MetricSplits: 6,

		EnergyCost: core.EnergyCostOptions{
			Cost: 25,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				spell.SetMetricsSplit(spell.Unit.ComboPoints())
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return rogue.ComboPoints() > 0
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			rogue.foreverVenomAura.Duration = time.Second * time.Duration(6+3*rogue.ComboPoints())
			rogue.foreverVenomAura.Activate(sim)
			rogue.SpendComboPoints(sim, spell)
		},
	})
	rogue.Finishers = append(rogue.Finishers, rogue.foreverVenom)
}
