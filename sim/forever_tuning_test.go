package sim

// Reports how much the WoW Forever spell retune (forever_spell_tuning_gen.go) moves each
// spec at level 60, and fails if it stops applying to casters.
//
//	go test --tags=with_db ./sim -run TestForeverSpellTuningReport -v
import (
	"github.com/wowsims/sod/sim/core"
	"testing"
)

func TestForeverSpellTuningReport(t *testing.T) {
	RegisterAll()
	for _, s := range loadForeverSpecs(t) {
		if s.Role == "Healer" {
			continue
		}
		run := func() float64 {
			r := core.RunRaidSim(foreverRaidRequest(foreverPlayer(t, s, true), s.Role, 300))
			return r.RaidMetrics.Parties[0].Players[0].Dps.Avg
		}
		with := run()
		saved := core.ForeverSpellTunings
		core.ForeverSpellTunings = map[int32]core.ForeverSpellTuning{}
		without := run()
		core.ForeverSpellTunings = saved
		t.Logf("%-20s classic numbers %6.1f  forever numbers %6.1f  (%+.0f%%)", s.Key, without, with, (with/without-1)*100)
		if (s.Key == "warlock" || s.Key == "mage" || s.Key == "elemental_shaman") && with >= without {
			t.Errorf("%s: Forever spell numbers not applied", s.Key)
		}
	}
}
