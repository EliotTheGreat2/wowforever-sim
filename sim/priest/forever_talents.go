package priest

import (
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever priest talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/priest.json).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields;
// changed and new talents are applied in applyForeverTalents.
//
// Not modeled (healing, defensive, utility or PvP; no effect on a single-target shadow DPS sim):
// Wand Specialization (wands are not in the rotations), Improved Power Word: Shield, Martyrdom,
// Improved Inner Fire, Soul Warding, Improved Mana Burn, Renewed Hope, Divine Aegis,
// Twilight Focus, Improved Renew, Holy Nova (no engine spell), Blessed Recovery, Inspiration,
// Holy Reach, Improved Healing, Binding Heal, Litany of Light, Spirit of Redemption,
// Spiritual Healing, Prayer of Mending, Blackout, Shadow Reach, Improved Psychic Scream,
// Improved Fade, Silence, Searing Light's free Holy Nova proc, Devouring Contagion's spread
// (needs a dying second target), Improved Mind Flay's range/slow change,
// Shadowform's 15% Physical damage reduction (only matters for a tank sim),
// Power in Light (Smite/Penance vs Holy Fire targets; no simmed spec casts both).
// Vampiric Embrace is mapped (the Forever version heals 20% for 30 sec; healing is not simmed).

const (
	// Forever's Shadow Word: Death has no public spell ID yet; TBC's rank 1 ID is used for name/icon.
	foreverShadowWordDeathSpellID = 32379
	foreverShadowWeavingSpellID   = 15334
)

// Instant-cast damage spells (Twin Disciplines, Mental Agility). Mind Flay is a channel.
const foreverInstantMask = ClassSpellMask_PriestShadowWordPain | ClassSpellMask_PriestDevouringPlague |
	ClassSpellMask_PriestShadowWordDeath | ClassSpellMask_PriestVampiricEmbrace

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from New.
func (priest *Priest) useForeverTalentLayout() {
	ft := priest.ForeverTalents
	if ft == nil {
		return
	}
	priest.Talents = &proto.PriestTalents{
		// Discipline
		InnerFocus: ft.Has("Inner Focus"),
		Meditation: ft.Rank("Meditation"),
		// Holy
		HolySpecialization: ft.Rank("Holy Specialization"),
		SpellWarding:       ft.Rank("Spell Warding"),
		DivineFury:         ft.Rank("Divine Fury"),
		// Shadow
		SpiritTap:              ft.Rank("Spirit Tap"),
		ImprovedShadowWordPain: ft.Rank("Improved Shadow Word: Pain"),
		ImprovedMindBlast:      ft.Rank("Improved Mind Blast"),
		MindFlay:               ft.Has("Mind Flay"),
		VampiricEmbrace:        ft.Has("Vampiric Embrace"),
		Darkness:               ft.Rank("Darkness"),
	}
	// Forever's Penance talent is SoD's Penance rune with Forever numbers (see foreverPenanceDamage).
	if ft.Has("Penance") {
		priest.GrantForeverRune(int32(proto.PriestRune_RuneHandsPenance))
	}
}

// dotsCanCrit: periodic damage can crit under Forever's combat rules (and with SoD's Despair rune).
func (priest *Priest) dotsCanCrit() bool {
	return priest.ForeverCombatRules || priest.HasRune(proto.PriestRune_RuneBracersDespair)
}

