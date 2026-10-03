package hunter

import (
	"slices"
	"strconv"
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever hunter talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/hunter.json) and the Forever spellbook changes that
// matter to a DPS sim (https://foreverchanges.pro/spellbook/hunter). Everything here is
// gated on ForeverTalents != nil, so Classic/SoD sims are unchanged.
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents.
//
// Spellbook changes modeled (foreverSpellbook):
//   - Aimed Shot is trainer-taught, +166 ranged damage at rank 6 (Classic 600; lower ranks
//     scaled by the same ratio), 2 sec cast (wow.gg Forever MM guide: "a two-second cast and a
//     six-second cooldown shared with Multi-Shot"), 6 sec cooldown shared with Multi-Shot
//     instead of Arcane Shot. Multi-Shot keeps its own 10 sec cooldown on that shared timer.
//   - Aspect of the Hawk: 55 RAP at max rank (Classic 110); all ranks halved.
//   - Raptor Strike: +70 at max rank (Classic 140); all ranks halved.
//   - Rapid Fire also increases melee attack speed.
//   - No Aspect of the Lion (a SoD ability the engine gives every hunter).
//   - Arcane Shot 217 and Volley 112/s come from spell_tuning.csv.
//   - Mongoose Bite: melee weapon damage plus 57 at rank 4 (Classic 115 flat; lower ranks scaled).
//   - Hunter's Mark: 71 ranged attack power at rank 4 (Classic 110; lower ranks scaled).
//   - Aspect of the Beast (level 30): +50 melee attack power; Deadly Aspects' melee clause.
//   - Lacerate (new): 406 bleed damage over 21 sec, rank 4 at level 60 only (lower ranks,
//     mana cost and cooldown unpublished; estimates in registerForeverLacerate).
//   - Strider Kick (Survival talent): 100% melee weapon damage, 8 sec cooldown (estimated cost).
//
// Not modeled (no effect on a single-target DPS sim, or not enough data); utility talents
// are still mapped to their Classic fields where one exists:
// Endurance Training's armor clause, Improved Aspect of the Monkey, Pathfinding,
// Improved Revive Pet, Bestial Swiftness, Improved Mend Pet, Spirit Bond, Intimidation's stun
// (the talent still unlocks Bestial Wrath), Summon Hawk (no hawk damage/AP data published, so
// Unleashed Fury/Ferocity apply to the pet only), Hawk Eye, Improved Concussive Shot,
// Improved Stings' Viper/Scorpid clauses, Rapid Killing's on-kill buff and Rapid
// Recuperation's Rapid Killing clause (no kills in a single-target sim; the Rapid Fire
// cooldown reduction and Rapid Recuperation's Serpent Sting clause are modeled), Scatter Shot,
// Sniper Shot's range clause, Deflection, Entrapment, Improved Wing Clip, Deterrence,
// Survival Tactics' Feign Death clause, Counterattack (needs parries taken),
// Survivalist's Discipline, Strider Kick's movement speed clause.

const (
	foreverSniperShotSpellID         = 1290201 // no public spell ID yet
	foreverLaceratingStrikesSpellID  = 1290202
	foreverLoneWolfSpellID           = 1290203
	foreverFocusedFireSpellID        = 1290204
	foreverRapidRecuperationSpellID  = 1290205
	foreverStriderKickSpellID        = 1290206
	foreverLacerateSpellID           = 1290207
	foreverAspectOfTheBeastSpellID   = 13161 // Classic Aspect of the Beast
	foreverDeadlyAspectsBeastSpellID = 1290208
	foreverAimedShotScale            = 166.0 / 600.0
	foreverAspectOfTheHawkScale      = 55.0 / 110.0
	foreverRaptorStrikeScale         = 70.0 / 140.0
	foreverMongooseBiteScale         = 57.0 / 115.0
	foreverHuntersMarkScale          = 71.0 / 110.0
	foreverAspectOfTheBeastAP        = 50.0
	foreverStriderKickCooldown       = time.Second * 8
	foreverStriderKickBaseManaPct    = 0.05 // not published; estimate near Raptor Strike's cost
	foreverLacerateTotalDamage       = 406.0
	foreverLacerateLevel             = 60   // rank 4 is the only published rank
	foreverLacerateBaseManaPct       = 0.05 // not published; estimate
	foreverAimedShotCastTime         = time.Millisecond * 2000
	foreverAimedShotCooldown         = time.Second * 6
	foreverSniperShotBonusDamage     = 160.0
	foreverSniperShotCastTime        = time.Millisecond * 4000
	foreverSniperShotCooldown        = time.Second * 15
	foreverSniperShotBaseManaPercent = 0.06 // not published; estimate between Arcane Shot and Aimed Shot
)

