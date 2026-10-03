#!/usr/bin/env python3
"""Add WoW Forever talent builds (and the rotations that use their new spells) to
ui/core/forever/specs.json.

A build is a pick order: points go into talents in this order as the character levels,
one point per level from 10, so every level 10-60 gets a legal build that matches how
people actually level. Specs listed here sim with their Forever talent trees; the rest
keep Classic talents until their Forever talents are implemented in the engine.

Run from the repo root after tools/forever/gen_specs.py and tools/forever/gen_talents.py:
    python3 tools/forever/gen_forever_builds.py
"""
import json

# One file per spec in tools/forever/builds/<spec key>.json:
#   {"rotation": "ui/<spec>/apls/forever/<file>.apl.json",
#    "options": {spec option overrides},
#    "order": [[tree index, "Talent Name", points], ...]}   # 51 points, in leveling order
# A file for a key that isn't in specs.json adds a spec VARIANT that runs on an existing
# engine sim, e.g. tools/forever/builds/mage_fire.json:
#    "base": "mage", "label": "Fire", "role": "Caster DPS" (optional, default: base's),
#    "weapons": {"mainHand": ["Dagger"], "offHand": ["Dagger"]}  (optional: weapon types to
#    re-pick at every level from db.json, for specs that need e.g. daggers). For a two-hander:
#    "weapons": {"twoHand": ["Axe", "Sword"], "excludeRaid": true, "level60": {"mainHand": 12784}}
#    picks the best two-handed weapon of those types for the main hand and empties the off hand;
#    "excludeRaid" skips raid drops (the level 60 set is pre-raid), "level60" pins item IDs at 60.
#    "dualWieldLevel": 20 (in "weapons") leaves the off hand empty below that level;
#    "distance": 5  (optional: yards from the target, e.g. for a melee variant of a ranged spec)
import copy
import glob
import os

BUILDS = {}
BUILD_FILES = {}
for path in sorted(glob.glob('tools/forever/builds/*.json')):
    b = json.load(open(path))
    BUILDS[os.path.basename(path)[:-5]] = (b['rotation'], [tuple(x) for x in b['order']], b.get('options', {}))
    BUILD_FILES[os.path.basename(path)[:-5]] = b

# proto Class enum (classId in specs.json) -> talents.json class key
CLASS_FILE = {1: 'druid', 2: 'hunter', 3: 'mage', 4: 'paladin', 5: 'priest', 6: 'rogue', 7: 'shaman', 8: 'warlock', 9: 'warrior'}


SLOT = {'mainHand': 14, 'offHand': 15}
RAID_ZONES = {2717, 2677, 3428, 3456, 1977, 3429, 2159}  # MC, BWL, AQ40, Naxx, ZG, AQ20, Onyxia
WEAPON_TYPE = {'Axe': 1, 'Dagger': 2, 'Fist': 3, 'Mace': 4, 'OffHand': 5, 'Polearm': 6, 'Shield': 7, 'Staff': 8, 'Sword': 9}
_ITEMS = None


