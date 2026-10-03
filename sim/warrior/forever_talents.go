package warrior

import (
	"math"
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever warrior talents (beta build in core.ForeverTalentsBuild,
// assets/db_inputs/forever/talents/warrior.json).
//
// Talents that work as in Classic are mapped onto the engine's Classic talent fields
// (useForeverTalentLayout); changed and new talents are applied in applyForeverTalents.
// Forever spellbook changes (foreverchanges.pro) live in the spell files, gated on
// warrior.Forever: Battle Shout (139 AP at max rank, 3 min), Slam (max rank + 68),
// Shield Block (7 sec, 2 blocks), Shield Slam (640-670 + block value), Revenge (no AP
// scaling; base damage via forever_spell_tuning.csv).
//
// Not modeled (no effect on a single-target DPS/threat sim, or not enough data):
// Improved Charge, Improved Hamstring, Booming Voice (radius only in Forever), Iron Will,
// Piercing Howl, Blood Craze, Improved Intercept, Improved Disarm, Vanguard,
// Improved Shield Wall (cooldown longer than the fight), Improved Shield Bash,
// Concussion Blow (stun only), Mortal Strike's and Bloodthirst's non-damage riders
// (healing reduction, movement speed), Anger Management's out-of-combat clause,
// Improved Berserker Rage's snare removal. Spellbook changes not modeled: Victory Rush (needs
// a kill), Demoralizing Shout (204 AP for 45 sec), Thunder Clap's 20% slow, Shield Wall and
// Retaliation cooldowns (15 min, longer than the fight), Taunt's cooldown.
//
// Estimates (no published data):
//   - Spearing Strike: 10 Rage, 6 sec cooldown, on the GCD, normalized main-hand weapon damage.
//   - Forever's Slam cooldown (implied by Improved Slam): 6 sec base.
//   - Improved Tactical Mastery: no baseline retained Rage assumed, so 3 Rage per rank.
//   - Weaponmaster (Mace/Staff): "ignore X% armor" is applied as armor penetration equal to X%
//     of the target's armor when the fight starts, and only when the main hand is a mace or staff.
//   - Battle Shout below max rank: Classic AP scaled by Forever's max-rank ratio (139/232).

const (
	foreverSpearingStrikeSpellID = 1290101 // no public spell ID yet
	foreverBloodthrillSpellID    = 1290102 // no public spell ID yet
	foreverSlamCooldown          = time.Second * 6
)

// foreverBracket returns the highest of the given levels at or below level (the lowest if
// none is), so spell rank tables keyed by Classic brackets work at any WoW Forever level.
func foreverBracket(level int32, brackets ...int32) int32 {
	best := brackets[0]
	for _, b := range brackets {
		if b <= level {
			best = b
		}
	}
	return best
}

// useForeverTalentLayout replaces the Classic talent proto with the talents a Forever talent
// string maps onto. Called from NewWarrior.
func (warrior *Warrior) useForeverTalentLayout() {
	ft := warrior.ForeverTalents
	if ft == nil {
		return
	}
	weaponmaster := ft.Rank("Weaponmaster")
	warrior.Talents = &proto.WarriorTalents{
		// Arms
		ImprovedHeroicStrike:          ft.Rank("Improved Heroic Strike"),
		Deflection:                    ft.Rank("Deflection"),
		ImprovedOverpower:             ft.Rank("Improved Overpower"),
		AngerManagement:               ft.Has("Anger Management"),
		DeepWounds:                    ft.Rank("Deep Wounds"),
		TwoHandedWeaponSpecialization: ft.Rank("Two-Handed Weapon Specialization"),
		Impale:                        ft.Rank("Impale"),
		SweepingStrikes:               ft.Has("Sweeping Strikes"),
		MortalStrike:                  ft.Has("Mortal Strike"),
		// Weaponmaster: Axe/Polearm crit and Sword extra attacks work like Classic's
		// Axe/Polearm/Sword Specialization (1% per rank); Mace/Staff is applied below.
		AxeSpecialization:     weaponmaster,
		PolearmSpecialization: weaponmaster,
		SwordSpecialization:   weaponmaster,
		// Fury
		Cruelty:                 ft.Rank("Cruelty"),
		DualWieldSpecialization: ft.Rank("Dual Wield Specialization"),
		ImprovedExecute:         ft.Rank("Improved Execute"),
		DeathWish:               ft.Has("Death Wish"),
		ImprovedBerserkerRage:   ft.Rank("Improved Berserker Rage"),
		Flurry:                  ft.Rank("Flurry"),
		Bloodthirst:             ft.Has("Bloodthirst"),
		// Protection
		// Anticipation: +4 Defense per rank in Forever (Classic: +2).
		Anticipation:        2 * ft.Rank("Anticipation"),
		ImprovedBloodrage:   ft.Rank("Improved Bloodrage"),
		Toughness:           ft.Rank("Toughness"),
		LastStand:           ft.Has("Last Stand"),
		ImprovedSunderArmor: ft.Rank("Improved Sunder Armor"),
		ConcussionBlow:      ft.Has("Concussion Blow"),
		ShieldSlam:          ft.Has("Shield Slam"),
	}
}

func (warrior *Warrior) applyForeverTalents() {
	ft := warrior.ForeverTalents
	if ft == nil {
		return
	}
	rank := func(name string) float64 { return float64(ft.Rank(name)) }

	// ---------------- Arms ----------------
	if v := []int64{0, 12, 23, 35}[ft.Rank("Improved Rend")]; v > 0 {
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_WarriorRend, IntValue: v})
	}
	warrior.foreverExtraRetainedRage = 3 * rank("Improved Tactical Mastery")
	if ft.Has("Spearing Strike") {
		warrior.registerForeverSpearingStrike()
	}
	if r := rank("Bloodthrill"); r > 0 {
		warrior.applyForeverBloodthrill(0.04 * r)
	}
	if r := rank("Weaponmaster"); r > 0 {
		warrior.applyForeverWeaponmasterMace(0.03 * r)
	}
	if r := ft.Rank("Improved Slam"); r > 0 {
		warrior.foreverSlamKeepsSwing = true
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_CastTime_Flat, ClassMask: ClassSpellMask_WarriorSlam, TimeValue: -250 * time.Millisecond * time.Duration(r)})
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_GlobalCooldown_Flat, ClassMask: ClassSpellMask_WarriorSlam, TimeValue: -250 * time.Millisecond * time.Duration(r)})
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_Cooldown_Flat, ClassMask: ClassSpellMask_WarriorSlam, TimeValue: -1500 * time.Millisecond * time.Duration(r)})
	}

	// ---------------- Fury ----------------
	if r := rank("Unbridled Wrath"); r > 0 {
		warrior.applyForeverUnbridledWrath(0.12 * r)
	}
	cleaveCost := ft.Rank("Improved Cleave")
	if ft.Has("Raging Blows") {
		warrior.foreverRagingBlows = true
		cleaveCost += 2
	}
	if cleaveCost > 0 {
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Flat, ClassMask: ClassSpellMask_WarriorCleave, IntValue: -int64(cleaveCost)})
	}
	if r := rank("Boundless Rage"); r > 0 {
		warrior.AddMaxRage(10 * r)
	}
	if r := rank("Dual Wield Specialization"); r > 0 {
		// The off-hand damage part is mapped; Forever adds +2% off-hand hit chance per rank.
		bonusHit := 2 * r * core.MeleeHitRatingPerHitChance
		warrior.OnSpellRegistered(func(spell *core.Spell) {
			if spell.ProcMask.Matches(core.ProcMaskMeleeOH) {
				spell.BonusHitRating += bonusHit
			}
		})
	}
	if r := rank("Enrage"); r > 0 {
		warrior.applyForeverEnrage(0.02 * r)
	}
	if ft.Rank("Improved Execute") == 1 {
		// 3 Rage at rank 1 (Classic: 2); rank 2 is 5 in both.
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Flat, ClassMask: ClassSpellMask_WarriorExecute, IntValue: -1})
	}
	if r := rank("Precision"); r > 0 {
		warrior.AddStat(stats.MeleeHit, r*core.MeleeHitRatingPerHitChance)
	}

	// ---------------- Protection ----------------
	if r := rank("Shield Specialization"); r > 0 {
		warrior.applyForeverShieldSpecialization(r)
	}
	if v := []int64{0, 2, 4, 6}[ft.Rank("Improved Thunder Clap")]; v > 0 {
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_PowerCost_Flat, ClassMask: ClassSpellMask_WarriorThunderClap, IntValue: -v})
	}
	if r := rank("Master of Defense"); r > 0 {
		warrior.applyForeverMasterOfDefense(0.5 * r)
	}
	if r := rank("Improved Revenge"); r > 0 {
		warrior.AddStaticMod(core.SpellModConfig{Kind: core.SpellMod_DamageDone_Flat, ClassMask: ClassSpellMask_WarriorRevenge, IntValue: int64(20 * r)})
	}
	if r := rank("Defiance"); r > 0 && warrior.PseudoStats.CanBlock {
		warrior.foreverDefianceBonus = 0.05 * r
	}
	if r := rank("Bastion"); r > 0 && warrior.PseudoStats.CanBlock {
		warrior.PseudoStats.DamageDealtMultiplier *= 1 + 0.02*r
	}
	if r := int32(ft.Rank("Focused Rage")); r > 0 {
		warrior.OnSpellRegistered(func(spell *core.Spell) {
			if spell.Flags.Matches(SpellFlagOffensive) && spell.Cost != nil {
				spell.Cost.FlatModifier -= r
			}
		})
	}
}