func (priest *Priest) applyForeverTalents() {
	ft := priest.ForeverTalents
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

	// ---------------- Discipline ----------------
	if r := rank("Twin Disciplines"); r > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: foreverInstantMask, IntValue: int64(r)})
	}
	if v := byRank("Silent Resolve", 0.10, 0.20, 0.30); v > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Threat_Flat, ClassMask: ClassSpellMask_PriestAll, School: core.SpellSchoolHoly, FloatValue: -v})
	}
	if v := byRank("Holy Precision", 6, 12, 18); v > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, ClassMask: ClassSpellMask_PriestAll, School: core.SpellSchoolHoly, FloatValue: v * core.SpellHitRatingPerHitChance})
	}
	if v := byRank("Mental Agility", 3, 7, 10); v > 0 {
		priest.AddStaticMod(core.SpellModConfig{
			Kind:      core.SpellMod_PowerCost_Pct,
			ClassMask: ClassSpellMask_PriestSmite | ClassSpellMask_PriestHolyFire | foreverInstantMask | ClassSpellMask_PriestInnerFocus,
			IntValue:  -int64(v),
		})
	}
	if r := rank("Mental Strength"); r > 0 {
		priest.MultiplyStat(stats.Intellect, 1+0.03*r)
	}
	if ft.Has("Power Infusion") {
		priest.registerForeverPowerInfusion()
	}

	// ---------------- Holy ----------------
	if v := byRank("Searing Light", 2, 5); v > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_PriestAll, School: core.SpellSchoolHoly, IntValue: int64(v)})
	}
	if v := byRank("Spiritual Guidance", 0.01, 0.03, 0.05, 0.06, 0.08); v > 0 {
		priest.AddStatDependency(stats.Spirit, stats.SpellPower, v)
	}

	// ---------------- Shadow Magic ----------------
	if r := rank("Shadow Focus"); r > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_BonusHit_Flat, ClassMask: ClassSpellMask_PriestAll, School: core.SpellSchoolShadow, FloatValue: r * core.SpellHitRatingPerHitChance})
	}
	if v := byRank("Shadow Affinity", 0.10, 0.20, 0.30); v > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Threat_Flat, ClassMask: ClassSpellMask_PriestAll, School: core.SpellSchoolShadow, FloatValue: -v})
	}
	if r := rank("Improved Mind Flay"); r > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_PriestMindFlay, IntValue: int64(10 * r)})
	}
	if r := rank("Shadow Weaving"); r > 0 {
		priest.registerForeverShadowWeaving([]float64{0.33, 0.67, 1}[int(r)-1])
	}
	if v := byRank("Devouring Contagion", 25, 50); v > 0 {
		priest.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Pct, ClassMask: ClassSpellMask_PriestDevouringPlague, IntValue: -int64(v)})
	}
	if v := byRank("Early Demise", 15, 30); v > 0 {
		priest.foreverEarlyDemiseCrit = v * core.SpellCritRatingPerCritChance
	}
	if ft.Has("Shadowform") {
		priest.registerForeverShadowform()
	}
}

// foreverRankValue returns the value of the highest rank learned at the priest's level,
// from {learn level: value} (the lowest rank if none is learned yet).
func (priest *Priest) foreverRankValue(ranks map[int32]float64) float64 {
	best, bestLevel := 0.0, int32(-1)
	lowest, lowestLevel := 0.0, int32(1000)
	for level, v := range ranks {
		if level <= priest.Level && level > bestLevel {
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

// foreverPenanceDamage: damage per Penance bolt. Beta tooltips: rank 1 (talent tooltip) 81,
// rank 4 (level 60) 131. Ranks 2-3 and the learn levels (30/40/50/60) are estimates.
func (priest *Priest) foreverPenanceDamage() float64 {
	return priest.foreverRankValue(map[int32]float64{30: 81, 40: 98, 50: 114, 60: 131})
}

// registerForeverShadowWeaving: Forever's Shadow Weaving is a self buff: each Shadow damage
// spell has a 33/67/100% chance to give +2% Shadow damage for 15 sec, stacking 5 times.
// Uses the existing AddShadowWeavingStack call sites (Mind Blast, Mind Flay, SW:Pain, SW:Death...).
func (priest *Priest) registerForeverShadowWeaving(procChance float64) {
	aura := priest.RegisterAura(core.Aura{
		Label:     "Shadow Weaving (Forever)",
		ActionID:  core.ActionID{SpellID: foreverShadowWeavingSpellID},
		Duration:  time.Second * 15,
		MaxStacks: 5,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= 1 + 0.02*float64(oldStacks)
			aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1 + 0.02*float64(newStacks)
		},
	})

	priest.ShadowWeavingProc = priest.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: foreverShadowWeavingSpellID, Tag: 1},
		Flags:       core.SpellFlagNoOnCastComplete | core.SpellFlagNoMetrics | core.SpellFlagPassiveSpell,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			if procChance >= 1 || sim.RollWithLabel(0, 1, "ShadowWeaving") < procChance {
				aura.Activate(sim)
				aura.AddStack(sim)
			}
		},
	})
}

