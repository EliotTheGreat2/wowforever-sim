package druid

import (
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever druid talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/druid.json).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents. Forever druid spellbook changes
// that only matter with Forever talents are also switched on here: Omen of Clarity is trained
// (level 20) instead of a talent, Lacerate is a trained bear ability (level 42), Shred deals
// 155% weapon damage, and Furor's Cat Form part works differently.
//
// Not modeled (no effect on a single-target DPS/threat sim, or not enough data):
// Improved Entangling Roots, Overgrowth, Improved Starfire's stun, Feral Instinct's stealth part,
// Brutal Impact, Thick Hide (armor only), Feral Charge, Leader of the Pack/Moonkin Form party
// auras beyond the existing raid buff, Berserk's Fear immunity, Nature's Focus, Subtlety,
// Gift of Nature, Gift of the Earthmother, Tranquil Spirit, Improved Rejuvenation, Swiftmend,
// Nature's Swiftness, Improved Tranquility, Improved Regrowth, Wild Growth (healing).
//
// Estimates (no Forever data yet), all marked in the code below:
//   - Primal Bite (renamed SoD/TBC Mangle (Bear)): 15 Rage, 6 sec cooldown, 1.5x threat (the SoD
//     rune's values); flat damage 26/43/60/77 for ranks learned at 25/40/50/60 (ranks 1 and 4 known).
//   - Berserk: 3 min cooldown, 15 sec (cooldown per community write-ups).
//   - Shifting Power: 30 sec cooldown, 1 sec GCD.
//   - Lacerate ranks 1-2 (levels 42/50): 55/65 bleed damage per stack (rank 3: 75); 10 Rage.
//   - Omen of Clarity on spells: 6% per damaging spell cast (12% in Moonkin Form).
//   - Classic Shred flat damage per rank (54/72/99/144/180) with Forever's 155% weapon damage.

const (
	foreverPrimalBiteSpellID    = 407995  // no Forever spell ID; SoD Mangle (Bear) is used for name/icon
	foreverBerserkSpellID       = 417141  // SoD Berserk's ID for name/icon
	foreverShiftingPowerSpellID = 1290100 // no public spell ID
	foreverEclipseSpellID       = 408255  // SoD Lunar Eclipse's ID for name/icon
)

var foreverBalanceSpells = ClassSpellMask_DruidWrath | ClassSpellMask_DruidStarfire | ClassSpellMask_DruidMoonfire |
	ClassSpellMask_DruidInsectSwarm | ClassSpellMask_DruidHurricane

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from New, before any spell registers.
func (druid *Druid) useForeverTalentLayout() {
	ft := druid.ForeverTalents
	if ft == nil {
		return
	}
	druid.Talents = &proto.DruidTalents{
		// Balance
		ImprovedWrath:    ft.Rank("Improved Wrath"),
		Vengeance:        ft.Rank("Vengeance"),
		ImprovedStarfire: ft.Rank("Improved Starfire"),
		MoonkinForm:      ft.Has("Moonkin Form"),
		InsectSwarm:      ft.Has("Insect Swarm"),
		// Omen of Clarity is a trained spell in Forever.
		OmenOfClarity: druid.Level >= 20,
		// Feral
		Ferocity:         ft.Rank("Ferocity"),
		FelineSwiftness:  ft.Rank("Feral Swiftness"),
		BrutalImpact:     ft.Rank("Brutal Impact"),
		FeralCharge:      ft.Has("Feral Charge"),
		ImprovedShred:    ft.Rank("Shredding Attacks"), // -6 Shred Energy per rank, as Classic Improved Shred
		PredatoryStrikes: ft.Rank("Predatory Strikes"),
		// Forever's Blood Frenzy = Classic Primal Fury (bear Rage) + Classic Blood Frenzy (cat combo points).
		BloodFrenzy:     ft.Rank("Blood Frenzy"),
		PrimalFury:      ft.Rank("Blood Frenzy"),
		LeaderOfThePack: ft.Has("Leader of the Pack"),
		// Restoration (Furor's bear Rage works as in Classic; its Cat Form Energy is foreverFurorCatEnergy)
		Furor:               ft.Rank("Furor"),
		NaturalShapeshifter: ft.Rank("Natural Shapeshifter"),
	}

	// Sharpened Claws: 3% per rank (Classic: 2%).
	druid.foreverFormCrit = 3 * float64(ft.Rank("Sharpened Claws"))
	// Heart of the Wild: Intellect and Cat Strength 2% per rank (Classic 4%), Bear Stamina 4%.
	druid.foreverHotwCatStr = 0.02 * float64(ft.Rank("Heart of the Wild"))
	druid.foreverHotwBearStam = 0.04 * float64(ft.Rank("Heart of the Wild"))

	// Lacerate is a trained bear ability from level 42 (ranks at 42/50/58). Rank 3: 75 damage over
	// 15 sec per stack plus 10% weapon damage per existing application; ranks 1-2 are estimates.
	if druid.Level >= 42 {
		druid.GrantForeverRune(int32(proto.DruidRune_RuneLegsLacerate))
		druid.foreverLacerateTick = foreverStepValue(druid.Level, map[int32]float64{42: 55, 50: 65, 58: 75}) / 5
		druid.foreverLacerateWeaponPct = 0.10
	}
}