// foreverBattleShoutAura: Forever's Battle Shout gives 139 attack power at max rank (Classic
// 232) and lasts 3 min; lower ranks are scaled by the same ratio (estimate). Booming Voice
// only increases the radius in Forever.
func foreverBattleShoutAura(unit *core.Unit, impBattleShout int32) *core.Aura {
	const label = "Battle Shout"
	if aura := unit.GetAura(label); aura != nil {
		return aura
	}
	rank := core.LevelToBuffRank[core.BattleShout][unit.Level]
	ap := math.Floor(core.BattleShoutBaseAP[rank] * 139 / 232 * (1 + 0.05*float64(impBattleShout)))
	return unit.RegisterAura(core.Aura{
		Label:      label,
		ActionID:   core.ActionID{SpellID: core.BattleShoutSpellId[rank]},
		Duration:   time.Minute * 3,
		BuildPhase: core.CharacterBuildPhaseBuffs,
	}).AttachBuildPhaseStatsBuff(stats.Stats{stats.AttackPower: ap})
}

// registerForeverSpearingStrike: "A brutal attack that deals 40% weapon damage, plus an
// additional 80% weapon damage against Giants, Dragonkin, and mounted targets."
// Cost, cooldown and normalization are estimates (see the header).
func (warrior *Warrior) registerForeverSpearingStrike() {
	warrior.SpearingStrike = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		ClassSpellMask: ClassSpellMask_WarriorSpearingStrike,
		ActionID:       core.ActionID{SpellID: foreverSpearingStrikeSpellID},
		SpellSchool:    core.SpellSchoolPhysical,
		DefenseType:    core.DefenseTypeMelee,
		ProcMask:       core.ProcMaskMeleeMHSpecial,
		Flags:          core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost:   10,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			weaponPct := 0.4
			if target.MobType == proto.MobType_MobTypeGiant || target.MobType == proto.MobType_MobTypeDragonkin {
				weaponPct += 0.8
			}
			baseDamage := weaponPct * spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower())
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}

