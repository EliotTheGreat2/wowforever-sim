#!/usr/bin/env python3
"""Generate WoW Forever presets for every spec.

For each spec page this writes:
  ui/<spec>/apls/forever/default.apl.json        rune-free rotation (from wowsims/classic)
  ui/<spec>/gear_sets/forever/preraid.gear.json  pre-raid gear (classic curated set, gaps auto-filled)
and one manifest, ui/core/forever/specs.json, that the Quick Sim page and the
engine audit test read.

Run from the repo root:  python3 tools/forever/gen_specs.py /path/to/wowsims-classic
"""
import json
import os
import sys

CLASSIC = sys.argv[1] if len(sys.argv) > 1 else '../classic-ref'
DB = json.load(open('assets/database/db.json'))
ITEMS = {i['id']: i for i in DB['items']}
NPC_ZONE = {n['id']: n.get('zoneId') for n in DB['npcs']}
RAID_ZONES = {1977, 2159, 2677, 2717, 3428, 3429, 3456, 16074, 16236, 15475}

# Class ids (proto Class)
DRUID, HUNTER, MAGE, PALADIN, PRIEST, ROGUE, SHAMAN, WARLOCK, WARRIOR = 1, 2, 3, 4, 5, 6, 7, 8, 9
MAX_ARMOR = {DRUID: 2, HUNTER: 3, MAGE: 1, PALADIN: 4, PRIEST: 1, ROGUE: 2, SHAMAN: 3, WARLOCK: 1, WARRIOR: 4}
HORDE, ALLIANCE = 2, 1

# Vanilla weapon proficiencies (proto WeaponType: 1 axe, 2 dagger, 3 fist, 4 mace, 5 held off-hand, 6 polearm, 7 shield, 8 staff, 9 sword)
WEAPONS = {
    DRUID: {2, 3, 4, 5, 8}, HUNTER: {1, 2, 3, 5, 6, 8, 9}, MAGE: {2, 5, 8, 9}, PALADIN: {1, 4, 5, 6, 7, 9},
    PRIEST: {2, 4, 5, 8}, ROGUE: {2, 3, 4, 5, 9}, SHAMAN: {1, 2, 3, 4, 5, 7, 8}, WARLOCK: {2, 5, 8, 9},
    WARRIOR: {1, 2, 3, 4, 5, 6, 7, 8, 9},
}

# Stat indexes (proto Stat)
STR, AGI, STA, INT, SPI, SP = 0, 1, 2, 3, 4, 5
ARCANE, FIRE, FROST, HOLY, NATURE, SHADOW = 6, 7, 8, 9, 10, 11
MP5, SPHIT, SPCRIT, AP, HIT, CRIT = 12, 13, 14, 17, 18, 19
ARMOR, RAP, DEF, BLOCK, BLOCKV, DODGE, PARRY, HEALTH, BONUSARMOR, HEAL, SPDMG, FERALAP = 26, 27, 28, 29, 30, 31, 32, 34, 40, 41, 42, 43


def caster(*schools):
    w = {SP: 1, SPDMG: 1, SPCRIT: 10, SPHIT: 12, INT: 0.4, SPI: 0.1, MP5: 1, STA: 0.05}
    for s in schools:
        w[s] = 1
    return w


HEALER = {HEAL: 1, SP: 1, MP5: 2.5, INT: 0.6, SPI: 0.4, SPCRIT: 8, STA: 0.05}
MELEE_STR = {STR: 2, AGI: 1, AP: 1, CRIT: 25, HIT: 25, STA: 0.1}
MELEE_AGI = {STR: 1, AGI: 2, AP: 1, CRIT: 25, HIT: 25, STA: 0.1}
FERAL = {STR: 2, AGI: 2, AP: 1, FERALAP: 1, CRIT: 25, HIT: 25, STA: 0.1}
HUNTER_W = {AGI: 2, RAP: 1, AP: 0.3, CRIT: 25, HIT: 25, STA: 0.1, INT: 0.2}
TANK = {STA: 2, DEF: 3, ARMOR: 0.05, BONUSARMOR: 0.05, DODGE: 20, PARRY: 20, BLOCK: 8, BLOCKV: 0.5,
        STR: 0.6, AGI: 1, HIT: 10, HEALTH: 0.1, AP: 0.3}
BEAR = {STA: 2, DEF: 3, ARMOR: 0.08, BONUSARMOR: 0.08, DODGE: 20, AGI: 1.5, STR: 1, FERALAP: 0.5, HIT: 10}

