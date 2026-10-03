package paladin

import (
	"strings"
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever paladin talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/paladin.json) and the Forever baseline spell changes the
// talents build on. Everything here only runs when the player sent a Forever talent string
// (ForeverTalents != nil); Classic/SoD sims are unchanged.
//
// Baseline changes modeled (Forever spellbook, foreverchanges.pro / Sept 24 beta patch notes):
//   - Holy Strike: new baseline melee strike, 8 ranks, 10 sec cooldown.
//   - Seal of Fury: new seal, 7 ranks (35 Holy per melee hit, Judgement 154-167 at rank 7).
//   - Hammer of the Righteous: baseline at level 40, 3x main-hand DPS as Holy (SoD rune
//     implementation granted, damage/threat corrected).
//   - Judgement does not consume the Seal.
//   - Consecration is baseline (no talent) and retuned (consecration.go).
//   - Holy Shield grants 20% block (holy_shield.go); its damage comes from spell_tuning.csv.
//
// Talents mapped onto the Classic engine talents: Divine Strength, Divine Intellect, Healing
// Light, Spiritual Focus, Unyielding Faith, Divine Favor, Holy Shock, Toughness, Precision,
// Guardian's Favor, Anticipation (4 Defense per rank = 2x Classic), Shield Specialization
// (block value part), Improved Hammer of Justice, Reckoning (crit part), Holy Shield,
// Deflection, Improved Judgement, Conviction, Seal of Command, Pursuit of Justice,
// Eye for an Eye, Two-Handed Weapon Specialization, Repentance, Holy Power (+1% Holy crit
// per rank, as Classic).
//
// Forever-specific (applyForeverTalents): Benediction, Holy Conduit, Vindication, Sanctified
// Judgement, Sacred Arbiter, Vengeance, Champion of the Light, Instrument of Law, Twist of
// Light, Improved Seals, Reverence, Purifying Power, Divine Precision, Holy Power (Holy
// Strike/Holy Shock part), Redoubt, Improved Righteous Fury, Shield Specialization (mana on
// block), Sacred Duty, One-Handed Weapon Specialization, Reckoning (block part), Iron Creed.
//
// Not modeled (healing, utility, PvP or no effect on a single-target DPS/threat sim):
// Infusion of Light, Illumination, Light's Vigil, Voice of Truth, Consecrated Ground (needs
// enemies entering Consecration), Improved Seal of Fury and Seal of Fury's absorb shield
// (absorbs aren't simulated for the tank), Swift Judgement, Templar's Bulwark, Iron Creed's
// damage reduction is modeled but Seal of Fury's taunt is not.

const (
	// Forever's new spells have no public spell IDs yet.
	foreverHolyStrikeSpellID      = 1294001
	foreverSealOfFurySpellID      = 1294002
	foreverSealOfFuryProcID       = 1294003
	foreverJudgementOfFurySpellID = 1294004
	foreverEchoSpellID            = 1294005 // Twist of Light's Echo
	foreverIronCreedSpellID       = 1294006
)

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from NewPaladin.
func (paladin *Paladin) useForeverTalentLayout() {
	ft := paladin.ForeverTalents
	if ft == nil {
		return
	}
	paladin.Talents = &proto.PaladinTalents{
		// Holy
		DivineStrength:  ft.Rank("Divine Strength"),
		DivineIntellect: ft.Rank("Divine Intellect"),
		SpiritualFocus:  ft.Rank("Spiritual Focus"),
		HealingLight:    ft.Rank("Healing Light"),
		Consecration:    true, // baseline in Forever
		UnyieldingFaith: ft.Rank("Unyielding Faith"),
		DivineFavor:     ft.Has("Divine Favor"),
		HolyPower:       ft.Rank("Holy Power"),
		HolyShock:       ft.Has("Holy Shock"),
		// Protection (Redoubt, Improved Righteous Fury, One-Handed Weapon Specialization
		// work differently and are applied in applyForeverTalents)
		Precision:               ft.Rank("Precision"),
		GuardiansFavor:          ft.Rank("Guardian's Favor"),
		Toughness:               ft.Rank("Toughness"),
		ShieldSpecialization:    ft.Rank("Shield Specialization"),
		Anticipation:            2 * ft.Rank("Anticipation"), // 4 Defense per rank (Classic: 2)
		ImprovedHammerOfJustice: ft.Rank("Improved Hammer of Justice"),
		Reckoning:               ft.Rank("Reckoning"), // crit part: 20% per rank, as Classic
		HolyShield:              ft.Has("Holy Shield"),
		// Retribution (Benediction and Vengeance/Vindication are Forever versions)
		ImprovedJudgement:             ft.Rank("Improved Judgement"),
		Deflection:                    ft.Rank("Deflection"),
		Conviction:                    ft.Rank("Conviction"),
		SealOfCommand:                 ft.Has("Seal of Command"),
		PursuitOfJustice:              ft.Rank("Pursuit of Justice"),
		EyeForAnEye:                   ft.Rank("Eye for an Eye"),
		TwoHandedWeaponSpecialization: ft.Rank("Two-Handed Weapon Specialization"),
		Repentance:                    ft.Has("Repentance"),
	}
	if paladin.Level >= 40 {
		// Hammer of the Righteous is a level 40 baseline ability in Forever.
		paladin.GrantForeverRune(int32(proto.PaladinRune_RuneWristHammerOfTheRighteous))
	}
}

