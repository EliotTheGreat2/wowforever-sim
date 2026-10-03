package warlock

import (
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever warlock talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/warlock.json).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents.
//
// Not modeled (no effect on a single-target DPS sim, or not enough data):
// Fel Concentration, Intensity, Pyroclasm, Destructive Reach, Molten Skin, Demonic Aegis,
// Improved Health Funnel, Improved Voidwalker, Improved Sayaad, Improved Felhunter,
// Demonic Energies, Demonic Brand, Soul Harvest, Curse of Exhaustion, Amplify Curse,
// Bane of Havoc (needs a second target),
// Decimation's "3/6% increased damage" clause (tooltip is ambiguous), Shadow and Flame's
// Soul Shard refund (shards aren't modeled).

const (
	foreverIncinerateSpellID = 412758 // Forever's Incinerate has no public spell ID yet; SoD's is used for name/icon
	foreverWrackSpellID      = 1290000
)

var foreverDestructionMask = ClassSpellMask_WarlockShadowBolt | ClassSpellMask_WarlockImmolate | ClassSpellMask_WarlockConflagrate |
	ClassSpellMask_WarlockSearingPain | ClassSpellMask_WarlockSoulFire | ClassSpellMask_WarlockShadowburn |
	ClassSpellMask_WarlockRainOfFire | ClassSpellMask_WarlockIncinerate

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from NewWarlock.
func (warlock *Warlock) useForeverTalentLayout() {
	ft := warlock.ForeverTalents
	if ft == nil {
		return
	}
	warlock.Talents = &proto.WarlockTalents{
		// Affliction
		ImprovedLifeTap:    ft.Rank("Improved Life Tap"),
		ImprovedCorruption: ft.Rank("Improved Corruption"),
		AmplifyCurse:       ft.Has("Amplify Curse"),
		Nightfall:          ft.Rank("Nightfall"),
		SiphonLife:         ft.Has("Siphon Life"),
		CurseOfExhaustion:  ft.Has("Curse of Exhaustion"),
		// Demonology
		ImprovedHealthFunnel: ft.Rank("Improved Health Funnel"),
		ImprovedImp:          ft.Rank("Improved Imp"),
		DemonicEmbrace:       ft.Rank("Demonic Embrace"),
		ImprovedVoidwalker:   ft.Rank("Improved Voidwalker"),
		ImprovedSayaad:       ft.Rank("Improved Sayaad"),
		FelDomination:        ft.Has("Fel Domination"),
		MasterSummoner:       ft.Rank("Master Summoner"),
		DemonicSacrifice:     ft.Has("Demonic Sacrifice"),
		SoulLink:             ft.Has("Soul Link"),
		// Destruction
		ImprovedShadowBolt: ft.Rank("Improved Shadow Bolt"),
		Bane:               ft.Rank("Bane"),
		DestructiveReach:   ft.Rank("Destructive Reach"),
		Shadowburn:         ft.Has("Shadowburn"),
		Intensity:          ft.Rank("Intensity"),
		Pyroclasm:          ft.Rank("Pyroclasm"),
		Conflagrate:        ft.Has("Conflagrate"),
		// Aftermath: +10% Immolate initial damage per rank (Classic Improved Immolate is 5% per rank).
		ImprovedImmolate: 2 * ft.Rank("Aftermath"),
	}
}