func (druid *Druid) applyForeverTalents() {
	ft := druid.ForeverTalents
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

	// ---------------- Balance ----------------
	if r := rank("Improved Wrath"); r > 0 {
		// Cast time is the mapped Classic talent; Forever adds -10% Mana per rank.
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Pct, ClassMask: ClassSpellMask_DruidWrath, IntValue: -int64(10 * r)})
	}
	if r := rank("Genesis"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PeriodicDamageDone_Flat, ClassMask: ClassSpellMask_DruidAll, IntValue: int64(r)})
		// Lacerate's bleed is its own spell without a class mask.
		druid.RegisterAura(core.Aura{
			Label: "Genesis Lacerate (Forever)",
			OnInit: func(aura *core.Aura, sim *core.Simulation) {
				if druid.LacerateBleed != nil {
					druid.LacerateBleed.ApplyAdditivePeriodicDamageBonus(int64(r))
				}
			},
		})
	}
	if v := byRank("Moonglow", 8, 17, 25); v > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Pct, ClassMask: foreverBalanceSpells, IntValue: -int64(v)})
	}
	if r := rank("Improved Moonfire"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_DruidMoonfire, IntValue: int64(5 * r)})
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_DruidMoonfire, FloatValue: 5 * r * core.SpellCritRatingPerCritChance})
	}
	if r := rank("Nature's Majesty"); r > 0 {
		druid.AddStats(stats.Stats{
			stats.SpellCrit: 2 * r * core.SpellCritRatingPerCritChance,
			stats.MeleeCrit: 2 * r * core.CritRatingPerCritChance,
		})
	}
	if r := rank("Nature's Reach"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, ClassMask: foreverBalanceSpells | ClassSpellMask_DruidFaerieFire, FloatValue: 2 * r * core.SpellHitRatingPerHitChance})
	}
	if ft.Has("Nature's Splendor") {
		// Moonfire +3 sec (one 3 sec tick), Insect Swarm +2 sec (one 2 sec tick).
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DotNumberOfTicks_Flat, ClassMask: ClassSpellMask_DruidMoonfire | ClassSpellMask_DruidInsectSwarm, IntValue: 1})
	}
	if ft.Has("Nature's Grace") {
		druid.applyForeverNaturesGrace()
	}
	if r := rank("Eclipse"); r > 0 {
		druid.applyForeverEclipse(byRank("Eclipse", 170, 330, 500))
	}
	if r := rank("Moonfury"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, School: core.SpellSchoolArcane | core.SpellSchoolNature, ClassMask: ClassSpellMask_DruidAll, IntValue: int64(2 * r)})
	}
	if druid.Talents.OmenOfClarity {
		druid.applyForeverOmenOfClaritySpells()
	}

	// ---------------- Feral Combat ----------------
	if r := rank("Heart of the Wild"); r > 0 {
		druid.MultiplyStat(stats.Intellect, 1+0.02*r)
	}
	if r := rank("Feral Instinct"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_DruidSwipeBear | ClassSpellMask_DruidSwipeCat, IntValue: int64(10 * r)})
	}
	if r := rank("Shredding Attacks"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Flat, ClassMask: ClassSpellMask_DruidLacerate, IntValue: -int64(r)})
	}
	if r := rank("Savage Fury"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_DamageDone_Flat,
			ClassMask: ClassSpellMask_DruidRake | ClassSpellMask_DruidShred | ClassSpellMask_DruidMaul | ClassSpellMask_DruidSwipeBear | ClassSpellMask_DruidSwipeCat,
			IntValue:  int64(5 * r),
		})
	}
	if r := rank("Predatory Instincts"); r > 0 {
		druid.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_CritDamageBonus_Flat, ClassMask: ClassSpellMask_DruidAll, ProcMask: core.ProcMaskMeleeSpecial, FloatValue: 0.1 * r})
	}
	if r := rank("Natural Reaction"); r > 0 {
		druid.applyForeverNaturalReaction(r)
	}
	if r := rank("Rend and Tear"); r > 0 {
		druid.applyForeverRendAndTear(1 + 0.02*r)
	}
	if ft.Has("Primal Bite") {
		druid.registerForeverPrimalBite()
	}
	if ft.Has("Shifting Power") {
		druid.registerForeverShiftingPower(time.Second*30 - time.Second*4*time.Duration(ft.Rank("Improved Shifting Power")))
	}
	if ft.Has("Berserk") {
		druid.registerForeverBerserk()
	}

	// ---------------- Restoration ----------------
	if r := rank("Naturalist"); r > 0 {
		druid.PseudoStats.DamageDealtMultiplier *= 1 + 0.01*r
	}
	if v := byRank("Reflection", 0.17, 0.33, 0.50); v > 0 {
		druid.PseudoStats.SpiritRegenRateCasting += v
	}
	if r := rank("Living Spirit"); r > 0 {
		druid.MultiplyStat(stats.Spirit, 1+0.05*r)
	}
}