// foreverWeaponSpecializationModifier: Two-Handed Weapon Specialization 2/4/6%,
// One-Handed Weapon Specialization 3/7/10%.
func (paladin *Paladin) foreverWeaponSpecializationModifier() float64 {
	switch paladin.MainHand().HandType {
	case proto.HandType_HandTypeMainHand, proto.HandType_HandTypeOneHand:
		return 1 + []float64{0, 0.03, 0.07, 0.10}[paladin.ForeverTalents.Rank("One-Handed Weapon Specialization")]
	case proto.HandType_HandTypeTwoHand:
		return 1 + 0.02*float64(paladin.ForeverTalents.Rank("Two-Handed Weapon Specialization"))
	}
	return 1
}

// sealCostMultiplier is the mana cost multiplier of Seal spells (Benediction, Twist of Light).
func (paladin *Paladin) sealCostMultiplier() int32 {
	if paladin.ForeverTalents != nil && paladin.ForeverTalents.Has("Twist of Light") {
		return paladin.benediction() - 20
	}
	return paladin.benediction()
}

func (paladin *Paladin) applyForeverTalents() {
	ft := paladin.ForeverTalents
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
	sealsAndJudgements := ClassSpellMask_PaladinSealOfCommand | ClassSpellMask_PaladinSealOfMartyrdom | ClassSpellMask_PaladinSealOfRighteousness |
		ClassSpellMask_PaladinSealOfFury | ClassSpellMask_PaladinJudgements | ClassSpellMask_PaladinJudgementOfFury

	// ---------------- Holy ----------------
	if r := rank("Improved Seals"); r > 0 {
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Pct, ClassMask: sealsAndJudgements, FloatValue: 1 + 0.05*r})
	}
	if r := rank("Reverence"); r > 0 {
		paladin.PseudoStats.SpiritRegenRateCasting += 0.1 * r
	}
	if v := byRank("Purifying Power", 17, 33); v > 0 {
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Cooldown_Multi_Flat, ClassMask: ClassSpellMask_PaladinExorcism | ClassSpellMask_PaladinHolyWrath, IntValue: -int64(v)})
	}
	if r := rank("Divine Precision"); r > 0 {
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, School: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, FloatValue: 6 * r * core.SpellHitRatingPerHitChance})
	}
	if r := rank("Holy Power"); r > 0 {
		// +3% per rank on Holy Shock and Holy Strike; the Classic +1% Holy crit (mapped) covers 1% of it.
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_PaladinHolyShock | ClassSpellMask_PaladinHolyStrike, FloatValue: 2 * r * core.CritRatingPerCritChance})
	}

	// ---------------- Protection ----------------
	if r := rank("Redoubt"); r > 0 {
		paladin.applyForeverRedoubt(r)
	}
	if r := rank("Improved Righteous Fury"); r > 0 && paladin.Options.RighteousFury {
		paladin.PseudoStats.DamageTakenMultiplier *= 1 - 0.02*r
	}
	if r := rank("Shield Specialization"); r > 0 {
		paladin.applyForeverShieldSpecialization(r)
	}
	if r := rank("Sacred Duty"); r > 0 {
		paladin.MultiplyStat(stats.Stamina, 1+0.02*r)
	}
	if r := rank("Reckoning"); r > 0 {
		core.MakeProcTriggerAura(&paladin.Unit, core.ProcTrigger{
			Name:       "Reckoning Block Trigger (Forever)",
			Callback:   core.CallbackOnSpellHitTaken,
			Outcome:    core.OutcomeBlock,
			ProcMask:   core.ProcMaskMelee,
			ProcChance: 0.08 * r,
			Handler: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if result.DidBlock() {
					paladin.AutoAttacks.ExtraMHAttackProc(sim, 1, core.ActionID{SpellID: 20178}, spell)
				}
			},
		})
	}
	if r := rank("Iron Creed"); r > 0 {
		paladin.applyForeverIronCreed(r)
	}

	// ---------------- Retribution ----------------
	instants := ClassSpellMask_PaladinHolyStrike | ClassSpellMask_PaladinConsecration | ClassSpellMask_PaladinHammerOfTheRighteous |
		ClassSpellMask_PaladinHolyShield | ClassSpellMask_PaladinExorcism | ClassSpellMask_PaladinHolyShock
	if r := rank("Benediction"); r > 0 {
		// Seals and Judgement get it through benediction().
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Pct, ClassMask: instants, IntValue: -int64(2 * r)})
	}
	if r := rank("Holy Conduit"); r > 0 {
		paladin.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_PowerCost_Pct,
			ClassMask: ClassSpellMask_PaladinConsecration | ClassSpellMask_PaladinHolyWrath | ClassSpellMask_PaladinExorcism | ClassSpellMask_PaladinHammerOfWrath,
			IntValue:  -int64(20 * r),
		})
	}
	if r := rank("Vindication"); r > 0 {
		paladin.applyForeverVindication(r)
	}
	if v := byRank("Sanctified Judgement", 1.0/3, 2.0/3, 1); v > 0 {
		paladin.foreverSanctifiedChance = v
		paladin.foreverSanctifiedReturn = 0.2 * rank("Sanctified Judgement")
		paladin.foreverSanctifiedMetrics = paladin.NewManaMetrics(core.ActionID{SpellID: 31930})
	}
	if ft.Has("Sacred Arbiter") {
		paladin.foreverSacredArbiter = true
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Pct, ClassMask: ClassSpellMask_PaladinHolyStrike, FloatValue: 1.2})
	}
	if r := rank("Vengeance"); r > 0 {
		paladin.applyForeverVengeance(r)
	}
	if v := byRank("Champion of the Light", 0.33, 0.66, 1); v > 0 {
		paladin.AddStatDependency(stats.Intellect, stats.SpellPower, v)
	}
	if r := rank("Instrument of Law"); r > 0 {
		paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_CastTime_Flat, ClassMask: ClassSpellMask_PaladinHammerOfWrath, TimeValue: -time.Duration(r) * 500 * time.Millisecond})
		if !paladin.Options.RighteousFury {
			paladin.PseudoStats.ThreatMultiplier *= 1 - 0.1*r
		}
	}
	if ft.Has("Twist of Light") {
		paladin.foreverEchoAura = paladin.RegisterAura(core.Aura{
			Label:    "Echo of the Seal",
			ActionID: core.ActionID{SpellID: foreverEchoSpellID},
			Duration: time.Second * 30,
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				if seal := paladin.foreverEchoSeal; seal != nil && seal != paladin.currentSeal {
					seal.Deactivate(sim)
				}
				paladin.foreverEchoSeal = nil
			},
		})
		core.MakePermanent(paladin.RegisterAura(core.Aura{
			Label: "Twist of Light (Forever)",
			OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				// The swing applies both Seals (their own auras proc on it), then the Echo is used up.
				if spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && paladin.foreverEchoAura.IsActive() {
					echo := paladin.foreverEchoAura
					core.StartDelayedAction(sim, core.DelayedActionOptions{
						DoAt:     sim.CurrentTime,
						Priority: core.ActionPriorityLow,
						OnAction: func(sim *core.Simulation) { echo.Deactivate(sim) },
					})
				}
			},
		}))
	}

	// Hammer of the Righteous (Forever, level 40): 3x weapon DPS (SoD rune: 4x) and no SoD 2x threat.
	paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Pct, ClassMask: ClassSpellMask_PaladinHammerOfTheRighteous, FloatValue: 0.75})
	paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Threat_Pct, ClassMask: ClassSpellMask_PaladinHammerOfTheRighteous, FloatValue: 0.5})
}