func (warlock *Warlock) applyForeverTalents() {
	ft := warlock.ForeverTalents
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

	// ---------------- Affliction ----------------
	if r := rank("Suppression"); r > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, ClassMask: ClassSpellMask_WarlockAll, FloatValue: r * core.SpellHitRatingPerHitChance})
	}
	if r := rank("Improved Corruption"); r > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_WarlockCorruption, IntValue: int64(2 * r)})
	}
	if r := rank("Malediction"); r > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PeriodicDamageDone_Flat, ClassMask: ClassSpellMask_WarlockAll, IntValue: int64(r)})
	}
	drains := ClassSpellMask_WarlockDrainLife | ClassSpellMask_WarlockDrainSoul | ClassSpellMask_WarlockWrack
	if v := byRank("Improved Drains", 7, 13, 20); v > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BaseDamageDone_Flat, ClassMask: drains, IntValue: int64(v)})
	}
	if r := rank("Improved Bane of Agony"); r > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BaseDamageDone_Flat, ClassMask: ClassSpellMask_WarlockCurseOfAgony, IntValue: int64(5 * r)})
	}
	if v := byRank("Pandemic", 0.33, 0.67, 1); v > 0 {
		warlock.AddStaticMod(core.SpellModConfig{
			Kind: core.SpellMod_CritDamageBonus_Flat,
			ClassMask: ClassSpellMask_WarlockCorruption | ClassSpellMask_WarlockCurseOfAgony | ClassSpellMask_WarlockCurseOfDoom |
				ClassSpellMask_WarlockSiphonLife | drains,
			FloatValue: v,
		})
	}
	if r := rank("Malevolence"); r > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, School: core.SpellSchoolShadow, ClassMask: ClassSpellMask_WarlockAll, FloatValue: r * core.SpellCritRatingPerCritChance})
	}
	if r := rank("Soul Siphon"); r > 0 {
		warlock.foreverSoulSiphonPerEffect = 0.04 * r
		warlock.foreverSoulSiphonMax = 0.12 * r
	}
	if r := rank("Shadow Mastery"); r > 0 {
		// 1% per rank in Forever (Classic: 2%). Same split as Classic: some spells mod base damage.
		baseMod := ClassSpellMask_WarlockCurseOfAgony | ClassSpellMask_WarlockDeathCoil | drains
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, School: core.SpellSchoolShadow, ClassMask: ClassSpellMask_WarlockAll ^ baseMod, IntValue: int64(r)})
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BaseDamageDone_Flat, School: core.SpellSchoolShadow, ClassMask: baseMod, IntValue: int64(r)})
	}
	if ft.Has("Wrack") {
		warlock.registerForeverWrack()
	}

	// ---------------- Demonology ----------------
	if r := rank("Unholy Power"); r > 0 {
		for _, pet := range warlock.BasePets {
			pet.PseudoStats.DamageDealtMultiplier *= 1 + 0.02*r
		}
	}
	if r := rank("Fel Vitality"); r > 0 {
		warlock.MultiplyStat(stats.Mana, 1+0.05*r)
		for _, pet := range warlock.BasePets {
			pet.MultiplyStat(stats.Health, 1+0.05*r)
			pet.MultiplyStat(stats.Mana, 1+0.05*r)
		}
	}
	if r := rank("Master Demonologist"); r > 0 {
		warlock.applyForeverMasterDemonologist(0.02 * r)
	}
	if v := byRank("Demonic Knowledge", 1.0/3, 2.0/3, 1); v > 0 {
		warlock.applyForeverDemonicKnowledge(v * float64(warlock.Level))
	}
	if ft.Has("Demonic Pact") {
		// Summoning a different demon no longer cancels the Demonic Sacrifice effect
		// (re-summoning the sacrificed demon still does). Forever's Master Demonologist is
		// implemented in applyForeverMasterDemonologist, so this flag only affects Demonic Sacrifice.
		warlock.maintainBuffsOnSacrifice = true
	}
	if r := rank("Decimation"); r > 0 {
		warlock.applyForeverDecimation(r)
	}

	// ---------------- Destruction ----------------
	if v := byRank("Cataclysm", 3, 6, 10); v > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Pct, ClassMask: foreverDestructionMask, IntValue: -int64(v)})
	}
	if r := rank("Ruin"); r > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_CritDamageBonus_Flat, ClassMask: foreverDestructionMask, FloatValue: 0.2 * r})
	}
	if v := byRank("Agonizing Flames", 3, 7, 10); v > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_WarlockSearingPain, FloatValue: v * core.SpellCritRatingPerCritChance})
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: foreverDestructionMask, IntValue: int64(v)})
	}
	if v := byRank("Fire and Brimstone", 8, 17, 25); v > 0 {
		warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusCrit_Flat, ClassMask: ClassSpellMask_WarlockConflagrate, FloatValue: v * core.SpellCritRatingPerCritChance})
	}
	if r := rank("Shadow and Flame"); r > 0 {
		warlock.applyForeverShadowAndFlame(r)
	}
	if ft.Has("Incinerate") {
		warlock.registerForeverIncinerate()
	}
}

