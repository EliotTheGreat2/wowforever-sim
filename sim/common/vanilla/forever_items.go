package vanilla

// Vanilla item effects missing from the SoD item set, ported from wowsims/classic
// for WoW Forever pre-raid gear.

import (
	"time"

	"github.com/wowsims/sod/sim/common/itemhelpers"
	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/stats"
)

const (
	ForeverManualCrowdPummeler = 9449
	ForeverRivenspike          = 13286
)

// Armor reductions from item procs share one exclusive category so they don't stack with each other.
const foreverMinorArmorReductionCategory = "ForeverMinorArmorReduction"

func init() {
	core.AddEffectsToTest = false

	// https://www.wowhead.com/classic/item=9449/manual-crowd-pummeler
	// Use: Increases attack speed by 50% for 30 sec.
	core.NewItemEffect(ForeverManualCrowdPummeler, func(agent core.Agent) {
		character := agent.GetCharacter()
		actionID := core.ActionID{SpellID: 13494}

		hasteAura := character.GetOrRegisterAura(core.Aura{
			Label:    "Manual Crowd Pummeler Haste",
			ActionID: actionID,
			Duration: time.Second * 30,
		}).AttachMultiplyAttackSpeed(&character.Unit, 1.5)

		spell := character.RegisterSpell(core.SpellConfig{
			ActionID: actionID,
			Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagOffensiveEquipment,
			Cast: core.CastConfig{
				CD: core.Cooldown{
					Timer:    character.NewTimer(),
					Duration: time.Second * 30,
				},
			},
			ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
				hasteAura.Activate(sim)
			},
		})
		character.AddMajorCooldown(core.MajorCooldown{
			Type:  core.CooldownTypeDPS,
			Spell: spell,
		})
	})

	// https://www.wowhead.com/classic/item=13286/rivenspike
	// Chance on hit: Punctures target's armor lowering it by 200. Can be applied up to 3 times.
	// 2 PPM, matching wowsims/classic.
	itemhelpers.CreateWeaponProcSpell(ForeverRivenspike, "Rivenspike", 2.0, func(character *core.Character) *core.Spell {
		punctureArmorAuras := character.NewEnemyAuraArray(foreverPunctureArmorAura)

		return character.GetOrRegisterSpell(core.SpellConfig{
			ActionID:         core.ActionID{SpellID: 17315},
			SpellSchool:      core.SpellSchoolPhysical,
			DefenseType:      core.DefenseTypeMelee,
			ProcMask:         core.ProcMaskEmpty,
			DamageMultiplier: 1,
			ThreatMultiplier: 1,
			ApplyEffects: func(sim *core.Simulation, target *core.Unit, _ *core.Spell) {
				aura := punctureArmorAuras.Get(target)
				aura.Activate(sim)
				if aura.IsActive() {
					aura.AddStack(sim)
				}
			},
		})
	})

	core.AddEffectsToTest = true
}

func foreverPunctureArmorAura(target *core.Unit, _ int32) *core.Aura {
	const armorPerStack = 200.0

	var effect *core.ExclusiveEffect
	aura := target.GetOrRegisterAura(core.Aura{
		Label:     "Puncture Armor",
		ActionID:  core.ActionID{SpellID: 17315},
		Duration:  time.Second * 30,
		MaxStacks: 3,
		OnStacksChange: func(_ *core.Aura, sim *core.Simulation, _ int32, newStacks int32) {
			effect.SetPriority(sim, armorPerStack*float64(newStacks))
		},
	})

	effect = aura.NewExclusiveEffect(foreverMinorArmorReductionCategory, true, core.ExclusiveEffect{
		OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatDynamic(sim, stats.Armor, -ee.Priority)
		},
		OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatDynamic(sim, stats.Armor, ee.Priority)
		},
	})
	return aura
}
