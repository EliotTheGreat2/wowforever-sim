#!/usr/bin/env python3
"""Generate per-level (1-60) base stats and stat ratios for WoW Forever leveling sims.

The engine ships measured values only for levels 25/40/50/60. This fills every other
level from vanilla data, anchored so the engine's tested values stay exactly as they are:

  * Strength/Agility/Stamina/Intellect/Spirit per level: `player_levelstats`
    (cmangos classic-db, 1.12.1) for the class's baseline race, minus the engine's race offset.
  * Base health/mana: `player_classlevelstats` (same source).
  * Agility per 1% melee crit and per 1% dodge: vmangos `player_crit_per_agility` /
    `player_dodge_per_agility`, sniffed from the original game. Gaps are interpolated
    linearly in agility-per-percent, as vmangos itself does.
  * Intellect -> spell crit: the client game table (assets/db_inputs/basestats/chancetospellcrit.txt)
    for the per-level shape, scaled to the engine's measured values.

Where a source and the engine disagree at 25/40/50/60, the difference is carried across
levels by linear interpolation (held flat below 25) so the engine anchors are reproduced exactly.

Run from the repo root: python3 tools/forever/gen_levels.py
Writes sim/core/base_stats_forever_gen.go
"""
import csv
import json
import re
import subprocess

SRC = 'assets/db_inputs/forever/'
ANCHORS = [25, 40, 50, 60]
LEVELS = range(1, 61)

CLASSES = {  # proto name -> (vanilla class id, baseline race id, client table column)
    'ClassWarrior': (1, 1, 'Warrior'),
    'ClassPaladin': (2, 1, 'Paladin'),
    'ClassHunter': (3, 2, 'Hunter'),
    'ClassRogue': (4, 1, 'Rogue'),
    'ClassPriest': (5, 1, 'Priest'),
    'ClassShaman': (7, 2, 'Shaman'),
    'ClassMage': (8, 1, 'Mage'),
    'ClassWarlock': (9, 1, 'Warlock'),
    'ClassDruid': (11, 4, 'Druid'),
}
RACE_PROTO = {1: 'RaceHuman', 2: 'RaceOrc', 4: 'RaceNightElf'}
ATTRS = ['Strength', 'Agility', 'Stamina', 'Intellect', 'Spirit']
GO_STAT = {'Strength': 'stats.Strength', 'Agility': 'stats.Agility', 'Stamina': 'stats.Stamina', 'Intellect': 'stats.Intellect',
           'Spirit': 'stats.Spirit', 'Health': 'stats.Health', 'Mana': 'stats.Mana', 'AttackPower': 'stats.AttackPower',
           'RangedAttackPower': 'stats.RangedAttackPower'}


def engine_tables():
    """Dump the engine's current tables (anchors) via a throwaway Go test."""
    test = r'''package core
import ("encoding/json"; "os"; "testing"; "github.com/wowsims/sod/sim/core/stats")
func TestZZForeverDump(t *testing.T) {
	cls := map[string]map[int]map[string]float64{}
	for c, lv := range ClassBaseStats { cls[c.String()] = map[int]map[string]float64{}
		for l, st := range lv { m := map[string]float64{}; for i, v := range st { if v != 0 { m[stats.Stat(i).StatName()] = v } }; cls[c.String()][l] = m } }
	race := map[string]map[string]float64{}
	for r, st := range RaceOffsets { m := map[string]float64{}; for i, v := range st { if v != 0 { m[stats.Stat(i).StatName()] = v } }; race[r.String()] = m }
	agi := map[string]map[int]float64{}; for c, lv := range CritPerAgiAtLevel { agi[c.String()] = lv }
	intl := map[string]map[int]float64{}; for c, lv := range CritPerIntAtLevel { intl[c.String()] = lv }
	dodge := map[string]map[int]float64{}; for c, lv := range DodgePerAgiAtLevel { dodge[c.String()] = lv }
	b, _ := json.Marshal(map[string]any{"class": cls, "race": race, "critAgi": agi, "critInt": intl, "dodgeAgi": dodge})
	os.WriteFile(os.Getenv("FOREVER_DUMP_OUT"), b, 0644)
}
'''
    path = 'sim/core/zz_forever_dump_test.go'
    open(path, 'w').write(test)
    out = '/tmp/forever_engine_tables.json'
    try:
        subprocess.run(['go', 'test', './sim/core', '-run', 'TestZZForeverDump', '-count=1'], check=True,
                       env={**__import__('os').environ, 'FOREVER_DUMP_OUT': out}, capture_output=True)
    finally:
        __import__('os').remove(path)
    return json.load(open(out))


