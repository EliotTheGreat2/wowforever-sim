package core

import (
	"time"

	"github.com/wowsims/sod/sim/core/proto"
	"github.com/wowsims/sod/sim/core/stats"
)

// WoW Forever racials (Blizzard's official race/class list, Sept 22 2026, via Icy Veins).
// Forever reworked most racials and added two Skyborne races; the Classic versions in
// racials.go are used without the Forever ruleset.
//
// Not modeled: cooldowns Blizzard hasn't published (Elune's Light, Eureka!), utility
// (Stoneform, Will to Survive, War Stomp, Shatter Curse, ...), and Touch of the Grave
// (no proc rate published).

func applyForeverRaceEffects(character *Character) {
	mh := character.Equipment.MainHand()
	mhIs := func(types ...proto.WeaponType) bool {
		if mh == nil {
			return false
		}
		for _, t := range types {
			if mh.WeaponType == t {
				return true
			}
		}
		return false
	}
	critBonus := func(pct float64) {
		character.AddStat(stats.MeleeCrit, pct*CritRatingPerCritChance)
		character.AddStat(stats.SpellCrit, pct*SpellCritRatingPerCritChance)
	}
	versusMobType := func(mobType proto.MobType, mult float64) {
		character.Env.RegisterPostFinalizeEffect(func() {
			for _, t := range character.Env.Encounter.Targets {
				if t.MobType == mobType {
					for _, at := range character.AttackTables[t.UnitIndex] {
						at.DamageDealtMultiplier *= mult
					}
				}
			}
		})
	}

	switch character.Race {
	case proto.Race_RaceHuman:
		// The Human Spirit: 5% Spirit. Sword Specialization: +2% crit with a sword.
		character.MultiplyStat(stats.Spirit, 1.05)
		if mhIs(proto.WeaponType_WeaponTypeSword) {
			critBonus(2)
		}
	case proto.Race_RaceDwarf:
		// Mace Specialization: +1% crit with a mace. Big Game Hunter: +5% damage to Beasts.
		character.AddStat(stats.FrostResistance, 10)
		if mhIs(proto.WeaponType_WeaponTypeMace) {
			critBonus(1)
		}
		versusMobType(proto.MobType_MobTypeBeast, 1.05)
	case proto.Race_RaceNightElf:
		// Quickness: +1% dodge.
		character.AddStat(stats.NatureResistance, 10)
		character.AddStat(stats.Dodge, 1)
	case proto.Race_RaceGnome:
		// Expansive Mind: +5% maximum Mana, Rage or Energy.
		character.AddStat(stats.ArcaneResistance, 10)
		character.MultiplyStat(stats.Mana, 1.05)
	case proto.Race_RaceOrc:
		// Axe Specialization: +1% crit with an axe. Blood Fury: +10% Attack Power and Spell Power for 15 sec.
		if mhIs(proto.WeaponType_WeaponTypeAxe) {
			critBonus(1)
		}
		foreverBloodFury(character)
	case proto.Race_RaceTauren:
		// Endurance: +5% Health and +1% hit.
		character.AddStat(stats.NatureResistance, 10)
		character.MultiplyStat(stats.Health, 1.05)
		character.AddStat(stats.MeleeHit, 1*MeleeHitRatingPerHitChance)
		character.AddStat(stats.SpellHit, 1*SpellHitRatingPerHitChance)
	case proto.Race_RaceTroll:
		// Beast Slaying: +5% damage to Beasts. Berserking: +10% casting and attack speed for 10 sec.
		versusMobType(proto.MobType_MobTypeBeast, 1.05)
		foreverBerserking(character)
	case proto.Race_RaceUndead:
		character.AddStat(stats.ShadowResistance, 10)
	case proto.Race_RaceHighOrderSkyborne, proto.Race_RaceWindshaperSkyborne:
		// Wind Blessed: +1% melee, ranged and spell haste. Elemental Insight: +5% damage to Elementals.
		MakePermanent(character.RegisterAura(Aura{
			Label:    "Wind Blessed",
			ActionID: ActionID{SpellID: 1290101},
			OnGain: func(aura *Aura, sim *Simulation) {
				character.MultiplyAttackSpeed(sim, 1.01)
				character.MultiplyCastSpeed(1.01)
			},
			OnExpire: func(aura *Aura, sim *Simulation) {
				character.MultiplyAttackSpeed(sim, 1/1.01)
				character.MultiplyCastSpeed(1 / 1.01)
			},
		}))
		versusMobType(proto.MobType_MobTypeElemental, 1.05)
	}
}

// foreverBloodFury: +10% Attack Power and Spell Power for 15 sec (Classic's 2 min cooldown).
func foreverBloodFury(character *Character) {
	actionID := ActionID{SpellID: 20572}
	var ap, sp float64
	aura := character.RegisterAura(Aura{
		Label:    "Blood Fury",
		ActionID: actionID,
		Duration: time.Second * 15,
		OnGain: func(aura *Aura, sim *Simulation) {
			ap = character.GetStat(stats.AttackPower) * 0.10
			sp = character.GetStat(stats.SpellPower) * 0.10
			character.AddStatsDynamic(sim, stats.Stats{stats.AttackPower: ap, stats.RangedAttackPower: ap, stats.SpellPower: sp})
		},
		OnExpire: func(aura *Aura, sim *Simulation) {
			character.AddStatsDynamic(sim, stats.Stats{stats.AttackPower: -ap, stats.RangedAttackPower: -ap, stats.SpellPower: -sp})
		},
	})
	spell := character.RegisterSpell(SpellConfig{
		ActionID: actionID,
		Flags:    SpellFlagNoOnCastComplete,
		Cast: CastConfig{
			DefaultCast: Cast{GCD: GCDDefault},
			CD:          Cooldown{Timer: character.NewTimer(), Duration: time.Minute * 2},
		},
		ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
			aura.Activate(sim)
		},
	})
	character.AddMajorCooldown(MajorCooldown{Spell: spell, Type: CooldownTypeDPS})
}

// foreverBerserking: +10% casting and attack speed for 10 sec (Classic's 3 min cooldown).
func foreverBerserking(character *Character) {
	actionID := ActionID{SpellID: 26297}
	aura := character.RegisterAura(Aura{
		Label:    "Berserking",
		ActionID: actionID,
		Duration: time.Second * 10,
		OnGain: func(aura *Aura, sim *Simulation) {
			character.MultiplyAttackSpeed(sim, 1.10)
			character.MultiplyCastSpeed(1.10)
		},
		OnExpire: func(aura *Aura, sim *Simulation) {
			character.MultiplyAttackSpeed(sim, 1/1.10)
			character.MultiplyCastSpeed(1 / 1.10)
		},
	})
	spell := character.RegisterSpell(SpellConfig{
		ActionID: actionID,
		Flags:    SpellFlagNoOnCastComplete,
		Cast: CastConfig{
			CD: Cooldown{Timer: character.NewTimer(), Duration: time.Minute * 3},
		},
		ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
			aura.Activate(sim)
		},
	})
	character.AddMajorCooldown(MajorCooldown{Spell: spell, Type: CooldownTypeDPS})
}
