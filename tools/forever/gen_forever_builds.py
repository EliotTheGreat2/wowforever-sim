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
#    re-pick at every level from db.json, for specs that need e.g. daggers)
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
WEAPON_TYPE = {'Axe': 1, 'Dagger': 2, 'Fist': 3, 'Mace': 4, 'OffHand': 5, 'Polearm': 6, 'Shield': 7, 'Staff': 8, 'Sword': 9}
_ITEMS = None


def repick_weapons(spec, weapons):
    """Re-pick main/off hand at every level from db.json among the given weapon types (one-handers),
    by weapon DPS, for vanilla items the character can use at that level."""
    global _ITEMS
    if _ITEMS is None:
        _ITEMS = [i for i in json.load(open('assets/database/db.json'))['items'] if i['id'] < 25000]

    def best(slot, types, level, exclude=None):
        hand_ok = {14: (0, 1, 2), 15: (0, 3, 4)}[slot]  # HandType: unknown/one/main or one/off
        pool = [i for i in _ITEMS if i.get('weaponType') in types and i.get('handType', 0) in hand_ok
                and i.get('requiresLevel', 0) <= level and (i.get('requiresLevel', 0) or i.get('ilvl', 0) - 10 <= level)
                and i.get('quality', 0) <= (4 if level >= 60 else 3) and i['id'] != exclude
                and not i.get('classAllowlist')]
        if not pool:
            return None
        dps = lambda i: (i.get('weaponDamageMin', 0) + i.get('weaponDamageMax', 0)) / 2 / max(i.get('weaponSpeed', 1), 0.1)
        return max(pool, key=lambda i: (dps(i), i.get('ilvl', 0)))['id']

    def pick(level, gear):
        mh = None
        for name, types in weapons.items():
            ids = [WEAPON_TYPE[t] for t in types]
            choice = best(SLOT[name], ids, level, exclude=mh if name == 'offHand' else None)
            if choice:
                gear[SLOT[name]] = choice
                if name == 'mainHand':
                    mh = choice
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