// foreverEcho (Twist of Light): replacing a Seal of Command, Righteousness, Fury (or Justice)
// keeps it as an Echo that the next melee swing also applies.
func (paladin *Paladin) foreverEcho(sim *core.Simulation, oldSeal *core.Aura) bool {
	if paladin.foreverEchoAura == nil {
		return false
	}
	if !strings.HasPrefix(oldSeal.Label, "Seal of Command") && !strings.HasPrefix(oldSeal.Label, "Seal of Righteousness") &&
		!strings.HasPrefix(oldSeal.Label, "Seal of Fury") {
		return false
	}
	if prev := paladin.foreverEchoSeal; prev != nil && prev != oldSeal && prev != paladin.currentSeal {
		prev.Deactivate(sim)
	}
	paladin.foreverEchoSeal = oldSeal
	paladin.foreverEchoAura.Activate(sim)
	paladin.foreverEchoAura.Refresh(sim)
	return true
}

// foreverJudge: Forever's Judgement unleashes the active Seal without consuming it.
func (paladin *Paladin) foreverJudge(sim *core.Simulation, target *core.Unit) {
	if paladin.currentJudgement == nil {
		return
	}
	paladin.currentJudgement.Cast(sim, target)

	if paladin.foreverSanctifiedChance > 0 && sim.Proc(paladin.foreverSanctifiedChance, "Sanctified Judgement") {
		if seal := paladin.GetSpell(paladin.currentSeal.ActionID); seal != nil && seal.Cost != nil {
			paladin.AddMana(sim, paladin.foreverSanctifiedReturn*seal.Cost.BaseCost, paladin.foreverSanctifiedMetrics)
		}
	}
}