// foreverDemonicSacrificeSwap: Forever's Demonic Sacrifice grants the opposing aspect
// (Imp: Shadow damage, Succubus/Incubus: Fire damage, Voidwalker: Mana, Felhunter: Health).
func (warlock *Warlock) foreverDemonicSacrificeSwap() bool {
	return warlock.ForeverTalents != nil
}

// foreverNightfallProcMask: Forever's Nightfall also procs from Drain Soul and Wrack.
func (warlock *Warlock) foreverNightfallProcMask() uint64 {
	if warlock.ForeverTalents == nil {
		return 0
	}
	return ClassSpellMask_WarlockDrainSoul | ClassSpellMask_WarlockWrack
}

// calcForeverSoulSiphon: Drain Life, Drain Soul and Wrack deal more damage per other
// Affliction effect of the warlock's on the target.
func (warlock *Warlock) calcForeverSoulSiphon(target *core.Unit) float64 {
	if warlock.foreverSoulSiphonPerEffect == 0 {
		return 1
	}
	return 1 + min(warlock.foreverSoulSiphonMax, warlock.foreverSoulSiphonPerEffect*float64(warlock.activeEffects[target.UnitIndex]))
}

func (warlock *Warlock) applyForeverMasterDemonologist(bonus float64) {
	schoolFor := func(pet *WarlockPet) (stats.SchoolIndex, bool) {
		switch pet {
		case warlock.Imp:
			return stats.SchoolIndexFire, true
		case warlock.Succubus:
			return stats.SchoolIndexShadow, true
		}
		return 0, false
	}
	for _, pet := range warlock.BasePets {
		pet := pet
		school, ok := schoolFor(pet)
		if !ok {
			continue // Voidwalker/Felhunter versions reduce damage taken
		}
		cfg := func(unit *core.Unit) core.Aura {
			return core.Aura{
				Label:    "Master Demonologist (" + pet.Name + ")",
				ActionID: core.ActionID{SpellID: 23825, Tag: int32(school)},
				Duration: core.NeverExpires,
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[school] *= 1 + bonus
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[school] /= 1 + bonus
				},
			}
		}
		ownerAura := warlock.RegisterAura(cfg(&warlock.Unit))
		petAura := pet.RegisterAura(cfg(&pet.Unit))

		oldOnPetEnable := pet.OnPetEnable
		pet.OnPetEnable = func(sim *core.Simulation) {
			oldOnPetEnable(sim)
			ownerAura.Activate(sim)
			petAura.Activate(sim)
		}
		oldOnPetDisable := pet.OnPetDisable
		pet.OnPetDisable = func(sim *core.Simulation, isSacrifice bool) {
			oldOnPetDisable(sim, isSacrifice)
			ownerAura.Deactivate(sim)
			petAura.Deactivate(sim)
		}
	}
}

func (warlock *Warlock) applyForeverDemonicKnowledge(spellPower float64) {
	aura := warlock.RegisterAura(core.Aura{
		Label:    "Demonic Knowledge (Forever)",
		ActionID: core.ActionID{SpellID: int32(proto.WarlockRune_RuneBootsDemonicKnowledge)},
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			warlock.AddStatDynamic(sim, stats.SpellPower, spellPower)
			if warlock.ActivePet != nil {
				warlock.ActivePet.AddStatDynamic(sim, stats.SpellPower, spellPower)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			warlock.AddStatDynamic(sim, stats.SpellPower, -spellPower)
		},
	})
	for _, pet := range warlock.BasePets {
		pet := pet
		oldOnPetEnable := pet.OnPetEnable
		pet.OnPetEnable = func(sim *core.Simulation) {
			oldOnPetEnable(sim)
			aura.Activate(sim)
		}
		oldOnPetDisable := pet.OnPetDisable
		pet.OnPetDisable = func(sim *core.Simulation, isSacrifice bool) {
			oldOnPetDisable(sim, isSacrifice)
			if aura.IsActive() {
				pet.AddStatDynamic(sim, stats.SpellPower, -spellPower)
			}
			aura.Deactivate(sim)
		}
	}
}