# page dir -> settings
SPECS = [
    # key, class, label, role, talents, classic apl, classic gear, weights, weapons, ranged types, faction
    ('balance_druid', DRUID, 'Balance', 'Caster DPS', '5000550012551251--5005031', 'balance_druid/apls/balance.apl.json', 'balance_druid/gear_sets/p0.bis.gear.json', caster(ARCANE, NATURE), 'caster', [4], HORDE),
    ('feral_druid', DRUID, 'Feral (Cat)', 'Melee DPS', '500005301-5500021323202151-05', 'feral_druid/apls/feral.apl.json', 'feral_druid/gear_sets/p2.pre-bis.gear.json', FERAL, 'twohand', [4], HORDE),
    ('feral_tank_druid', DRUID, 'Guardian (Bear)', 'Tank', '-503232132322010353120300313511-20350001', 'feral_tank_druid/apls/default.apl.json', None, BEAR, 'twohand', [4], HORDE),
    ('restoration_druid', DRUID, 'Restoration', 'Healer', '05320031103--230023312131502331050313051', None, None, HEALER, 'caster', [4], HORDE),
    ('elemental_shaman', SHAMAN, 'Elemental', 'Caster DPS', '050331552000151--50105301005', 'elemental_shaman/apls/default.apl.json', None, caster(NATURE), 'caster_shield', [7], HORDE),
    ('enhancement_shaman', SHAMAN, 'Enhancement', 'Melee DPS', '05-5025002105023051-05105301', 'enhancement_shaman/apls/default.apl.json', None, MELEE_STR, 'twohand', [7], HORDE),
    ('restoration_shaman', SHAMAN, 'Restoration', 'Healer', '-30205033-05005331335010501122331251', None, None, HEALER, 'caster_shield', [7], HORDE),
    ('hunter', HUNTER, 'Marksmanship', 'Ranged DPS', '55000000505-05451002503051', 'hunter/apls/p1.apl.json', 'hunter/gear_sets/p0.bis.gear.json', HUNTER_W, 'twohand', [1, 2, 3], HORDE),
    ('mage', MAGE, 'Frost', 'Caster DPS', '230205021002--05353203102351001', 'mage/apls/p1.apl.json', 'mage/gear_sets/p0.bis.gear.json', caster(FROST), 'caster', [8], HORDE),
    ('rogue', ROGUE, 'Combat', 'Melee DPS', '005323105-0240052020050150231', 'rogue/apls/combat_sinister_strike.apl.json', 'rogue/gear_sets/combat_sinister_strike_prebis.gear.json', MELEE_AGI, 'dual', [1, 2, 3, 6], HORDE),
    ('holy_paladin', PALADIN, 'Holy', 'Healer', '50350151020013053100515221-50023131203', None, None, HEALER, 'caster_shield', [5], ALLIANCE),
    ('protection_paladin', PALADIN, 'Protection', 'Tank', '-053020335001551-0500535', 'protection_paladin/apls/basic_prot.apl.json', None, TANK, 'shield', [5], ALLIANCE),
    ('retribution_paladin', PALADIN, 'Retribution', 'Melee DPS', '500501-503-52230351200315', 'retribution_paladin/apls/basic_ret.apl.json', None, MELEE_STR, 'twohand', [5], ALLIANCE),
    ('healing_priest', PRIEST, 'Holy', 'Healer', '05032031103-234051032002152530004311051', 'healing_priest/apls/holy.apl.json', None, HEALER, 'caster', [8], HORDE),
    ('shadow_priest', PRIEST, 'Shadow', 'Caster DPS', '0512301302--5002504103501251', 'shadow_priest/apls/p1.apl.json', 'shadow_priest/gear_sets/p0.bis.gear.json', caster(SHADOW), 'caster', [8], HORDE),
    ('warlock', WARLOCK, 'Destruction', 'Caster DPS', '5502203112201105--52500051020001', 'warlock/apls/rotation.apl.json', 'warlock/gear_sets/prebis.gear.json', caster(SHADOW, FIRE), 'caster', [8], HORDE),
    ('warrior', WARRIOR, 'Fury', 'Melee DPS', '30305001302-05050005525010051', 'warrior/apls/dps_reck.apl.json', 'warrior/gear_sets/p0.bis.gear.json', MELEE_STR, 'dual', [1, 2, 3, 6], HORDE),
    ('tank_warrior', WARRIOR, 'Protection', 'Tank', '20304300302-03-55200110530201051', 'tank_warrior/apls/dps_no_reck.apl.json', 'tank_warrior/gear_sets/p0.bis.gear.json', TANK, 'shield', [1, 2, 3, 6], HORDE),
]

# Equipment slot order used by EquipmentSpec.items
SLOT_TYPES = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 11, 12, 12, 13, 13, 14]  # ..., MH, OH, Ranged