func (paladin *Paladin) applyForeverVindication(points float64) {
	dep := paladin.NewDynamicMultiplyStat(stats.AttackPower, 1+0.01*points)
	aura := paladin.RegisterAura(core.Aura{
		Label:    "Vindication (Forever)",
		ActionID: core.ActionID{SpellID: 26021},
		Duration: time.Second * 30,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			paladin.EnableDynamicStatDep(sim, dep)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			paladin.DisableDynamicStatDep(sim, dep)
		},
	})
	// Proc chance isn't published; like the Classic implementation, any landed melee hit.
	core.MakePermanent(paladin.RegisterAura(core.Aura{
		Label: "Vindication Trigger (Forever)",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.ProcMask.Matches(core.ProcMaskMelee) {
				aura.Activate(sim)
			}
		},
	}))
}

// applyForeverVengeance: +1/2/3% Physical and Holy damage per stack for 30 sec after a
// non-periodic critical strike, up to 3 stacks.
func (paladin *Paladin) applyForeverVengeance(points float64) {
	perStack := 0.01 * points
	aura := paladin.RegisterAura(core.Aura{
		Label:     "Vengeance (Forever)",
		ActionID:  core.ActionID{SpellID: 20059},
		Duration:  time.Second * 30,
		MaxStacks: 3,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks, newStacks int32) {
			mult := (1 + perStack*float64(newStacks)) / (1 + perStack*float64(oldStacks))
			aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexHoly] *= mult
			aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= mult
		},
	})
	core.MakePermanent(paladin.RegisterAura(core.Aura{
		Label: "Vengeance Trigger (Forever)",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidCrit() {
				aura.Activate(sim)
				aura.AddStack(sim)
			}
		},
	}))
}

