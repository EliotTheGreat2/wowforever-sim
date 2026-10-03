package druid

import (
	"time"

	"github.com/wowsims/sod/sim/core"
)

// See https://www.wowhead.com/classic/spell=436895/s03-tuning-and-overrides-passive-druid
// Modifies Effect #1's Value -24%:
// Modifies Effect #2's Value +76:
const ShredWeaponMultiplierBuff = 0.75 // increases multiplier additively by 75% from 2.25 to 3.0
const ShredFlatDmgMultiplier = 0.75    // decreases flat damage modifier multiplicatively by -25% to counteract 3.0/2.25 overall scaling buff

func (druid *Druid) registerShredSpell() {
	damageMultiplier := 2.25
	flatDamageBonus := core.AtLevel(druid.Level, map[int32]float64{
		25: 24,
		40: 44,
		50: 64,
		60: 80,
	}) * ShredFlatDmgMultiplier

	// In-game testing concluded that, unintuitively, Idol of the Dream's 1.02x damage applies to the original 2.25x
	// Shred mod, and to the flat damage bonus, but that the .75x SoD buff happens additively after Idol
	damageMultiplier += ShredWeaponMultiplierBuff

	// WoW Forever: Classic Shred numbers retuned to 155% weapon damage (rank 5: +180), and no
	// Season of Discovery bonus against bleeding targets.
	bleedBonus := 1.3
	if druid.ForeverTalents != nil {
		damageMultiplier = 1.55
		flatDamageBonus = foreverShredFlatDamage(druid.Level)
		bleedBonus = 1
	}

	druid.Shred = druid.RegisterSpell(Cat, core.SpellConfig{
		ClassSpellMask: ClassSpellMask_DruidShred,
		ActionID: core.ActionID{SpellID: core.AtLevel(druid.Level, map[int32]int32{
			25: 5221,
			40: 8992,
			50: 9829,
			60: 9830,
		})},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOmen | SpellFlagBuilder,

		EnergyCost: core.EnergyCostOptions{
			Cost:   60 - 6*float64(druid.Talents.ImprovedShred),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return druid.ShredPositionOverride || !druid.PseudoStats.InFrontOfTarget
		},

		DamageMultiplier: damageMultiplier,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := flatDamageBonus + spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower())

			oldMultiplier := spell.GetDamageMultiplier()
			if druid.BleedCategories.Get(target).AnyActive() {
				spell.ApplyMultiplicativeDamageBonus(bleedBonus)
			}

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
			spell.SetMultiplicativeDamageBonus(oldMultiplier)

			if result.Landed() {
				druid.AddComboPoints(sim, 1, target, spell.ComboPointMetrics())
			} else {
				spell.IssueRefund(sim)
			}
		},
		ExpectedInitialDamage: func(sim *core.Simulation, target *core.Unit, spell *core.Spell, _ bool) *core.SpellResult {
			baseDamage := flatDamageBonus + spell.Unit.AutoAttacks.MH().CalculateAverageWeaponDamage(spell.MeleeAttackPower())

			oldMultiplier := spell.GetDamageMultiplier()
			if druid.BleedCategories.Get(target).AnyActive() {
				spell.ApplyMultiplicativeDamageBonus(bleedBonus)
			}

			baseres := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeExpectedMagicAlwaysHit)
			spell.SetMultiplicativeDamageBonus(oldMultiplier)

			attackTable := spell.Unit.AttackTables[target.UnitIndex][spell.CastType]
			critChance := spell.PhysicalCritChance(attackTable)
			critMod := critChance * (spell.CritMultiplier(attackTable) - 1)

			baseres.Damage *= 1 + critMod

			return baseres
		},
	})
}

func (druid *Druid) CanShred() bool {
	return !druid.PseudoStats.InFrontOfTarget && druid.CurrentEnergy() >= druid.CurrentShredCost()
}

func (druid *Druid) CurrentShredCost() float64 {
	return druid.Shred.Cost.GetCurrentCost()
}
