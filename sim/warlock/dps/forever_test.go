package dps

import (
	"fmt"
	"testing"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
)

// WoW Forever comparison harness.
// Runs the same level-60 warlock with and without the Forever ruleset so the
// effect of each Forever mechanic is visible as it gets implemented.
//
//	go test --tags=with_db ./sim/warlock/dps -run TestForever -v

// Vanilla talent builds (Forever talent trees are not datamined yet).
var ForeverAffTalents = Phase4AffTalents       // affliction / ruin hybrid
var ForeverDestroTalents = Phase4DestroTalents // destruction

// Buffs a typical Forever launch group can provide (no SoD-only world buffs).
var foreverRaidBuffs = &proto.RaidBuffs{
	ArcaneBrilliance:   true,
	GiftOfTheWild:      proto.TristateEffect_TristateEffectImproved,
	PowerWordFortitude: proto.TristateEffect_TristateEffectImproved,
	DivineSpirit:       true,
}
var foreverPlayerBuffs = &proto.IndividualBuffs{
	BlessingOfKings:  true,
	BlessingOfWisdom: proto.TristateEffect_TristateEffectImproved,
}

func foreverRequest(talents, apl string, forever bool, iterations int32) *proto.RaidSimRequest {
	player := core.WithSpec(&proto.Player{
		Class:              proto.Class_ClassWarlock,
		Level:              60,
		Race:               proto.Race_RaceOrc,
		Equipment:          core.GetGearSet("../../../ui/warlock/gear_sets/forever", "preraid_placeholder").GearSet,
		Consumes:           &proto.Consumes{},
		Buffs:              foreverPlayerBuffs,
		TalentsString:      talents,
		Rotation:           core.GetAplRotation("../../../ui/warlock/apls/forever", apl).Rotation,
		DistanceFromTarget: core.MaxShortSpellRange,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
		ForeverRuleset:     forever,
	}, DefaultAfflictionWarlock)

	return &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, foreverRaidBuffs, &proto.Debuffs{}),
		Encounter: &proto.Encounter{
			Duration:             180,
			DurationVariation:    20,
			ExecuteProportion_20: 0.2,
			ExecuteProportion_25: 0.25,
			ExecuteProportion_35: 0.35,
			Targets:              []*proto.Target{core.NewDefaultTarget(60)},
		},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: 101},
	}
}

func TestForeverCompare(t *testing.T) {
	specs := []struct{ name, talents, apl string }{
		{"Affliction", ForeverAffTalents, "affliction"},
		{"Destruction", ForeverDestroTalents, "destruction"},
	}
	for _, s := range specs {
		var dps [2]float64
		for i, forever := range []bool{false, true} {
			res := core.RunRaidSim(foreverRequest(s.talents, s.apl, forever, 3000))
			if res.Error != nil {
				t.Fatalf("%s forever=%v: %s", s.name, forever, res.Error.Message)
			}
			dps[i] = res.RaidMetrics.Dps.Avg
			corruptionCrits := dotCrits(res, 11672)
			if forever != (corruptionCrits > 0) {
				t.Fatalf("%s forever=%v: Corruption crits=%d", s.name, forever, corruptionCrits)
			}
			if forever && dps[i] <= 0 {
				t.Fatalf("%s: no damage done in Forever mode", s.name)
			}
		}
		t.Log(fmt.Sprintf("%-12s vanilla rules: %7.1f DPS | Forever rules: %7.1f DPS (%+.1f%%)",
			s.name, dps[0], dps[1], 100*(dps[1]/dps[0]-1)))
	}
}

// dotCrits counts critical ticks of a periodic spell (the DoT action is tagged 1).
func dotCrits(res *proto.RaidSimResult, spellID int32) int32 {
	var crits int32
	for _, action := range res.RaidMetrics.Parties[0].Players[0].Actions {
		if action.Id.GetSpellId() != spellID {
			continue
		}
		for _, target := range action.Targets {
			crits += target.Crits + target.CritTicks
		}
	}
	return crits
}
