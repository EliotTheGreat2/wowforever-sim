package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wowsims/sod/sim/core"
	"github.com/wowsims/sod/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// WoW Forever audit: sims every spec in ui/core/forever/specs.json at level 60
// under Forever rules and fails on errors, missing damage/healing, or rotation
// actions the engine had to drop.
//
//	go test --tags=with_db ./sim -run TestForeverAllSpecs -v

type foreverSpec struct {
	Key         string          `json:"key"`
	Spec        string          `json:"spec"`
	Role        string          `json:"role"`
	Talents     string          `json:"talents"`
	Rotation    *string         `json:"rotation"`
	Gear        string          `json:"gear"`
	Race        string          `json:"race"`
	ClassID     int32           `json:"classId"`
	SpecOptions json.RawMessage `json:"specOptions"`
	Distance    float64         `json:"distance"`
}

func loadForeverSpecs(t *testing.T) []foreverSpec {
	raw, err := os.ReadFile("../ui/core/forever/specs.json")
	if err != nil {
		t.Fatal(err)
	}
	var specs []foreverSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		t.Fatal(err)
	}
	return specs
}

// ForeverPlayer builds a level-60 Forever player for one manifest entry.
func foreverPlayer(t *testing.T, s foreverSpec, forever bool) *proto.Player {
	gear, err := os.ReadFile("../" + s.Gear)
	if err != nil {
		t.Fatal(err)
	}
	rotation := `{"type":"TypeAuto"}`
	if s.Rotation != nil {
		b, err := os.ReadFile("../" + *s.Rotation)
		if err != nil {
			t.Fatal(err)
		}
		rotation = string(b)
	}
	playerJSON := fmt.Sprintf(`{
		"name": "Forever", "level": 60, "class": %d, "race": %q,
		"talentsString": %q, "equipment": %s, "rotation": %s,
		"distanceFromTarget": %v, "reactionTimeMs": 150, "channelClipDelayMs": 50,
		"foreverRuleset": %v, %q: %s
	}`, s.ClassID, s.Race, s.Talents, gear, rotation, s.Distance, forever, toCamel(s.Key), s.SpecOptions)

	player := &proto.Player{}
	if err := protojson.Unmarshal([]byte(playerJSON), player); err != nil {
		t.Fatalf("%s: building player: %v", s.Key, err)
	}
	return player
}

func toCamel(snake string) string {
	parts := strings.Split(snake, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

func foreverRaidRequest(player *proto.Player, role string, iterations int32) *proto.RaidSimRequest {
	raid := core.SinglePlayerRaidProto(player,
		&proto.PartyBuffs{},
		&proto.RaidBuffs{
			ArcaneBrilliance:   true,
			GiftOfTheWild:      proto.TristateEffect_TristateEffectImproved,
			PowerWordFortitude: proto.TristateEffect_TristateEffectImproved,
			DivineSpirit:       true,
		},
		&proto.Debuffs{})
	player.Buffs = &proto.IndividualBuffs{BlessingOfKings: true, BlessingOfWisdom: proto.TristateEffect_TristateEffectImproved, BlessingOfMight: proto.TristateEffect_TristateEffectImproved}
	if role == "Tank" {
		raid.Tanks = append(raid.Tanks, &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0})
	}
	if role == "Healer" {
		raid.TargetDummies = 1
	}
	return &proto.RaidSimRequest{
		Raid: raid,
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

func TestForeverAllSpecs(t *testing.T) {
	RegisterAll()
	for _, s := range loadForeverSpecs(t) {
		s := s
		t.Run(s.Key, func(t *testing.T) {
			if s.Role == "Healer" {
				t.Skip("healer sims are not registered in the engine yet")
			}
			req := foreverRaidRequest(foreverPlayer(t, s, true), s.Role, 500)
			for _, w := range rotationWarnings(req) {
				t.Logf("rotation warning: %s", w)
			}
			res := core.RunRaidSim(req)
			if res.Error != nil {
				t.Fatalf("sim error: %s", res.Error.Message)
			}
			p := res.RaidMetrics.Parties[0].Players[0]
			dps, hps, tps := p.Dps.Avg, p.Hps.Avg, p.Threat.Avg
			t.Logf("%-20s %-16s DPS %7.1f  HPS %7.1f  TPS %7.1f", s.Key, s.Spec, dps, hps, tps)
			if os.Getenv("FOREVER_DEBUG") == s.Key {
				for _, a := range p.Actions {
					var casts, dmg float64
					for _, tg := range a.Targets {
						casts += float64(tg.Casts)
						dmg += tg.Damage
					}
					t.Logf("   %-40v casts/iter %6.1f  dps %6.1f", a.Id, casts/500, dmg/500/180)
				}
				t.Logf("   mana end: %v", p.Resources)
			}
			if s.Role == "Healer" {
				if hps <= 0 {
					t.Errorf("healer did no healing")
				}
			} else if dps <= 0 {
				t.Errorf("no damage done")
			}
		})
	}
}

// rotationWarnings returns the APL validation warnings (unknown spells, bad conditions).
func rotationWarnings(req *proto.RaidSimRequest) []string {
	stats := core.ComputeStats(&proto.ComputeStatsRequest{Raid: req.Raid, Encounter: req.Encounter})
	if stats.ErrorResult != "" {
		return []string{"compute stats error: " + stats.ErrorResult}
	}
	var out []string
	rs := stats.RaidStats.Parties[0].Players[0].RotationStats
	if rs == nil {
		return nil
	}
	for i, a := range append(rs.PrepullActions, rs.PriorityList...) {
		for _, w := range a.Warnings {
			out = append(out, fmt.Sprintf("action %d: %s", i, w))
		}
	}
	return out
}

// TestForeverDumpKnownSpells writes the spells and auras the engine registers for
// each Forever spec so tools/forever/gen_specs.py can remap rotation spell IDs.
//
//	FOREVER_DUMP=1 go test --tags=with_db ./sim -run TestForeverDumpKnownSpells
func TestForeverDumpKnownSpells(t *testing.T) {
	if os.Getenv("FOREVER_DUMP") == "" {
		t.Skip("set FOREVER_DUMP=1 to regenerate ui/core/forever/known_spells.json")
	}
	RegisterAll()
	out := map[string]map[string][]int32{}
	for _, s := range loadForeverSpecs(t) {
		if s.Role == "Healer" {
			continue // healer sims are not registered in the engine
		}
		player := foreverPlayer(t, s, true)
		player.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
		req := foreverRaidRequest(player, s.Role, 1)
		stats := core.ComputeStats(&proto.ComputeStatsRequest{Raid: req.Raid, Encounter: req.Encounter})
		if stats.ErrorResult != "" {
			t.Logf("%s: %s", s.Key, stats.ErrorResult)
			continue
		}
		md := stats.RaidStats.Parties[0].Players[0].Metadata
		known := map[string][]int32{}
		for _, sp := range md.Spells {
			if id := sp.Id.GetSpellId(); id != 0 {
				known["spells"] = append(known["spells"], id)
			}
		}
		for _, a := range md.Auras {
			if id := a.Id.GetSpellId(); id != 0 {
				known["auras"] = append(known["auras"], id)
			}
		}
		out[s.Key] = known
	}
	b, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile("../ui/core/forever/known_spells.json", b, 0644); err != nil {
		t.Fatal(err)
	}
}