def interp(points, level, below='flat'):
    """Piecewise-linear interpolation through {level: value}; flat outside the range."""
    keys = sorted(points)
    if level <= keys[0]:
        return points[keys[0]]
    if level >= keys[-1]:
        return points[keys[-1]]
    for lo, hi in zip(keys, keys[1:]):
        if lo <= level <= hi:
            w = (level - lo) / (hi - lo)
            return points[lo] + (points[hi] - points[lo]) * w
    raise ValueError(level)


def load_levelstats():
    s = open(SRC + 'player_levelstats.sql').read()
    stats = {}
    for r, c, l, st, ag, sta, it, sp in re.findall(r"\((\d+),(\d+),(\d+),(\d+),(\d+),(\d+),(\d+),(\d+)\)", s):
        stats[(int(r), int(c), int(l))] = dict(zip(ATTRS, map(int, (st, ag, sta, it, sp))))
    s = open(SRC + 'player_classlevelstats.sql').read()
    base = {(int(c), int(l)): {'Health': int(h), 'Mana': int(m)} for c, l, h, m in re.findall(r"\((\d+),(\d+),(\d+),(\d+)\)", s)}
    return stats, base


def load_rates(files, table):
    rates = {}
    for f in files:
        s = open(SRC + f).read()
        for blk in re.findall(r"INSERT INTO `%s` \(`class`, `level`, `rate`\) VALUES\s*(.*?);" % table, s, re.S):
            for c, l, r in re.findall(r"\((\d+),\s*(\d+),\s*([\d.]+)\)", blk):
                rates.setdefault(int(c), {})[int(l)] = float(r)
    return rates


def load_client_table(name):
    rows = list(csv.reader(open('assets/db_inputs/basestats/' + name), delimiter='\t'))
    hdr = rows[0]
    return {int(r[0]): {hdr[i]: float(r[i]) for i in range(1, len(hdr))} for r in rows[1:]}