# PvP rank, battleground reputation and later-patch event gear does not fit a Forever launch character.
EXCLUDE_NAME_PARTS = ("Insignia", "Lieutenant", "Marshal", "Legionnaire", "Champion's", "Highlander's", "Defiler's",
                      "Outrider's", "Sentinel's", "Stormpike", "Frostwolf", "Darkmoon", "Zandalar", "Sergeant",
                      "Blood Guard", "Knight-", "Warlord", "General's", "Commander", "Bronze Dragonflight", "Core ")


def preraid_ok(item, cls, faction):
    # ids below 19000 are original-release / pre-Zul'Gurub items; ilvl 65 cap excludes Molten Core and later raids
    if item['id'] >= 19000 or item.get('ilvl', 0) > 65 or item.get('requiresLevel', 0) > 60:
        return False
    if any(part in item['name'] for part in EXCLUDE_NAME_PARTS):
        return False
    if item.get('quality', 0) > 4 or item.get('requiredProfession') or item.get('randomSuffixOptions'):
        return False
    if item.get('sanctified') or item.get('timeworn'):
        return False
    if item.get('classAllowlist') and cls not in item['classAllowlist']:
        return False
    if item.get('factionRestriction') and item['factionRestriction'] != faction:
        return False
    if item['type'] in range(1, 11) and item.get('armorType', 0) > MAX_ARMOR[cls]:
        return False
    if item['type'] == 13 and item.get('weaponType') not in WEAPONS[cls]:
        return False
    for src in item.get('sources') or []:
        drop = (src or {}).get('drop')
        if drop and (drop.get('zoneId') or NPC_ZONE.get(drop.get('npcId'))) in RAID_ZONES:
            return False
    return True


def score(item, weights, melee):
    s = sum(item['stats'][k] * w for k, w in weights.items())
    if melee and item.get('weaponSpeed'):
        dps = (item['weaponDamageMin'] + item['weaponDamageMax']) / 2 / item['weaponSpeed']
        s += dps * 14
    return s


def pick(cls, weights, weapons, ranged_types, faction, current):
    melee = weapons in ('twohand', 'dual', 'shield') and weights is not HEALER
    pool = [i for i in ITEMS.values() if preraid_ok(i, cls, faction)]
    used = {x for x in current if x}

    def best(pred):
        cands = sorted((i for i in pool if pred(i) and (i['id'] not in used or not i.get('unique'))), key=lambda i: score(i, weights, melee), reverse=True)
        return cands[0]['id'] if cands else None

    out = list(current)
    for slot, t in enumerate(SLOT_TYPES[:14]):
        if out[slot]:
            continue
        chosen = best(lambda i, t=t: i['type'] == t and i['id'] not in used)
        out[slot] = chosen
        if chosen:
            used.add(chosen)
    if not out[14] and not out[15]:
        two = best(lambda i: i['type'] == 13 and i.get('handType') == 4 and (weapons != 'caster' or i.get('weaponType') == 8 or i['stats'][SP] or i['stats'][HEAL]))
        if weapons == 'twohand' and two:
            out[14] = two
        else:
            mh = best(lambda i: i['type'] == 13 and i.get('handType') in (1, 2))
            if weapons == 'dual':
                oh = best(lambda i: i['type'] == 13 and i.get('handType') in (2, 3) and i.get('weaponType') not in (5, 7) and i['id'] != mh)
            elif weapons in ('shield', 'caster_shield'):
                oh = best(lambda i: i['type'] == 13 and i.get('weaponType') == 7)
            else:
                oh = best(lambda i: i['type'] == 13 and i.get('weaponType') == 5)
            one_hand_score = sum(score(ITEMS[x], weights, melee) for x in (mh, oh) if x)
            if weapons == 'caster' and two and score(ITEMS[two], weights, melee) > one_hand_score:
                out[14] = two
            else:
                out[14], out[15] = mh, oh
    mh_item = ITEMS.get(out[14]) if out[14] else None
    if mh_item and mh_item.get('handType') != 4 and not out[15]:
        if weapons == 'dual':
            out[15] = best(lambda i: i['type'] == 13 and i.get('handType') in (2, 3) and i.get('weaponType') not in (5, 7) and i['id'] != out[14])
        elif weapons in ('shield', 'caster_shield'):
            out[15] = best(lambda i: i['type'] == 13 and i.get('weaponType') == 7)
        elif weapons == 'caster':
            out[15] = best(lambda i: i['type'] == 13 and i.get('weaponType') == 5)
    if not out[16]:
        out[16] = best(lambda i: i['type'] == 14 and i.get('rangedWeaponType') in ranged_types)
    return out


