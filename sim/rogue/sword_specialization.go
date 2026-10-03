package rogue

import (
	"time"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
)

func (rogue *Rogue) registerSwordSpecialization(mask core.ProcMask) {
	if rogue.Talents.SwordSpecialization == 0 || rogue.GetProcMaskForTypes(proto.WeaponType_WeaponTypeSword) == core.ProcMaskUnknown {
		return
	}

	rogue.registerExtraAttackProc("Sword Specialization", mask, 0.01*float64(rogue.Talents.SwordSpecialization))
}

// registerExtraAttackProc: a chance on hit with the masked weapons to gain an extra main-hand
// attack (Sword Specialization; WoW Forever's Hack and Slash with Swords and Axes).
func (rogue *Rogue) registerExtraAttackProc(label string, mask core.ProcMask, procChance float64) {
	icd := core.Cooldown{
		Timer:    rogue.NewTimer(),
		Duration: time.Millisecond * 200,
	}

	rogue.RegisterAura(core.Aura{
		Label:    label,
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			if !spell.ProcMask.Matches(mask) {
				return
			}
			if !icd.IsReady(sim) {
				return
			}
			if sim.RandomFloat(label) < procChance {
				icd.Use(sim)
				rogue.AutoAttacks.ExtraMHAttack(sim, 1, core.ActionID{SpellID: 13964}, spell.ActionID)
			}
		},
	})
}