// foreverStepValue returns the value of the highest rank learned at the given level, from
// {learn level: value} (the lowest rank if none is learned yet).
func foreverStepValue(level int32, ranks map[int32]float64) float64 {
	best, bestLevel := 0.0, int32(-1)
	lowest, lowestLevel := 0.0, int32(1000)
	for l, v := range ranks {
		if l <= level && l > bestLevel {
			best, bestLevel = v, l
		}
		if l < lowestLevel {
			lowest, lowestLevel = v, l
		}
	}
	if bestLevel < 0 {
		return lowest
	}
	return best
}

// foreverShredFlatDamage: Classic Shred ranks (learned at 22/30/38/46/54); Forever keeps rank 5's +180.
func foreverShredFlatDamage(level int32) float64 {
	return foreverStepValue(level, map[int32]float64{22: 54, 30: 72, 38: 99, 46: 144, 54: 180})
}

// foreverFurorCatEnergy: shifting into Cat Form restores 20% per Furor rank of the Energy the druid
// had when last in Cat Form, plus 2 Energy per rank for each second spent out of Bear/Cat Form,
// up to 20 Energy per rank.
func (druid *Druid) foreverFurorCatEnergy(sim *core.Simulation) float64 {
	r := float64(druid.ForeverTalents.Rank("Furor"))
	if r == 0 {
		return 0
	}
	away := max(0, (sim.CurrentTime - druid.foreverCatLeftAt).Seconds())
	return min(20*r, 0.2*r*druid.foreverCatLeftEnergy+2*r*away)
}

// Nature's Grace (Forever): non-periodic spell crits increase casting speed and reduce the
// global cooldown by 10% for 3 sec.
func (druid *Druid) applyForeverNaturesGrace() {
	druid.NaturesGraceProcAura = druid.RegisterAura(core.Aura{
		Label:     "Natures Grace Proc",
		ActionID:  core.ActionID{SpellID: 16886},
		Duration:  time.Second * 3,
		MaxStacks: 1,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.MultiplyCastSpeed(1.1)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.MultiplyCastSpeed(1 / 1.1)
		},
	}).AttachSpellMod(core.SpellModConfig{
		Kind:      core.SpellMod_GlobalCooldown_Flat,
		ClassMask: ClassSpellMask_DruidHarmfulGCDSpells,
		TimeValue: -core.GCDDefault / 10,
	})

	core.MakePermanent(druid.RegisterAura(core.Aura{
		Label: "Natures Grace",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			// Wrath and Starfire proc it themselves as the cast finishes (travel time).
			if spell.MissileSpeed == 0 && spell.ProcMask.Matches(core.ProcMaskSpellDamage) && result.DidCrit() {
				druid.NaturesGraceProcAura.Activate(sim)
				druid.NaturesGraceProcAura.SetStacks(sim, 1)
			}
		},
	}))
}