# Spec options (protojson) and default race used by the Quick Sim page and the audit test.
OPTIONS = {
    'feral_tank_druid': {'options': {'startingRage': 20}},
    'hunter': {'options': {'ammo': 'ThoriumHeadedArrow', 'petType': 'Cat', 'petUptime': 1}},
    'mage': {'options': {'armor': 'MageArmor'}},
    'holy_paladin': {'options': {'aura': 'ConcentrationAura'}},
    'protection_paladin': {'options': {'primarySeal': 'Righteousness', 'aura': 'DevotionAura', 'righteousFury': True}},
    'retribution_paladin': {'options': {'primarySeal': 'Command', 'aura': 'SanctityAura'}},
    'healing_priest': {'options': {'useInnerFire': True}},
    'shadow_priest': {'options': {'armor': 'InnerFire'}},
    'warlock': {'options': {'armor': 'DemonArmor', 'summon': 'Imp'}},
}
RACE = {DRUID: 'RaceTauren', HUNTER: 'RaceOrc', MAGE: 'RaceUndead', PALADIN: 'RaceHuman', PRIEST: 'RaceUndead',
        ROGUE: 'RaceOrc', SHAMAN: 'RaceOrc', WARLOCK: 'RaceOrc', WARRIOR: 'RaceOrc'}


SOD_SPELLS = {x['id']: x for x in DB.get('spellIcons', [])}
_known_path = 'ui/core/forever/known_spells.json'
KNOWN = json.load(open(_known_path)) if os.path.exists(_known_path) else {}
REMAP_LOG = []


def spell_name(spell_id, classic_spells):
    info = SOD_SPELLS.get(spell_id) or classic_spells.get(spell_id)
    return (info['name'], info.get('rank', 0)) if info else (None, 0)


def remap_rotation(key, apl, classic_spells):
    """Point rotation spell/aura IDs at the IDs the engine registers for this spec, matched by name and rank."""
    known = KNOWN.get(key)
    if not known:
        return apl
    pools = {'spells': set(known.get('spells', [])), 'auras': set(known.get('auras', []))}

    def by_name(pool):
        names = {}
        for sid in pools[pool]:
            name, rank = spell_name(sid, classic_spells)
            if name:
                names.setdefault(name, []).append((rank, sid))
        return names

    names = {'spells': by_name('spells'), 'auras': by_name('auras')}

    def fix(action_id, pool):
        sid = action_id.get('spellId')
        if not sid or sid in pools[pool]:
            return
        name, rank = spell_name(sid, classic_spells)
        cands = names[pool].get(name) or (names['spells'].get(name) if pool == 'auras' else None)
        if not cands:
            return
        same = [c for c in cands if c[0] == rank]
        new_rank, new_id = same[0] if same else max(cands)
        action_id['spellId'] = new_id
        if new_rank:
            action_id['rank'] = new_rank
        else:
            action_id.pop('rank', None)
        REMAP_LOG.append(f'{key}: {name} r{rank} {sid} -> {new_id} r{new_rank}')

    def walk(node, parent_key=None):
        if isinstance(node, dict):
            if 'spellId' in node and isinstance(node['spellId'], int):
                fix(node, 'auras' if parent_key in ('auraId', 'buffAuraId', 'debuffAuraId') else 'spells')
            for k, v in node.items():
                walk(v, k)
        elif isinstance(node, list):
            for v in node:
                walk(v, parent_key)

    walk(apl)
    return apl


def cmp_(op, lhs, val):
    return {'cmp': {'op': op, 'lhs': lhs, 'rhs': {'const': {'val': val}}}}


RAGE = {'currentRage': {}}
MANA_PCT = {'currentManaPercent': {}}
REMAINING = {'remainingTime': {}}

# Rotations written for Forever where the wowsims/classic one does not fit this engine.
OVERRIDE_APL = {
    # Classic's bear rotation targets later-expansion spell IDs. Vanilla bear: keep Faerie Fire up,
    # queue Maul, Swipe with spare rage, Enrage when starved.
    'feral_tank_druid': {
        'type': 'TypeAPL',
        'prepullActions': [{'action': {'castSpell': {'spellId': {'spellId': 9634}}}, 'doAtValue': {'const': {'val': '-1s'}}}],
        'priorityList': [
            {'action': {'condition': {'not': {'val': {'auraIsActive': {'auraId': {'spellId': 9634}}}}}, 'castSpell': {'spellId': {'spellId': 9634}}}},
            {'action': {'autocastOtherCooldowns': {}}},
            {'action': {'condition': cmp_('OpLt', RAGE, '15'), 'castSpell': {'spellId': {'spellId': 5229}}}},
            {'action': {'condition': {'not': {'val': {'auraIsActive': {'sourceUnit': {'type': 'CurrentTarget'}, 'auraId': {'spellId': 17392, 'rank': 4}}}}},
                        'castSpell': {'spellId': {'spellId': 17392, 'rank': 4}}}},
            {'action': {'condition': cmp_('OpGe', RAGE, '45'), 'castSpell': {'spellId': {'spellId': 9908, 'rank': 5}}}},
            {'action': {'condition': cmp_('OpGe', RAGE, '15'), 'castSpell': {'spellId': {'spellId': 9881, 'tag': 1, 'rank': 7}}}},
        ],
    },
}