// foreverSpellbook reports whether the Forever spellbook changes apply.
func (hunter *Hunter) foreverSpellbook() bool {
	return hunter.ForeverTalents != nil
}

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from NewHunter.
func (hunter *Hunter) useForeverTalentLayout() {
	ft := hunter.ForeverTalents
	if ft == nil {
		return
	}
	hunter.Talents = &proto.HunterTalents{
		// Beast Mastery
		// Deadly Aspects: 2% per rank chance on Auto Shot for +30% ranged speed for 12 sec
		// (Classic Improved Aspect of the Hawk: 1% per rank, same proc).
		ImprovedAspectOfTheHawk:    2 * ft.Rank("Deadly Aspects"),
		EnduranceTraining:          ft.Rank("Endurance Training"),
		ImprovedAspectOfTheMonkey:  ft.Rank("Improved Aspect of the Monkey"),
		ImprovedRevivePet:          ft.Rank("Improved Revive Pet"),
		Pathfinding:                ft.Rank("Pathfinding"),
		BestialSwiftness:           ft.Has("Bestial Swiftness"),
		ImprovedMendPet:            ft.Rank("Improved Mend Pet"),
		SpiritBond:                 ft.Rank("Spirit Bond"),
		Intimidation:               ft.Has("Intimidation"),
		BestialDiscipline:          ft.Rank("Bestial Discipline"),
		Frenzy:                     ft.Rank("Frenzy"),
		BestialWrath:               ft.Has("Bestial Wrath"),
		ImprovedConcussiveShot:     ft.Rank("Improved Concussive Shot"),
		LethalShots:                ft.Rank("Lethal Attacks"),
		AimedShot:                  true, // trainer-taught in Forever
		HawkEye:                    ft.Rank("Hawk Eye"),
		ImprovedSerpentSting:       ft.Rank("Improved Serpent Sting"),
		MortalShots:                ft.Rank("Mortal Shots"),
		ScatterShot:                ft.Has("Scatter Shot"),
		RangedWeaponSpecialization: ft.Rank("Ranged Weapon Specialization"),
		// Survival
		Deflection:       ft.Rank("Deflection"),
		Entrapment:       ft.Rank("Entrapment"),
		ImprovedWingClip: ft.Rank("Improved Wing Clip"),
		CleverTraps:      ft.Rank("Clever Traps"),
		Survivalist:      ft.Rank("Survivalist"),
		Deterrence:       ft.Has("Deterrence"),
		TrapMastery:      ft.Rank("Survival Tactics"),
		Surefooted:       ft.Rank("Surefooted"),
		Counterattack:    ft.Has("Counterattack"),
	}
}