// Eclipse (Forever): Wrath reduces the cast time of the next 2 Starfires; up to 4 charges, 15 sec.
func (druid *Druid) applyForeverEclipse(reductionMs float64) {
	druid.ForeverEclipseAura = druid.RegisterAura(core.Aura{
		Label:     "Eclipse (Forever)",
		ActionID:  core.ActionID{SpellID: foreverEclipseSpellID},
		Duration:  time.Second * 15,
		MaxStacks: 4,
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell.Matches(ClassSpellMask_DruidStarfire) {
				aura.RemoveStack(sim)
			}
		},
	}).AttachSpellMod(core.SpellModConfig{
		Kind:      core.SpellMod_CastTime_Flat,
		ClassMask: ClassSpellMask_DruidStarfire,
		TimeValue: -time.Duration(reductionMs) * time.Millisecond,
	})

	core.MakePermanent(druid.RegisterAura(core.Aura{
		Label: "Eclipse Trigger (Forever)",
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell.Matches(ClassSpellMask_DruidWrath) {
				eclipse := druid.ForeverEclipseAura
				stacks := eclipse.GetStacks()
				eclipse.Activate(sim)
				eclipse.SetStacks(sim, min(eclipse.MaxStacks, stacks+2))
			}
		},
	}))
}

// Omen of Clarity (Forever): "Your spells and attacks have a chance to grant you Clearcasting."
// The engine's melee proc is kept; damaging spell casts proc it 6% of the time (estimate),
// doubled in Moonkin Form.
func (druid *Druid) applyForeverOmenOfClaritySpells() {
	core.MakePermanent(druid.RegisterAura(core.Aura{
		Label: "Omen of Clarity Spells (Forever)",
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if druid.ClearcastingAura == nil || !spell.ProcMask.Matches(core.ProcMaskSpellDamage) || !spell.Flags.Matches(SpellFlagOmen) {
				return
			}
			chance := core.TernaryFloat64(druid.InForm(Moonkin), 0.12, 0.06)
			if sim.Proc(chance, "Omen of Clarity (Spells)") {
				druid.ClearcastingAura.Activate(sim)
			}
		},
	}))
}

// Natural Reaction: +1% dodge per rank and a 20% chance per rank to gain 5 Rage on dodging.
func (druid *Druid) applyForeverNaturalReaction(r float64) {
	druid.AddStat(stats.Dodge, r*core.DodgeRatingPerDodgeChance)
	rageMetrics := druid.NewRageMetrics(core.ActionID{SpellID: 1290101})
	core.MakePermanent(druid.RegisterAura(core.Aura{
		Label: "Natural Reaction (Forever)",
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Outcome.Matches(core.OutcomeDodge) && druid.InForm(Bear) && sim.Proc(0.2*r, "Natural Reaction") {
				druid.AddRage(sim, 5, rageMetrics)
			}
		},
	}))
}

// Rend and Tear: the druid's melee abilities deal more damage to bleeding targets.
func (druid *Druid) applyForeverRendAndTear(multiplier float64) {
	abilities := ClassSpellMask_DruidShred | ClassSpellMask_DruidRake | ClassSpellMask_DruidRip | ClassSpellMask_DruidFerociousBite |
		ClassSpellMask_DruidMaul | ClassSpellMask_DruidSwipeBear | ClassSpellMask_DruidSwipeCat | ClassSpellMask_DruidLacerate |
		ClassSpellMask_DruidMangleBear | ClassSpellMask_DruidMangleCat
	isBleeding := func(target *core.Unit) bool {
		return druid.BleedsActive[target.UnitIndex] > 0 || druid.BleedCategories.Get(target).AnyActive() ||
			(druid.LacerateBleed != nil && druid.LacerateBleed.Dot(target).IsActive())
	}
	druid.RegisterAura(core.Aura{
		Label: "Rend and Tear (Forever)",
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			for _, ds := range druid.DruidSpells {
				if !ds.Matches(abilities) || ds.ApplyEffects == nil {
					continue
				}
				apply := ds.ApplyEffects
				ds.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
					if !isBleeding(target) {
						apply(sim, target, spell)
						return
					}
					old := spell.GetDamageMultiplier()
					spell.ApplyMultiplicativeDamageBonus(multiplier)
					apply(sim, target, spell)
					spell.SetMultiplicativeDamageBonus(old)
				}
			}
		},
	})
}