// applyForeverBloodthrill: main-hand melee attacks against a target with the warrior's Rend
// have a 4% chance per rank to allow Overpower for 6 sec.
func (warrior *Warrior) applyForeverBloodthrill(procChance float64) {
	warrior.foreverBloodthrillAura = warrior.RegisterAura(core.Aura{
		Label:    "Bloodthrill",
		ActionID: core.ActionID{SpellID: foreverBloodthrillSpellID},
		Duration: time.Second * 6,
	})

	core.MakePermanent(warrior.RegisterAura(core.Aura{
		Label: "Bloodthrill Trigger",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.ProcMask.Matches(core.ProcMaskMeleeMH) || warrior.Rend == nil {
				return
			}
			if warrior.Rend.Dot(result.Target).IsActive() && sim.Proc(procChance, "Bloodthrill") {
				warrior.foreverBloodthrillAura.Activate(sim)
			}
		},
	}))
}

// applyForeverWeaponmasterMace: with a mace or staff, attacks ignore 3% of the target's armor
// per rank (estimate: armor penetration from the target's armor at the start of the fight).
func (warrior *Warrior) applyForeverWeaponmasterMace(armorIgnored float64) {
	mh := warrior.MainHand()
	if mh == nil || (mh.WeaponType != proto.WeaponType_WeaponTypeMace && mh.WeaponType != proto.WeaponType_WeaponTypeStaff) {
		return
	}
	var bonus float64
	core.MakePermanent(warrior.RegisterAura(core.Aura{
		Label: "Weaponmaster (Mace)",
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			bonus = 0
			if warrior.CurrentTarget != nil {
				bonus = armorIgnored * warrior.CurrentTarget.Armor()
			}
			warrior.AddStatDynamic(sim, stats.ArmorPenetration, bonus)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			warrior.AddStatDynamic(sim, stats.ArmorPenetration, -bonus)
		},
	}))
}