func (hunter *Hunter) applyForeverTalents() {
	ft := hunter.ForeverTalents
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
	pet := hunter.pet

	// ---------------- Beast Mastery ----------------
	if r := rank("Focused Fire"); r > 0 && pet != nil {
		hunter.applyForeverFocusedFire(0.01 * r)
	}
	if r := rank("Unleashed Fury"); r > 0 && pet != nil {
		pet.PseudoStats.DamageDealtMultiplierAdditive += 0.03 * r
	}
	if r := rank("Ferocity"); r > 0 && pet != nil {
		pet.AddStat(stats.MeleeCrit, 2*r*core.CritRatingPerCritChance)
		pet.AddStat(stats.SpellCrit, 2*r*core.SpellCritRatingPerCritChance)
	}
	if r := rank("Bestial Discipline"); r > 0 {
		// Focus regen part is the Classic mapping; this is the mana-regen-while-casting part.
		hunter.PseudoStats.SpiritRegenRateCasting += 0.25 * r
	}

	// ---------------- Marksmanship ----------------
	if r := rank("Efficiency"); r > 0 {
		hunter.AddStaticMod(core.SpellModConfig{
			Kind: core.SpellMod_PowerCost_Pct,
			ClassMask: ClassSpellMask_HunterShots | ClassSpellMask_HunterStrikes | ClassSpellMask_HunterStings | ClassSpellMask_HunterVolley |
				ClassSpellMask_HunterMongooseBite | ClassSpellMask_HunterWingClip,
			IntValue: -3 * int64(r),
		})
	}
	if r := rank("Careful Aim"); r > 0 {
		hunter.AddStatDependency(stats.Intellect, stats.AttackPower, 0.2*r)
		hunter.AddStatDependency(stats.Intellect, stats.RangedAttackPower, 0.2*r)
	}
	if v := byRank("Improved Stings", 6, 13, 20); v > 0 {
		hunter.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_DamageDone_Flat,
			ClassMask: ClassSpellMask_HunterSerpentSting | ClassSpellMask_HunterSoFSerpentSting | ClassSpellMask_HunterChimeraSerpent,
			IntValue:  int64(v),
		})
	}
	if r := rank("Rapid Killing"); r > 0 {
		hunter.foreverRapidFireCDReduction = time.Minute * time.Duration(r)
	}
	if r := rank("Improved Arcane Shot"); r > 0 {
		hunter.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_Cooldown_Flat,
			ClassMask: ClassSpellMask_HunterArcaneShot,
			TimeValue: -time.Millisecond * time.Duration(300*r),
		})
	}
	if ft.Has("Lone Wolf") {
		hunter.applyForeverLoneWolf()
	}
	if ft.Has("Trueshot Aura") {
		// Party aura, +30 RAP (flat; Classic: 100/150/200).
		hunter.AddStat(stats.RangedAttackPower, 30)
	}
	if r := rank("Rapid Recuperation"); r > 0 {
		hunter.applyForeverRapidRecuperation(0.25 * r)
	}
	if v := byRank("Barrage", 3, 7, 10); v > 0 {
		hunter.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_DamageDone_Flat,
			ClassMask: ClassSpellMask_HunterMultiShot | ClassSpellMask_HunterAimedShot | ClassSpellMask_HunterVolley,
			IntValue:  int64(v),
		})
	}

	// ---------------- Survival ----------------
	if r := rank("Improved Tracking"); r > 0 {
		hunter.applyForeverImprovedTracking(1 + 0.01*r)
	}
	if r := rank("Savage Strikes"); r > 0 {
		hunter.AddStaticMod(core.SpellModConfig{
			Kind:       core.SpellMod_BonusCrit_Flat,
			ClassMask:  ClassSpellMask_HunterStrikes | ClassSpellMask_HunterMongooseBite | ClassSpellMask_HunterWingClip,
			FloatValue: 2 * r * core.CritRatingPerCritChance,
		})
	}
	if r := rank("Predator's Edge"); r > 0 {
		hunter.applyForeverPredatorsEdge(0.06*r, 1+0.1*r)
	}
	if r := rank("Resourcefulness"); r > 0 {
		hunter.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_PowerCost_Pct,
			ClassMask: ClassSpellMask_HunterStrikes | ClassSpellMask_HunterMongooseBite | ClassSpellMask_HunterWingClip | ClassSpellMask_HunterTraps,
			IntValue:  -30 * int64(r),
		})
	}
	if r := rank("Expose Prey"); r > 0 {
		hunter.applyForeverExposePrey(0.05 * r)
	}
	if ft.Has("Lacerating Strikes") {
		hunter.applyForeverLaceratingStrikes()
	}
	if r := rank("Lightning Reflexes"); r > 0 {
		hunter.MultiplyStat(stats.Agility, 1+0.02*r)
	}
}

// applyForeverFocusedFire: +1/2% damage for the hunter and the pet while the pet is active.
func (hunter *Hunter) applyForeverFocusedFire(bonus float64) {
	pet := hunter.pet
	pet.PseudoStats.DamageDealtMultiplier *= 1 + bonus // only matters while the pet is out

	aura := hunter.RegisterAura(core.Aura{
		Label:    "Focused Fire (Forever)",
		ActionID: core.ActionID{SpellID: foreverFocusedFireSpellID},
		Duration: core.NeverExpires,
	}).AttachMultiplicativePseudoStatBuff(&hunter.PseudoStats.DamageDealtMultiplier, 1+bonus)

	pet.ApplyOnPetEnable(func(sim *core.Simulation) { aura.Activate(sim) })
	pet.ApplyOnPetDisable(func(sim *core.Simulation) { aura.Deactivate(sim) })
}