func (warlock *Warlock) applyForeverDecimation(points float64) {
	warlock.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Cooldown_Multi_Flat, ClassMask: ClassSpellMask_WarlockSoulFire, IntValue: -int64(45 * points)})

	decimation := warlock.RegisterAura(core.Aura{
		Label:    "Decimation (Forever)",
		ActionID: core.ActionID{SpellID: 440873},
		Duration: time.Second * 10,
	}).AttachSpellMod(core.SpellModConfig{Kind: core.SpellMod_CastTime_Pct, ClassMask: ClassSpellMask_WarlockSoulFire, FloatValue: -0.2 * points})

	core.MakePermanent(warlock.RegisterAura(core.Aura{
		Label: "Decimation Trigger (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && sim.IsExecutePhase35() && spell.Matches(ClassSpellMask_WarlockShadowBolt|ClassSpellMask_WarlockSearingPain) {
				decimation.Activate(sim)
			}
		},
	}))
}

func (warlock *Warlock) applyForeverShadowAndFlame(points float64) {
	bonus := 1 + 0.02*points
	schoolAura := func(label string, spellID int32, school stats.SchoolIndex) *core.Aura {
		return warlock.RegisterAura(core.Aura{
			Label:    label,
			ActionID: core.ActionID{SpellID: spellID},
			Duration: time.Second * 20,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				warlock.PseudoStats.SchoolDamageDealtMultiplier[school] *= bonus
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				warlock.PseudoStats.SchoolDamageDealtMultiplier[school] /= bonus
			},
		})
	}
	shadowAura := schoolAura("Shadow and Flame (Shadow)", 426316, stats.SchoolIndexShadow)
	fireAura := schoolAura("Shadow and Flame (Fire)", 426317, stats.SchoolIndexFire)
	warlock.foreverConflagKeepsImmolateChance = 0.2 * points

	core.MakePermanent(warlock.RegisterAura(core.Aura{
		Label: "Shadow and Flame Trigger (Forever)",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			if spell.Matches(ClassSpellMask_WarlockConflagrate) {
				shadowAura.Activate(sim)
			} else if spell.Matches(ClassSpellMask_WarlockShadowburn) {
				fireAura.Activate(sim)
			}
		},
	}))
}