// applyForeverRedoubt: damaging melee attacks against you have a 10% chance to grant
// +6% block per rank for 10 sec or 5 blocks.
func (paladin *Paladin) applyForeverRedoubt(points float64) {
	paladin.redoubtAura = paladin.RegisterAura(core.Aura{
		Label:     "Redoubt",
		ActionID:  core.ActionID{SpellID: 20134},
		Duration:  time.Second * 10,
		MaxStacks: 5,
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidBlock() {
				aura.RemoveStack(sim)
			}
		},
	}).AttachStatBuff(stats.Block, 6*points*core.BlockRatingPerBlockChance)

	core.MakeProcTriggerAura(&paladin.Unit, core.ProcTrigger{
		Name:       "Redoubt Trigger (Forever)",
		Callback:   core.CallbackOnSpellHitTaken,
		Outcome:    core.OutcomeLanded,
		ProcMask:   core.ProcMaskMelee,
		ProcChance: 0.1,
		Handler: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			paladin.redoubtAura.Activate(sim)
			paladin.redoubtAura.SetStacks(sim, 5)
		},
	})
}

// applyForeverShieldSpecialization: blocks have a 33/66/100% chance to restore 6% of maximum
// Mana (beta tooltip, no internal cooldown listed). The block value part is mapped.
func (paladin *Paladin) applyForeverShieldSpecialization(points float64) {
	metrics := paladin.NewManaMetrics(core.ActionID{SpellID: 20150})
	core.MakeProcTriggerAura(&paladin.Unit, core.ProcTrigger{
		Name:       "Shield Specialization (Forever)",
		Callback:   core.CallbackOnSpellHitTaken,
		Outcome:    core.OutcomeBlock,
		ProcMask:   core.ProcMaskMelee,
		ProcChance: []float64{0, 0.33, 0.66, 1}[int(points)],
		Handler: func(sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidBlock() {
				paladin.AddMana(sim, 0.06*paladin.MaxMana(), metrics)
			}
		},
	})
}

// applyForeverIronCreed: Holy Strike threat +5% per rank; with Righteous Fury, Holy Strike
// also reduces damage taken by 2% per rank for 6 sec.
func (paladin *Paladin) applyForeverIronCreed(points float64) {
	paladin.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Threat_Pct, ClassMask: ClassSpellMask_PaladinHolyStrike, FloatValue: 1 + 0.05*points})
	if !paladin.Options.RighteousFury {
		return
	}
	reduction := 1 - 0.02*points
	aura := paladin.RegisterAura(core.Aura{
		Label:    "Iron Creed",
		ActionID: core.ActionID{SpellID: foreverIronCreedSpellID},
		Duration: time.Second * 6,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.DamageTakenMultiplier *= reduction
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.DamageTakenMultiplier /= reduction
		},
	})
	core.MakePermanent(paladin.RegisterAura(core.Aura{
		Label: "Iron Creed Trigger",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.Matches(ClassSpellMask_PaladinHolyStrike) && result.Landed() {
				aura.Activate(sim)
			}
		},
	}))
}

// foreverRank returns the index of the highest rank learned at the paladin's level (the
// first rank if none is learned yet).
func (paladin *Paladin) foreverRank(levels []int32) int {
	best := 0
	for i, level := range levels {
		if level <= paladin.Level {
			best = i
		}
	}
	return best
}

// registerForeverSpells registers Forever's new baseline spells. Called from Initialize.
func (paladin *Paladin) registerForeverSpells() {
	if paladin.ForeverTalents == nil {
		return
	}
	paladin.registerForeverHolyStrike()
	paladin.registerForeverSealOfFury()
}

// registerForeverHolyStrike: instant strike for X% weapon damage plus Holy damage, 10 sec
// cooldown. Rank 8 (level 60): 50% weapon damage + 81-105 Holy. Weapon % per rank from the
// Sept 24 patch notes (25/29/32/36/39/43/46/50%). Estimates: rank learn levels (8 ranks from
// level 6), lower ranks' Holy damage (scaled by learn level), mana cost (5% of base mana),
// normalized weapon damage, and the whole strike rolling as one Holy melee attack.
func (paladin *Paladin) registerForeverHolyStrike() {
	levels := []int32{6, 12, 20, 28, 36, 44, 52, 60}
	weaponPct := []float64{0.25, 0.29, 0.32, 0.36, 0.39, 0.43, 0.46, 0.50}
	r := paladin.foreverRank(levels)
	scale := float64(levels[r]) / 60
	minDamage, maxDamage := 81*scale, 105*scale
	pct := weaponPct[r]

	paladin.holyStrike = paladin.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: foreverHolyStrikeSpellID},
		SpellSchool:    core.SpellSchoolHoly,
		DefenseType:    core.DefenseTypeMelee,
		ProcMask:       core.ProcMaskMeleeMHSpecial,
		Flags:          core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlag_RV | core.SpellFlagIgnoreResists | core.SpellFlagBatchStartAttackMacro,
		ClassSpellMask: ClassSpellMask_PaladinHolyStrike,

		RequiredLevel: int(levels[0]),
		Rank:          r + 1,

		ManaCost: core.ManaCostOptions{BaseCost: 0.05},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{GCD: core.GCDDefault},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    paladin.NewTimer(),
				Duration: time.Second * 10,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return paladin.MainHand().WeaponType != proto.WeaponType_WeaponTypeUnknown
		},

		DamageMultiplier: paladin.getWeaponSpecializationModifier(),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := pct*spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower()) + sim.Roll(minDamage, maxDamage)
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if paladin.foreverSacredArbiter && result.Landed() {
				for _, aura := range target.GetAurasWithTag(core.JudgementAuraTag) {
					if aura.IsActive() && aura.Duration < core.NeverExpires {
						aura.Refresh(sim)
					}
				}
			}
		},
	})
}