// applyForeverLoneWolf: +20% damage with all attacks while no pet is active.
func (hunter *Hunter) applyForeverLoneWolf() {
	aura := hunter.RegisterAura(core.Aura{
		Label:    "Lone Wolf (Forever)",
		ActionID: core.ActionID{SpellID: foreverLoneWolfSpellID},
		Duration: core.NeverExpires,
	}).AttachMultiplicativePseudoStatBuff(&hunter.PseudoStats.DamageDealtMultiplier, 1.2)

	pet := hunter.pet
	if pet == nil {
		core.MakePermanent(aura)
		return
	}
	pet.ApplyOnPetEnable(func(sim *core.Simulation) { aura.Deactivate(sim) })
	pet.ApplyOnPetDisable(func(sim *core.Simulation) { aura.Activate(sim) })
}

// applyForeverRapidRecuperation: Serpent Sting hits allow 25/50% of mana regen while casting
// for 15 sec.
func (hunter *Hunter) applyForeverRapidRecuperation(regen float64) {
	buff := hunter.RegisterAura(core.Aura{
		Label:    "Rapid Recuperation (Forever)",
		ActionID: core.ActionID{SpellID: foreverRapidRecuperationSpellID},
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			hunter.PseudoStats.SpiritRegenRateCasting += regen
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			hunter.PseudoStats.SpiritRegenRateCasting -= regen
		},
	})
	core.MakePermanent(hunter.RegisterAura(core.Aura{
		Label: "Rapid Recuperation Trigger (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.Matches(ClassSpellMask_HunterSerpentSting) {
				buff.Activate(sim)
			}
		},
	}))
}

// applyForeverImprovedTracking: the hunter is assumed to track the target's creature type
// (Beasts, Demons, Dragonkin, Elementals, Giants, Humanoids, Undead).
func (hunter *Hunter) applyForeverImprovedTracking(multiplier float64) {
	trackable := []proto.MobType{proto.MobType_MobTypeBeast, proto.MobType_MobTypeDemon, proto.MobType_MobTypeDragonkin,
		proto.MobType_MobTypeElemental, proto.MobType_MobTypeGiant, proto.MobType_MobTypeHumanoid, proto.MobType_MobTypeUndead}
	targets := core.FilterSlice(hunter.Env.Encounter.Targets, func(t *core.Target) bool { return slices.Contains(trackable, t.MobType) })
	if len(targets) == 0 {
		return
	}
	hunter.Env.RegisterPostFinalizeEffect(func() {
		for _, t := range targets {
			for _, at := range hunter.AttackTables[t.UnitIndex] {
				at.DamageDealtMultiplier *= multiplier
				at.CritMultiplier *= multiplier
			}
		}
	})
}

// applyForeverPredatorsEdge: melee crit damage bonus and Off Hand weapon damage.
func (hunter *Hunter) applyForeverPredatorsEdge(critBonus float64, ohMultiplier float64) {
	hunter.AutoAttacks.MHConfig().CritDamageBonus += critBonus
	hunter.AutoAttacks.OHConfig().CritDamageBonus += critBonus
	hunter.AutoAttacks.OHConfig().DamageMultiplier *= ohMultiplier
	hunter.OnSpellRegistered(func(spell *core.Spell) {
		// Off-hand Raptor Strike reads OHConfig().DamageMultiplier at registration.
		if spell.ProcMask.Matches(core.ProcMaskMeleeSpecial) {
			spell.CritDamageBonus += critBonus
		}
	})
}

// applyForeverExposePrey: attacks against a target with the hunter's Hunter's Mark have a
// 5/10% chance to enable Mongoose Bite for 5 sec.
func (hunter *Hunter) applyForeverExposePrey(chance float64) {
	core.MakePermanent(hunter.RegisterAura(core.Aura{
		Label: "Expose Prey (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.ProcMask.Matches(core.ProcMaskMeleeOrRanged) || hunter.HuntersMarkAuras == nil {
				return
			}
			if mark := hunter.HuntersMarkAuras.Get(result.Target); mark != nil && mark.IsActive() && sim.Proc(chance, "Expose Prey") {
				hunter.DefensiveState.Activate(sim)
			}
		},
	}))
}

