package core

// WoW Forever retuned most class spells: caster base damage was cut sharply (Shadow Bolt
// 482-538 -> 253-283 at max rank) and some periodic coefficients were raised. The engine's
// spells carry Classic numbers, so each tuned spell rank gets a base damage scale (Forever
// max rank / Classic max rank) and, where measured, a new spell power coefficient.
// Data: assets/db_inputs/forever/spell_tuning.csv -> forever_spell_tuning_gen.go
// (tools/forever/gen_spell_tuning.py).

type ForeverSpellTuning struct {
	DirectScale float64 // multiplies the base damage of the initial hit
	DotScale    float64 // multiplies periodic base damage
	DirectCoeff float64 // replaces the spell power coefficient of the hit (0 = keep)
	TickCoeff   float64 // replaces the per-tick spell power coefficient (0 = keep)
}

func (spell *Spell) applyForeverTuning() {
	t, ok := ForeverSpellTunings[spell.ActionID.SpellID]
	if !ok {
		return
	}
	spell.foreverDirectScale = t.DirectScale
	spell.foreverDotScale = t.DotScale

	tickCoeffUsed := false
	if t.TickCoeff > 0 {
		for _, dot := range spell.dots {
			if dot != nil && dot.BonusCoefficient > 0 {
				dot.BonusCoefficient = t.TickCoeff
				tickCoeffUsed = true
			}
		}
		if spell.aoeDot != nil && spell.aoeDot.BonusCoefficient > 0 {
			spell.aoeDot.BonusCoefficient = t.TickCoeff
			tickCoeffUsed = true
		}
	}
	if spell.BonusCoefficient > 0 {
		if t.DirectCoeff > 0 {
			spell.BonusCoefficient = t.DirectCoeff
		} else if t.TickCoeff > 0 && !tickCoeffUsed {
			// Channels whose ticks are dealt as direct damage.
			spell.BonusCoefficient = t.TickCoeff
		}
	}
}