def repick_weapons(spec, weapons):
    """Re-pick main/off hand at every level from db.json among the given weapon types (one-handers,
    or "twoHand": a two-hander in the main hand and nothing in the off hand), by weapon DPS, for
    vanilla items the character can use at that level."""
    global _ITEMS
    if _ITEMS is None:
        _ITEMS = [i for i in json.load(open('assets/database/db.json'))['items'] if i['id'] < 25000]
    weapons = dict(weapons)
    exclude_raid = weapons.pop('excludeRaid', False)
    level60 = weapons.pop('level60', {})
    dual_wield_level = weapons.pop('dualWieldLevel', 0)  # no off-hand weapon below this level

    def is_raid_drop(i):
        return any((s.get('drop') or {}).get('zoneId') in RAID_ZONES for s in i.get('sources') or [])

    def best(slot, types, level, exclude=None, two_hand=False):
        # HandType: unknown/main/one-hand or unknown/one-hand/off (one-handers), or two-hand.
        hand_ok = (4,) if two_hand else {14: (0, 1, 2), 15: (0, 2, 3)}[slot]
        pool = [i for i in _ITEMS if i.get('weaponType') in types and i.get('handType', 0) in hand_ok
                and i.get('requiresLevel', 0) <= level and (i.get('requiresLevel', 0) or i.get('ilvl', 0) - 10 <= level)
                and i.get('quality', 0) <= (4 if level >= 60 else 3) and i['id'] != exclude
                and not i.get('classAllowlist') and not (exclude_raid and is_raid_drop(i))]
        if not pool:
            return None
        dps = lambda i: (i.get('weaponDamageMin', 0) + i.get('weaponDamageMax', 0)) / 2 / max(i.get('weaponSpeed', 1), 0.1)
        return max(pool, key=lambda i: (dps(i), i.get('ilvl', 0)))['id']

    def pick(level, gear):
        mh = None
        for name, types in weapons.items():
            ids = [WEAPON_TYPE[t] for t in types]
            if name == 'twoHand':
                choice = level60.get('mainHand') if level >= 60 else None
                choice = choice or best(SLOT['mainHand'], ids, level, two_hand=True)
                if choice:
                    gear[SLOT['mainHand']] = choice
                    gear[SLOT['offHand']] = 0
                continue
            choice = level60.get(name) if level >= 60 else None
            choice = choice or best(SLOT[name], ids, level, exclude=mh if name == 'offHand' else None)
            if choice:
                gear[SLOT[name]] = choice
                if name == 'mainHand':
                    mh = choice
        if level < dual_wield_level:
            gear[SLOT['offHand']] = 0
        return gear

    items = [it.get('id', 0) for it in spec['gearSpec']['items']]
    items += [0] * (17 - len(items))
    items = pick(60, items)
    spec['gearSpec'] = {'items': [{'id': x} if x else {} for x in items]}
    for level, lv in spec['levels'].items():
        lv['gear'] = pick(int(level), list(lv['gear']) + [0] * (17 - len(lv['gear'])))


def build_string(trees, order, points):
    alloc = [[0] * len(t['talents']) for t in trees]
    index = [{x['name']: j for j, x in enumerate(t['talents'])} for t in trees]
    spent = 0
    for tree, name, n in order:
        j = index[tree][name]
        tal = trees[tree]['talents'][j]
        for _ in range(n):
            if spent >= points:
                break
            if sum(alloc[tree]) < (tal['tier'] - 1) * 5:
                raise SystemExit(f'{name}: needs {(tal["tier"] - 1) * 5} points in {trees[tree]["name"]} first')
            if tal['requires'] and alloc[tree][index[tree][tal['requires']]] == 0:
                raise SystemExit(f'{name}: requires {tal["requires"]}')
            alloc[tree][j] += 1
            spent += 1
        if alloc[tree][j] > tal['maxRank']:
            raise SystemExit(f'{name}: {alloc[tree][j]} > {tal["maxRank"]}')
    if points >= 51 and spent != 51:
        raise SystemExit(f'pick order spends {spent} points, expected 51')
    return '-'.join(''.join(map(str, a)).rstrip('0') for a in alloc)


def main():
    specs = json.load(open('ui/core/forever/specs.json'))
    talents = json.load(open('ui/core/forever/talents.json'))
    specs = [s for s in specs if not s.get('sim')]  # variants are regenerated below
    by_key = {s['key']: s for s in specs}
    variants = []
    for key, b in BUILD_FILES.items():
        if key in by_key or 'base' not in b:
            continue
        v = copy.deepcopy(by_key[b['base']])
        v.update(key=key, sim=b['base'], spec=b['label'], role=b.get('role', v['role']))
        if 'distance' in b:
            v['distance'] = b['distance']
        if 'weapons' in b:
            repick_weapons(v, b['weapons'])
        variants.append(v)
    # Keep each class's specs together, base spec first.
    for v in variants:
        i = max(i for i, s in enumerate(specs) if s['classId'] == v['classId'])
        specs.insert(i + 1, v)
    for spec in specs:
        if spec['key'] not in BUILDS:
            spec.pop('foreverTalents', None)
            for lv in spec.get('levels', {}).values():
                lv.pop('foreverTalents', None)
            continue
        rotation, order, options = BUILDS[spec['key']]
        spec['specOptions']['options'].update(options)
        trees = talents[CLASS_FILE[spec['classId']]]['trees']
        spec['foreverTalents'] = build_string(trees, order, 51)
        spec['rotation'] = rotation
        spec['rotationJson'] = json.load(open(rotation))
        for level, lv in spec['levels'].items():
            lv['foreverTalents'] = build_string(trees, order, int(level) - 9)
        print(f'{spec["key"]:20s} {spec["foreverTalents"]}  (level 30: {spec["levels"]["30"]["foreverTalents"]})')
    json.dump(specs, open('ui/core/forever/specs.json', 'w'), indent=1)


if __name__ == '__main__':
    main()