// applyForeverLaceratingStrikes: Mongoose Bite also bleeds for 40% of its damage over 21 sec
// (7 ticks of 3 sec; a new bite adds to what is left of the bleed).
func (hunter *Hunter) applyForeverLaceratingStrikes() {
	const ticks = 7
	bleed := hunter.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: foreverLaceratingStrikesSpellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagIgnoreAttackerModifiers | core.SpellFlagNoOnCastComplete | core.SpellFlagPassiveSpell,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Lacerating Strikes",
			},
			NumberOfTicks: ticks,
			TickLength:    time.Second * 3,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
	})

	core.MakePermanent(hunter.RegisterAura(core.Aura{
		Label: "Lacerating Strikes Trigger (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || result.Damage <= 0 || !spell.Matches(ClassSpellMask_HunterMongooseBite) {
				return
			}
			dot := bleed.Dot(result.Target)
			total := 0.4*result.Damage + dot.OutstandingDmg()
			dot.SnapshotBaseDamage = total / ticks
			dot.SnapshotAttackerMultiplier = 1
			dot.Apply(sim)
		},
	}))
}

// registerForeverSniperShot: Marksmanship capstone. "A long-range shot that deals ranged
// damage plus 160". 4 sec base cast (hasted like Aimed Shot), 15 sec cooldown (wow.gg Forever MM
// guide). Mana cost is not published (estimate). Single rank; the +160 is not level scaled.
func (hunter *Hunter) registerForeverSniperShot() {
	hunter.SniperShot = hunter.RegisterSpell(core.SpellConfig{
		ClassSpellMask: ClassSpellMask_HunterSniperShot,
		ActionID:       core.ActionID{SpellID: foreverSniperShotSpellID},
		SpellSchool:    core.SpellSchoolPhysical,
		CastType:       proto.CastType_CastTypeRanged,
		DefenseType:    core.DefenseTypeRanged,
		ProcMask:       core.ProcMaskRangedSpecial,
		Flags:          core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		MinRange:     core.MinRangedAttackRange,
		MaxRange:     core.MaxRangedAttackRange,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost: foreverSniperShotBaseManaPercent,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: foreverSniperShotCastTime,
			},
			CD: core.Cooldown{
				Timer:    hunter.NewTimer(),
				Duration: foreverSniperShotCooldown,
			},
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				cast.CastTime = spell.CastTime()
				hunter.AutoAttacks.CancelAutoSwing(sim)
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
			CastTime: func(spell *core.Spell) time.Duration {
				return time.Duration(float64(spell.DefaultCast.CastTime) / hunter.RangedSwingSpeed())
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := hunter.AutoAttacks.Ranged().CalculateNormalizedWeaponDamage(sim, spell.RangedAttackPower(target, false)) +
				hunter.AmmoDamageBonus +
				foreverSniperShotBonusDamage
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)

			hunter.AutoAttacks.EnableAutoSwing(sim)

			spell.WaitTravelTime(sim, func(s *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}

// foreverHuntersMarkAura: Forever's Hunter's Mark grants 71 ranged attack power at rank 4
// (Classic 110); lower ranks are scaled by the same ratio. Same tag and exclusive effect as
// core.HuntersMarkAura so the strongest mark wins.
func (hunter *Hunter) foreverHuntersMarkAura(target *core.Unit) *core.Aura {
	spellID := core.AtLevel(hunter.Level, map[int32]int32{25: 14323, 40: 14324, 50: 14324, 60: 14325})
	bonus := core.AtLevel(hunter.Level, map[int32]float64{25: 45, 40: 75, 50: 75, 60: 110}) * foreverHuntersMarkScale

	aura := target.GetOrRegisterAura(core.Aura{
		Label:    "HuntersMark-Forever-" + strconv.Itoa(int(bonus)),
		Tag:      core.HuntersMarkAuraTag,
		ActionID: core.ActionID{SpellID: spellID},
		Duration: time.Minute * 2,
	})
	aura.NewExclusiveEffect("HuntersMark", true, core.ExclusiveEffect{
		Priority: bonus,
		OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.PseudoStats.BonusRangedAttackPowerTaken += bonus
		},
		OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.PseudoStats.BonusRangedAttackPowerTaken -= bonus
		},
	})
	return aura
}

// registerForeverAspectOfTheBeast: Forever's Aspect of the Beast (level 30) increases melee
// attack power by 50. With Deadly Aspects, melee auto attacks have a 2% per rank chance to
// increase melee attack speed by 30% for 12 sec. Shares the "Aspect" exclusivity with Hawk.
func (hunter *Hunter) registerForeverAspectOfTheBeast() {
	if hunter.Level < 30 {
		return
	}
	var procAura *core.Aura
	procChance := 0.01 * float64(hunter.Talents.ImprovedAspectOfTheHawk) // 2 per Deadly Aspects rank
	if procChance > 0 {
		procAura = hunter.createImprovedHawkAura("Deadly Aspects (Beast)", core.ActionID{SpellID: foreverDeadlyAspectsBeastSpellID}, true)
	}

	actionID := core.ActionID{SpellID: foreverAspectOfTheBeastSpellID}
	aspectAura := hunter.NewTemporaryStatsAuraWrapped(
		"Aspect of the Beast (Forever)",
		actionID,
		stats.Stats{stats.AttackPower: foreverAspectOfTheBeastAP},
		core.NeverExpires,
		func(aura *core.Aura) {
			aura.OnSpellHitDealt = func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if procAura != nil && spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && sim.Proc(procChance, "Deadly Aspects (Beast)") {
					procAura.Activate(sim)
				}
			}
		})
	aspectAura.NewExclusiveEffect("Aspect", true, core.ExclusiveEffect{})

	hunter.RegisterSpell(core.SpellConfig{
		ActionID:      actionID,
		Flags:         core.SpellFlagAPL,
		RequiredLevel: 30,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return !aspectAura.IsActive()
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			aspectAura.Activate(sim)
		},
	})
}