# Extra actions inserted before the filler (last) action of a rotation.
EXTRA_BEFORE_FILLER = {
    # Classic's mage rotation never uses Evocation and runs dry on pre-raid mana pools.
    'mage': [{'action': {'condition': {'and': {'vals': [cmp_('OpLt', MANA_PCT, '12%'), cmp_('OpGt', REMAINING, '10s')]}},
                         'channelSpell': {'spellId': {'spellId': 12051}, 'interruptIf': cmp_('OpGt', MANA_PCT, '90%')}}}],
}


def clean_rotation(apl):
    """Drop placeholder conditions on an empty aura id (always false), which silently disable actions."""
    for item in apl.get('priorityList', []):
        cond = item.get('action', {}).get('condition')
        if cond and any(k in cond and not cond[k].get('auraId') for k in ('auraIsKnown', 'auraIsActive')):
            del item['action']['condition']
    return apl


def main():
    classic_db = json.load(open(os.path.join(CLASSIC, 'assets/database/db.json')))
    classic_spells = {x['id']: x for x in classic_db.get('spellIcons', [])}
    manifest = []
    for key, cls, label, role, talents, apl_src, gear_src, weights, weapons, ranged, faction in SPECS:
        current = [None] * 17
        source = 'auto-picked pre-raid'
        if gear_src:
            g = json.load(open(os.path.join(CLASSIC, 'ui', gear_src)))
            for slot, it in enumerate(g['items'][:17]):
                if it.get('id') in ITEMS and preraid_ok(ITEMS[it['id']], cls, faction):
                    current[slot] = it['id']
            source = 'wowsims/classic pre-raid' + (' + auto-filled' if None in current else '')
        items = pick(cls, weights, weapons, ranged, faction, current)

        gear_dir = f'ui/{key}/gear_sets/forever'
        os.makedirs(gear_dir, exist_ok=True)
        json.dump({'items': [{'id': x} if x else {} for x in items]}, open(f'{gear_dir}/preraid.gear.json', 'w'), indent=1)

        apl_path = None
        if apl_src:
            apl_dir = f'ui/{key}/apls/forever'
            os.makedirs(apl_dir, exist_ok=True)
            apl_path = f'{apl_dir}/default.apl.json'
            if key in OVERRIDE_APL:
                apl = OVERRIDE_APL[key]
            else:
                apl = clean_rotation(remap_rotation(key, json.load(open(os.path.join(CLASSIC, 'ui', apl_src))), classic_spells))
                if key in EXTRA_BEFORE_FILLER:
                    apl['priorityList'][-1:-1] = EXTRA_BEFORE_FILLER[key]
            json.dump(apl, open(apl_path, 'w'), indent=1)

        manifest.append({
            'key': key, 'classId': cls, 'spec': label, 'role': role, 'talents': talents,
            'rotation': apl_path, 'gear': f'{gear_dir}/preraid.gear.json', 'gearSource': source,
            'faction': 'Horde' if faction == HORDE else 'Alliance',
            'race': RACE[cls], 'specOptions': OPTIONS.get(key, {'options': {}}),
            'supported': role != 'Healer',
            # inlined for the Quick Sim page so it needs a single import
            'gearSpec': {'items': [{'id': x} if x else {} for x in items]},
            'rotationJson': json.load(open(apl_path)) if apl_path else {'type': 'TypeAuto'},
            'distance': 25 if role in ('Caster DPS', 'Ranged DPS', 'Healer') else 5,
        })
        names = [ITEMS[x]['name'] if x else '-' for x in items]
        print(f'{key:22s} {source:40s} {", ".join(names)}')

    print('\n'.join(REMAP_LOG))
    os.makedirs('ui/core/forever', exist_ok=True)
    json.dump(manifest, open('ui/core/forever/specs.json', 'w'), indent=1)


if __name__ == '__main__':
    main()