// foreverRankValue returns the value of the highest rank learned at the warlock's level,
// from {learn level: value} (the lowest rank if none is learned yet).
func (warlock *Warlock) foreverRankValue(ranks map[int32]float64) float64 {
	best, bestLevel := 0.0, int32(-1)
	lowest, lowestLevel := 0.0, int32(1000)
	for level, v := range ranks {
		if level <= warlock.Level && level > bestLevel {
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

// registerForeverIncinerate: Forever's Incinerate (Destruction capstone).
// Ranks 1-3 learned at 40/50/60; rank 3 deals 201-233 Fire, rank 1 99-115 (beta tooltips).
// +25% damage if the target has Immolate. 2.5 sec cast, 0.714 coefficient.
func (warlock *Warlock) registerForeverIncinerate() {
	low := warlock.foreverRankValue(map[int32]float64{40: 99, 50: 150, 60: 201})
	high := warlock.foreverRankValue(map[int32]float64{40: 115, 50: 174, 60: 233})
	mana := warlock.foreverRankValue(map[int32]float64{40: 205, 50: 255, 60: 305})

	warlock.Incinerate = warlock.RegisterSpell(core.SpellConfig{
		ClassSpellMask: ClassSpellMask_WarlockIncinerate,
		ActionID:       core.ActionID{SpellID: foreverIncinerateSpellID},
		SpellSchool:    core.SpellSchoolFire,
		DefenseType:    core.DefenseTypeMagic,
		ProcMask:       core.ProcMaskSpellDamage,
		Flags:          core.SpellFlagAPL | core.SpellFlagResetAttackSwing | core.SpellFlagBinary | WarlockFlagDestruction,
		MissileSpeed:   24,

		ManaCost: core.ManaCostOptions{FlatCost: mana},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 2500,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 0.714,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcDamage(sim, target, sim.Roll(low, high), spell.OutcomeMagicHitAndCrit)
			if warlock.getActiveImmolateSpell(target) != nil {
				result.Damage *= 1.25
			}
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}

// registerForeverWrack: Affliction capstone channel. 36 Shadow damage every 1 sec for 6 sec
// (beta tooltip, single rank), 0.143 coefficient per tick, and the target takes 10% more
// damage from the warlock's other Shadow damage-over-time effects while channeled.
// Mana cost is not published; Drain Life's max-rank cost is used.
func (warlock *Warlock) registerForeverWrack() {
	dotSpells := ClassSpellMask_WarlockCorruption | ClassSpellMask_WarlockCurseOfAgony | ClassSpellMask_WarlockCurseOfDoom |
		ClassSpellMask_WarlockSiphonLife | ClassSpellMask_WarlockDrainLife | ClassSpellMask_WarlockDrainSoul
	debuffs := warlock.NewEnemyAuraArray(func(target *core.Unit, _ int32) *core.Aura {
		return target.RegisterAura(core.Aura{
			Label:    "Wrack-" + warlock.Label,
			ActionID: core.ActionID{SpellID: foreverWrackSpellID, Tag: 1},
			Duration: time.Second * 6,
		})
	})
	// The extra 10% on other Shadow DoT ticks is dealt as its own line so it shows in the breakdown.
	amplify := warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: foreverWrackSpellID, Tag: 2},
		SpellSchool: core.SpellSchoolShadow,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagIgnoreModifiers | core.SpellFlagNoOnCastComplete | core.SpellFlagPassiveSpell,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
	})
	core.MakePermanent(warlock.RegisterAura(core.Aura{
		Label: "Wrack Amplify (Forever)",
		OnPeriodicDamageDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Damage > 0 && spell.Matches(dotSpells) && spell.SpellSchool == core.SpellSchoolShadow && debuffs.Get(result.Target).IsActive() {
				amplify.CalcAndDealDamage(sim, result.Target, result.Damage*0.10, amplify.OutcomeAlwaysHit)
			}
		},
	}))

	warlock.foreverWrack = warlock.RegisterSpell(core.SpellConfig{
		ClassSpellMask: ClassSpellMask_WarlockWrack,
		ActionID:       core.ActionID{SpellID: foreverWrackSpellID},
		SpellSchool:    core.SpellSchoolShadow,
		DefenseType:    core.DefenseTypeMagic,
		ProcMask:       core.ProcMaskSpellDamage,
		Flags:          core.SpellFlagAPL | core.SpellFlagChanneled | core.SpellFlagResetAttackSwing | WarlockFlagAffliction,

		ManaCost: core.ManaCostOptions{FlatCost: 300},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{GCD: core.GCDDefault},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Wrack-" + warlock.Label,
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					debuffs.Get(aura.Unit).Activate(sim)
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					debuffs.Get(aura.Unit).Deactivate(sim)
				},
			},
			NumberOfTicks:    6,
			TickLength:       time.Second,
			BonusCoefficient: 0.143,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, 36, isRollover)
				dot.SnapshotAttackerMultiplier *= warlock.calcForeverSoulSiphon(target)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, core.Ternary(warlock.ForeverCombatRules, dot.OutcomeSnapshotCrit, dot.OutcomeTick))
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			}
			spell.DealOutcome(sim, result)
		},
	})
}