// registerForeverStriderKick: Survival talent. "A powerful kick that deals 100% melee weapon
// damage and increases movement speed by 30% for 3 sec." 8 sec cooldown (wow.gg Forever
// Survival overview, which calls it Runner's Strike). Mana cost not published (estimate). The
// movement speed clause does nothing in a sim.
func (hunter *Hunter) registerForeverStriderKick() {
	hunter.StriderKick = hunter.RegisterSpell(core.SpellConfig{
		ClassSpellMask: ClassSpellMask_HunterStriderKick,
		ActionID:       core.ActionID{SpellID: foreverStriderKickSpellID},
		SpellSchool:    core.SpellSchoolPhysical,
		CastType:       proto.CastType_CastTypeMainHand,
		DefenseType:    core.DefenseTypeMelee,
		ProcMask:       core.ProcMaskMeleeMHSpecial,
		Flags:          core.SpellFlagMeleeMetrics | core.SpellFlagAPL,
		MaxRange:       core.MaxMeleeAttackRange,

		ManaCost: core.ManaCostOptions{
			BaseCost: foreverStriderKickBaseManaPct,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
			CD: core.Cooldown{
				Timer:    hunter.NewTimer(),
				Duration: foreverStriderKickCooldown,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			damage := hunter.MHWeaponDamage(sim, spell.MeleeAttackPower())
			spell.CalcAndDealDamage(sim, target, damage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
		},
	})
}

// registerForeverLacerate: new Forever Survival ability. "Wounds the target causing them to
// bleed for 406 damage over 21 sec." (rank 4, level 60, requires a melee weapon). Only rank 4
// is published, so it is registered at level 60 only. Mana cost and cooldown are not
// published (estimate: 5% of base mana, no cooldown). No attack power scaling in the tooltip.
func (hunter *Hunter) registerForeverLacerate() {
	if hunter.Level < foreverLacerateLevel || !hunter.HasMHWeapon() {
		return
	}
	const ticks = 7
	hunter.Lacerate = hunter.RegisterSpell(core.SpellConfig{
		ClassSpellMask: ClassSpellMask_HunterLacerate,
		ActionID:       core.ActionID{SpellID: foreverLacerateSpellID},
		SpellSchool:    core.SpellSchoolPhysical,
		DefenseType:    core.DefenseTypeMelee,
		ProcMask:       core.ProcMaskMeleeMHSpecial,
		Flags:          core.SpellFlagAPL | core.SpellFlagMeleeMetrics,
		MaxRange:       core.MaxMeleeAttackRange,
		RequiredLevel:  foreverLacerateLevel,

		ManaCost: core.ManaCostOptions{
			BaseCost: foreverLacerateBaseManaPct,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Lacerate (Forever)",
			},
			NumberOfTicks: ticks,
			TickLength:    time.Second * 3,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, foreverLacerateTotalDamage/ticks, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHitNoHitCounter)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			}
			spell.DealOutcome(sim, result)
		},
	})
}