// applyForeverUnbridledWrath: 12% chance per rank to gain 1 Rage on a weapon hit, 2 Rage
// with a two-handed weapon (Classic: 8% per rank, 1 Rage).
func (warrior *Warrior) applyForeverUnbridledWrath(procChance float64) {
	rage := 1.0
	if mh := warrior.MainHand(); mh != nil && mh.HandType == proto.HandType_HandTypeTwoHand {
		rage = 2
	}
	rageMetrics := warrior.NewRageMetrics(core.ActionID{SpellID: 12964})

	core.MakePermanent(warrior.RegisterAura(core.Aura{
		Label: "Unbridled Wrath",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && sim.Proc(procChance, "Unbridled Wrath") {
				warrior.AddRage(sim, rage, rageMetrics)
			}
		},
	}))
}

// applyForeverEnrage: 30% chance after being the victim of any damaging attack to deal 2%
// per rank more Physical damage for 12 sec (Classic: on being crit, 5% per rank, 12 swings).
func (warrior *Warrior) applyForeverEnrage(bonus float64) {
	warrior.EnrageAura = warrior.GetOrRegisterAura(core.Aura{
		Label:    "Enrage",
		ActionID: core.ActionID{SpellID: 13048},
		Duration: time.Second * 12,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			warrior.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= 1 + bonus
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			warrior.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] /= 1 + bonus
		},
	})

	core.MakePermanent(warrior.RegisterAura(core.Aura{
		Label: "Enrage Trigger",
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Damage > 0 && sim.Proc(0.3, "Enrage") {
				warrior.EnrageAura.Activate(sim)
			}
		},
	}))
}

// applyForeverShieldSpecialization: +1% block chance per rank and a 20% chance per rank to
// gain 5 Rage on a block (Classic: 1 Rage).
func (warrior *Warrior) applyForeverShieldSpecialization(points float64) {
	warrior.AddStat(stats.Block, points*core.BlockRatingPerBlockChance)
	procChance := 0.2 * points
	rageMetrics := warrior.NewRageMetrics(core.ActionID{SpellID: 12727})

	core.MakePermanent(warrior.RegisterAura(core.Aura{
		Label: "Shield Specialization",
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidBlock() && sim.Proc(procChance, "Shield Specialization") {
				warrior.AddRage(sim, 5, rageMetrics)
			}
		},
	}))
}

// applyForeverMasterOfDefense: 50% chance per rank to gain 5 Rage on a dodge or parry while
// a shield is equipped.
func (warrior *Warrior) applyForeverMasterOfDefense(procChance float64) {
	rageMetrics := warrior.NewRageMetrics(core.ActionID{SpellID: 1290103}) // no public spell ID yet

	core.MakePermanent(warrior.RegisterAura(core.Aura{
		Label: "Master of Defense",
		OnSpellHitTaken: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !warrior.PseudoStats.CanBlock || !result.Outcome.Matches(core.OutcomeDodge|core.OutcomeParry) {
				return
			}
			if sim.Proc(procChance, "Master of Defense") {
				warrior.AddRage(sim, 5, rageMetrics)
			}
		},
	}))
}