def main():
    eng = engine_tables()
    levelstats, classlevel = load_levelstats()
    crit_rates = load_rates(['vmangos_player_crit_per_agility.sql', 'vmangos_crit_dodge_fill.sql'], 'player_crit_per_agility')
    dodge_rates = load_rates(['vmangos_player_dodge_per_agility.sql', 'vmangos_crit_dodge_fill.sql'], 'player_dodge_per_agility')
    spell_tbl = load_client_table('chancetospellcrit.txt')

    base_out, crit_out, dodge_out, int_out = {}, {}, {}, {}
    max_err = 0.0
    for cname, (cid, rid, col) in CLASSES.items():
        anchors = {int(k): v for k, v in eng['class'][cname].items()}
        offset = eng['race'][RACE_PROTO[rid]]

        def source(level):
            row = dict(levelstats[(rid, cid, level)])
            row = {k: v - offset.get(k, 0) for k, v in row.items()}
            row.update(classlevel[(cid, level)])
            return row

        keys = set(ATTRS) | {'Health', 'Mana'}
        corr = {k: {L: anchors[L].get(k, 0) - source(L)[k] for L in ANCHORS} for k in keys}
        # Remaining stats (attack power etc.) are linear in level in every class; fit through the anchors.
        extra = {k for L in ANCHORS for k in anchors[L]} - keys
        fits = {}
        for k in extra:
            xs = ANCHORS
            ys = [anchors[L].get(k, 0) for L in xs]
            n = len(xs)
            mx, my = sum(xs) / n, sum(ys) / n
            sxx = sum((x - mx) ** 2 for x in xs)
            slope = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sxx
            fits[k] = (slope, my - slope * mx)
            max_err = max(max_err, max(abs(slope * x + fits[k][1] - y) for x, y in zip(xs, ys)))
            if k not in GO_STAT:
                raise SystemExit(f'{cname}: unhandled base stat {k}')

        base_out[cname] = {}
        for L in LEVELS:
            if L in anchors:
                continue
            src = source(L)
            row = {k: round(src[k] + interp(corr[k], L)) for k in keys}
            for k, (a, b) in fits.items():
                row[k] = round(a * L + b)
            base_out[cname][L] = row

        def rate_table(rates, engine_pct):
            pts = dict(rates.get(cid, {}))
            for L, pct in engine_pct.items():
                if pct:
                    pts[int(L)] = 1.0 / pct  # engine stores percent per point; rates are points per percent
            return {L: 1.0 / interp(pts, L) for L in LEVELS}

        crit_out[cname] = rate_table(crit_rates, eng['critAgi'][cname])
        dodge_out[cname] = rate_table(dodge_rates, eng['dodgeAgi'][cname])

        eng_int = {int(k): v for k, v in eng['critInt'][cname].items()}
        if any(eng_int.values()):
            scale = {L: eng_int[L] / (spell_tbl[L][col] * 100) for L in ANCHORS}
            int_out[cname] = {L: spell_tbl[L][col] * 100 * interp(scale, L) for L in LEVELS}
        else:
            int_out[cname] = {L: 0.0 for L in LEVELS}

        # Anchors must round-trip exactly.
        for L in ANCHORS:
            for name, tbl, ref in (('critAgi', crit_out, eng['critAgi']), ('dodgeAgi', dodge_out, eng['dodgeAgi']), ('critInt', int_out, eng['critInt'])):
                assert abs(tbl[cname][L] - ref[cname][str(L)]) < 1e-9, (cname, name, L)

    lines = [
        '// Code generated by tools/forever/gen_levels.py. DO NOT EDIT.',
        '// Per-level (1-60) base stats and stat ratios for WoW Forever leveling sims.',
        '// Sources: cmangos classic-db player_levelstats / player_classlevelstats (1.12.1),',
        '// vmangos player_crit_per_agility / player_dodge_per_agility, client chancetospellcrit table.',
        '// Levels the engine already defines (25/40/50/60) are left untouched.',
        '',
        'package core',
        '',
        'import (',
        '\t"github.com/wowsims/sod/sim/core/proto"',
        '\t"github.com/wowsims/sod/sim/core/stats"',
        ')',
        '',
        'func init() {',
    ]
    for cname in CLASSES:
        lines.append(f'\tforeverFillBaseStats(proto.Class_{cname}, map[int]stats.Stats{{')
        for L, row in sorted(base_out[cname].items()):
            parts = ', '.join(f'{GO_STAT[k]}: {v}' for k, v in sorted(row.items()) if v)
            lines.append(f'\t\t{L}: {{{parts}}},')
        lines.append('\t})')
        for var, tbl in (('CritPerAgiAtLevel', crit_out), ('DodgePerAgiAtLevel', dodge_out), ('CritPerIntAtLevel', int_out)):
            vals = ', '.join(f'{L}: {tbl[cname][L]:.6f}' for L in LEVELS)
            lines.append(f'\tforeverFillRatios({var}, proto.Class_{cname}, map[int]float64{{{vals}}})')
    lines.append('}')
    open('sim/core/base_stats_forever_gen.go', 'w').write('\n'.join(lines) + '\n')
    print('wrote sim/core/base_stats_forever_gen.go; attack-power fit max error', max_err)


if __name__ == '__main__':
    main()