// registerForeverShadowform: +10% Shadow damage, -50% mana cost of Shadow spells and +100%
// critical strike damage bonus for Shadow spells (crits deal 200%).
func (priest *Priest) registerForeverShadowform() {
	actionID := core.ActionID{SpellID: 15473}

	priest.ShadowformAura = priest.RegisterAura(core.Aura{
		Label:    "Shadowform",
		ActionID: actionID,
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1.10
			aura.Unit.PseudoStats.SchoolCostMultiplier[stats.SchoolIndexShadow] -= 50
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= 1.10
			aura.Unit.PseudoStats.SchoolCostMultiplier[stats.SchoolIndexShadow] += 50
		},
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			if spell.SpellSchool.Matches(core.SpellSchoolHoly) && spell.Matches(ClassSpellMask_PriestAll) {
				aura.Deactivate(sim)
			}
		},
	}).AttachSpellMod(core.SpellModConfig{
		Kind:       core.SpellMod_CritDamageBonus_Flat,
		ClassMask:  ClassSpellMask_PriestAll,
		School:     core.SpellSchoolShadow,
		FloatValue: 1,
	})

	priest.Shadowform = priest.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL,

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			priest.ShadowformAura.Activate(sim)
		},
	})
}

// registerForeverPowerInfusion: self-cast Power Infusion, +20% spell damage for 15 sec.
// Cooldown and mana are not in the beta tooltip; Classic's 3 min / 16% base mana are used.
func (priest *Priest) registerForeverPowerInfusion() {
	actionID := core.ActionID{SpellID: 10060}
	aura := core.PowerInfusionAura(&priest.Unit, priest.Index)

	spell := priest.RegisterSpell(core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL | core.SpellFlagHelpful,

		ManaCost: core.ManaCostOptions{BaseCost: 0.16},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{GCD: core.GCDDefault},
			CD: core.Cooldown{
				Timer:    priest.NewTimer(),
				Duration: core.PowerInfusionCD,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			aura.Activate(sim)
		},
	})

	priest.AddMajorCooldown(core.MajorCooldown{
		Spell: spell,
		Type:  core.CooldownTypeDPS,
	})
}

// registerForeverShadowWordDeath: Forever spellbook spell (all priests, 4 ranks from level 32).
// Beta tooltip: rank 4 (level 56) 444-472 Shadow damage; the target-not-killed backlash
// (10% of max health to the priest) is not modeled. Ranks 1-3 damage, learn levels
// (32/40/48/56), mana costs, the 12 sec cooldown and the 0.429 coefficient are estimates
// (TBC values / Mind Blast scaling); the beta tooltip doesn't list them.
// Early Demise adds crit chance on targets at or below 20% health.
func (priest *Priest) registerForeverShadowWordDeath() {
	if priest.Level < 32 {
		return
	}
	low := priest.foreverRankValue(map[int32]float64{32: 250, 40: 310, 48: 375, 56: 444})
	high := priest.foreverRankValue(map[int32]float64{32: 266, 40: 330, 48: 399, 56: 472})
	mana := priest.foreverRankValue(map[int32]float64{32: 180, 40: 230, 48: 280, 56: 330})

	priest.ShadowWordDeath = priest.RegisterSpell(core.SpellConfig{
		ActionID:       core.ActionID{SpellID: foreverShadowWordDeathSpellID},
		ClassSpellMask: ClassSpellMask_PriestShadowWordDeath,
		SpellSchool:    core.SpellSchoolShadow,
		DefenseType:    core.DefenseTypeMagic,
		ProcMask:       core.ProcMaskSpellDamage,
		Flags:          core.SpellFlagBinary | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{FlatCost: mana},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{GCD: core.GCDDefault},
			CD: core.Cooldown{
				Timer:    priest.NewTimer(),
				Duration: time.Second * 12,
			},
		},

		BonusCoefficient: 0.429,
		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bonusCrit := core.TernaryFloat64(sim.IsExecutePhase20(), priest.foreverEarlyDemiseCrit, 0)
			spell.BonusCritRating += bonusCrit
			result := spell.CalcDamage(sim, target, sim.Roll(low, high), spell.OutcomeMagicHitAndCrit)
			spell.BonusCritRating -= bonusCrit
			if result.Landed() {
				priest.AddShadowWeavingStack(sim, target)
			}
			spell.DealDamage(sim, result)
		},
		ExpectedInitialDamage: func(sim *core.Simulation, target *core.Unit, spell *core.Spell, _ bool) *core.SpellResult {
			return spell.CalcDamage(sim, target, (low+high)/2, spell.OutcomeExpectedMagicHitAndCrit)
		},
	})
}