// registerForeverPrimalBite: Forever's bear ability (renamed Mangle (Bear), no debuff).
// "Bite the target, dealing 100% normal damage plus 77 and generating a high amount of threat."
func (druid *Druid) registerForeverPrimalBite() {
	flatDamage := foreverStepValue(druid.Level, map[int32]float64{0: 26, 40: 43, 50: 60, 60: 77})

	druid.ForeverPrimalBite = druid.RegisterSpell(Bear, core.SpellConfig{
		ClassSpellMask: ClassSpellMask_DruidMangleBear,
		ActionID:       core.ActionID{SpellID: foreverPrimalBiteSpellID},
		SpellSchool:    core.SpellSchoolPhysical,
		DefenseType:    core.DefenseTypeMelee,
		ProcMask:       core.ProcMaskMeleeMHSpecial,
		Flags:          SpellFlagOmen | core.SpellFlagMeleeMetrics | core.SpellFlagAPL,

		RageCost: core.RageCostOptions{
			Cost:   15,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1.5,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			berserking := druid.ForeverBerserkAura.IsActive()
			numHits := min(core.TernaryInt32(berserking, 3, 1), druid.Env.GetNumTargets())
			for i := int32(0); i < numHits; i++ {
				baseDamage := flatDamage + spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower())
				result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
				if i == 0 && !result.Landed() && !berserking {
					spell.IssueRefund(sim)
				}
				target = sim.Environment.NextTargetUnit(target)
			}
		},
	})
}

// registerForeverShiftingPower: "Instantly convert 55% of base Mana into 40 Energy." Its cost is
// reduced by Natural Shapeshifter. Cooldown is an estimate (Improved Shifting Power: -4/8 sec).
func (druid *Druid) registerForeverShiftingPower(cooldown time.Duration) {
	actionID := core.ActionID{SpellID: foreverShiftingPowerSpellID}
	energyMetrics := druid.NewEnergyMetrics(actionID)

	druid.ForeverShiftingPower = druid.RegisterSpell(Cat, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.55,
			Multiplier: 100 - 10*druid.Talents.NaturalShapeshifter,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: cooldown,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			druid.AddEnergy(sim, 40, energyMetrics)
		},
	})
}

// registerForeverBerserk: Primal Bite strikes up to 3 targets with no cooldown, and combo point
// generating abilities get +100% critical strike chance, for 15 sec. 3 min cooldown.
func (druid *Druid) registerForeverBerserk() {
	actionID := core.ActionID{SpellID: foreverBerserkSpellID}
	druid.ForeverBerserkAura = druid.RegisterAura(core.Aura{
		Label:    "Berserk",
		ActionID: actionID,
		Duration: time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if druid.ForeverPrimalBite != nil {
				druid.ForeverPrimalBite.CD.Reset()
			}
		},
	}).AttachSpellMod(core.SpellModConfig{
		Kind:      core.SpellMod_Cooldown_Multi_Flat,
		ClassMask: ClassSpellMask_DruidMangleBear,
		IntValue:  -100,
	}).AttachSpellMod(core.SpellModConfig{
		Kind:       core.SpellMod_BonusCrit_Flat,
		SpellFlags: SpellFlagBuilder,
		FloatValue: 100 * core.CritRatingPerCritChance,
	})

	druid.Berserk = druid.RegisterSpell(Cat|Bear, core.SpellConfig{
		ClassSpellMask: ClassSpellMask_DruidBerserk,
		ActionID:       actionID,
		Flags:          core.SpellFlagAPL,

		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Minute * 3,
			},
			IgnoreHaste: true,
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			druid.ForeverBerserkAura.Activate(sim)
		},
	})

	druid.AddMajorCooldown(core.MajorCooldown{
		Spell: druid.Berserk.Spell,
		Type:  core.CooldownTypeDPS,
		ShouldActivate: func(sim *core.Simulation, character *core.Character) bool {
			return druid.InForm(Cat | Bear)
		},
	})
}