// registerForeverSealOfFury: melee attacks deal additional Holy damage (35 at rank 7, level
// 58); Judgement deals 154-167 Holy damage (and taunts, not modeled). Estimates: rank learn
// levels (Seal of Righteousness ranks 2-8), lower ranks scaled by learn level, mana costs
// (Seal of Righteousness of the same level), coefficients (0.05 per hit, 0.5 Judgement).
// The absorb shield it grants with a shield equipped isn't modeled.
func (paladin *Paladin) registerForeverSealOfFury() {
	levels := []int32{10, 18, 26, 34, 42, 50, 58}
	manaCosts := []float64{40, 60, 90, 120, 140, 170, 200}
	if paladin.Level < levels[0] {
		return
	}
	r := paladin.foreverRank(levels)
	scale := float64(levels[r]) / 58
	hitDamage := 35 * scale
	judgeMin, judgeMax := 154*scale, 167*scale

	judgeSpell := paladin.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: foreverJudgementOfFurySpellID},
		SpellSchool:    core.SpellSchoolHoly,
		DefenseType:    core.DefenseTypeMagic,
		ProcMask:       core.ProcMaskSpellDamage,
		Flags:          core.SpellFlagMeleeMetrics | SpellFlag_RV | core.SpellFlagSuppressWeaponProcs | core.SpellFlagSuppressEquipProcs | core.SpellFlagBinary,
		ClassSpellMask: ClassSpellMask_PaladinJudgementOfFury,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 0.5,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealDamage(sim, target, sim.Roll(judgeMin, judgeMax), spell.OutcomeMagicHitAndCrit)
		},
	})

	procSpell := paladin.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: foreverSealOfFuryProcID},
		SpellSchool:    core.SpellSchoolHoly,
		DefenseType:    core.DefenseTypeMelee,
		ProcMask:       core.ProcMaskMeleeMHSpecial,
		Flags:          core.SpellFlagMeleeMetrics | core.SpellFlagSuppressEquipProcs,
		ClassSpellMask: ClassSpellMask_PaladinSealOfFury,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 0.05,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealDamage(sim, target, hitDamage, spell.OutcomeMeleeSpecialCritOnly)
		},
	})

	aura := paladin.RegisterAura(core.Aura{
		Label:    "Seal of Fury" + paladin.Label,
		ActionID: core.ActionID{SpellID: foreverSealOfFurySpellID},
		Duration: time.Second * 30,
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) {
				procSpell.Cast(sim, result.Target)
			}
		},
	})
	paladin.aurasSoF = append(paladin.aurasSoF, aura)

	paladin.sealOfFury = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    aura.ActionID,
		SpellSchool: core.SpellSchoolHoly,
		Flags:       core.SpellFlagAPL | core.SpellFlagBatchStartAttackMacro,

		RequiredLevel: int(levels[0]),
		Rank:          r + 1,

		ManaCost: core.ManaCostOptions{
			FlatCost:   manaCosts[r] - paladin.getLibramSealCostReduction(),
			Multiplier: paladin.sealCostMultiplier(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{GCD: core.GCDDefault},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			paladin.applySeal(aura, judgeSpell, sim)
		},
	})
}
