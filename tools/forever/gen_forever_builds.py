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

# spec key -> (rotation file, pick order [(tree index, talent name, points)], spec option overrides)
BUILDS = {
    'warlock': ('ui/warlock/apls/forever/forever_destruction.apl.json', [
        (2, 'Bane', 5),
        (2, 'Improved Shadow Bolt', 5),
        (2, 'Ruin', 5),
        (2, 'Agonizing Flames', 3),
        (2, 'Conflagrate', 1),
        (2, 'Cataclysm', 3),
        (2, 'Bane of Havoc', 1),
        (2, 'Fire and Brimstone', 3),
        (2, 'Aftermath', 3),
        (2, 'Shadow and Flame', 5),
        (2, 'Incinerate', 1),
        (2, 'Aftermath', 2),
        (1, 'Demonic Embrace', 5),
        (1, 'Unholy Power', 5),
        (1, 'Demonic Sacrifice', 1),
        (1, 'Master Summoner', 2),
        (1, 'Fel Vitality', 1),
    ], {'summon': 'Succubus'}),  # Forever's Demonic Sacrifice: Succubus -> +15% Fire
}

CLASS_FILE = {1: 'warrior', 2: 'paladin', 3: 'hunter', 4: 'rogue', 5: 'priest', 7: 'shaman', 8: 'mage', 9: 'warlock', 11: 'druid'}
PROTO_CLASS_TO_GAME = {1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 7, 7: 8, 8: 9, 9: 11}  # proto Class enum -> game class id


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
    for spec in specs:
        if spec['key'] not in BUILDS:
            spec.pop('foreverTalents', None)
            for lv in spec.get('levels', {}).values():
                lv.pop('foreverTalents', None)
            continue
        rotation, order, options = BUILDS[spec['key']]
        spec['specOptions']['options'].update(options)
        trees = talents[CLASS_FILE[PROTO_CLASS_TO_GAME[spec['classId']]]]['trees']
        spec['foreverTalents'] = build_string(trees, order, 51)
        spec['rotation'] = rotation
        spec['rotationJson'] = json.load(open(rotation))
        for level, lv in spec['levels'].items():
            lv['foreverTalents'] = build_string(trees, order, int(level) - 9)
        print(f'{spec["key"]:20s} {spec["foreverTalents"]}  (level 30: {spec["levels"]["30"]["foreverTalents"]})')
    json.dump(specs, open('ui/core/forever/specs.json', 'w'), indent=1)


if __name__ == '__main__':
    main()
