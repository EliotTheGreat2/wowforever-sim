#!/usr/bin/env python3
"""Re-pull WoW Forever data when a new beta build or the live game ships.

Run this on your own computer (it needs normal internet access), from the repo root:

    python3 tools/forever/update_forever_data.py talents      # talent trees -> raw/<class>.txt, shows what changed
    python3 tools/forever/update_forever_data.py spells       # saves spellbook pages for the spell-number update
    python3 tools/forever/update_forever_data.py items --branch forever   # after launch: item stats from Wowhead
    python3 tools/forever/update_forever_data.py rebuild      # regenerate everything and run the checks

Every download is also saved under assets/db_inputs/forever/pages/<date>/ so nothing is lost
if a site changes its page layout. If a parser can't read a page, the script says so; send
those saved pages to Claude and the parser (or the data) can be fixed from them.
"""
import argparse
import datetime
import difflib
import html
import json
import os
import re
import subprocess
import sys
import urllib.request

CLASSES = ['warrior', 'paladin', 'hunter', 'rogue', 'priest', 'shaman', 'mage', 'warlock', 'druid']
TALENTS_URL = 'https://wow-forever.gg/talents/{cls}/'
SPELLBOOK_URL = 'https://foreverchanges.pro/spellbook/{cls}'
RAW = 'assets/db_inputs/forever/talents/raw/'
PAGES = os.path.join('assets/db_inputs/forever/pages', datetime.date.today().isoformat())
UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0 Safari/537.36'


def fetch(url, save_as):
    req = urllib.request.Request(url, headers={'User-Agent': UA, 'Accept': 'text/html,application/json'})
    with urllib.request.urlopen(req, timeout=60) as r:
        body = r.read().decode('utf-8', 'replace')
    os.makedirs(os.path.dirname(save_as), exist_ok=True)
    open(save_as, 'w', encoding='utf-8').write(body)
    return body


# ---------------------------------------------------------------------------
# Talents

def json_blobs(page):
    """Every JSON value embedded in the page's <script> tags (Next.js/Nuxt data, JSON-LD, ...)."""
    for m in re.finditer(r'<script[^>]*>(.*?)</script>', page, re.S):
        body = m.group(1).strip()
        candidates = [body]
        # Next.js app router streams JSON inside self.__next_f.push([1,"..."]) strings.
        for s in re.findall(r'self\.__next_f\.push\(\[\d+,\s*"(.*?)"\]\)', body, re.S):
            try:
                candidates.append(json.loads('"' + s + '"'))
            except ValueError:
                pass
        for c in candidates:
            for start in [i for i, ch in enumerate(c) if ch in '[{'][:200]:
                try:
                    yield json.JSONDecoder().raw_decode(c[start:])[0]
                    break
                except ValueError:
                    continue


def walk(node):
    if isinstance(node, dict):
        yield node
        for v in node.values():
            yield from walk(v)
    elif isinstance(node, list):
        for v in node:
            yield from walk(v)


def first(d, *keys):
    for k in keys:
        if k in d and d[k] not in (None, ''):
            return d[k]
    return None


def talents_from_page(page):
    """Best-effort: find talent objects (name + max rank + tier/row) in embedded JSON."""
    found = []
    for blob in json_blobs(page):
        for d in walk(blob):
            name = first(d, 'name', 'talentName', 'title')
            max_rank = first(d, 'maxRank', 'max_rank', 'maxRanks', 'ranks', 'rankCount')
            tier = first(d, 'tier', 'row', 'tierIndex', 'rowIndex')
            if not isinstance(name, str) or tier is None or max_rank is None:
                continue
            if isinstance(max_rank, list):
                max_rank = len(max_rank)
            tree = first(d, 'tree', 'treeName', 'tab', 'spec', 'tabName')
            col = first(d, 'column', 'col', 'columnIndex')
            desc = first(d, 'description', 'desc', 'tooltip', 'text') or ''
            req = first(d, 'requires', 'prereq', 'prerequisite', 'requiredTalent')
            found.append({'tree': tree, 'tier': int(tier), 'col': col, 'name': name, 'maxRank': int(max_rank),
                          'requires': req if isinstance(req, str) else None,
                          'description': re.sub(r'<[^>]+>', '', html.unescape(str(desc))).strip()})
    return found


def talents_cmd(_args):
    problems = []
    for cls in CLASSES:
        page = fetch(TALENTS_URL.format(cls=cls), os.path.join(PAGES, 'talents', cls + '.html'))
        build = re.search(r'\b1\.\d+\.\d+\.\d{5}\b', page)
        talents = talents_from_page(page)
        trees = sorted({t['tree'] for t in talents if t['tree']}, key=lambda x: [t['tree'] for t in talents].index(x))
        if len(talents) < 40 or len(trees) != 3:
            problems.append(cls)
            print(f'{cls:8s} could not read the talent data from the page (found {len(talents)} talents, {len(trees)} trees)')
            continue
        tier_base = 0 if min(t['tier'] for t in talents) == 0 else 1
        lines = []
        for tree in trees:
            ts = [t for t in talents if t['tree'] == tree]
            ts.sort(key=lambda t: (t['tier'], t['col'] if isinstance(t['col'], int) else 0))
            for t in ts:
                desc = t['description'].replace('|', '/').replace('\n', ' ')
                col = t['col'] if isinstance(t['col'], int) else '?'
                lines.append(f"{tree} | {t['tier'] + 1 - tier_base} | {col} | {t['name']} | {t['maxRank']} | {t['requires'] or '-'} | {desc}")
        path = RAW + cls + '.txt'
        old = open(path).read().splitlines() if os.path.exists(path) else []
        diff = list(difflib.unified_diff(old, lines, 'current', 'new', lineterm='', n=0))
        print(f'{cls:8s} build {build.group(0) if build else "?"}: {len(lines)} talents, ' + (f'{len(diff)} changed lines' if diff else 'no changes'))
        for d in diff[2:]:
            print('   ', d[:160])
        open(path, 'w').write('\n'.join(lines) + '\n')
    if problems:
        print(f'\nPages saved in {PAGES}/talents. Couldn\'t parse: {", ".join(problems)} - send those files to Claude.')
    print('\nNext: update BUILD in tools/forever/gen_talents.py, then run: python3 tools/forever/update_forever_data.py rebuild')


# ---------------------------------------------------------------------------
# Spells

def spells_cmd(_args):
    for cls in CLASSES:
        page = fetch(SPELLBOOK_URL.format(cls=cls), os.path.join(PAGES, 'spellbook', cls + '.html'))
        text = re.sub(r'<[^>]+>', ' ', page)
        print(f'{cls:8s} saved ({len(text)} chars)')
    print(f'\nSpellbook pages saved in {PAGES}/spellbook. Spell numbers go into assets/db_inputs/forever/spell_tuning.csv;\n'
          'send the saved pages to Claude to update that file, then run the rebuild step.')


# ---------------------------------------------------------------------------
# Items

def items_cmd(args):
    cmd = ['go', 'run', './tools/forever_items', '-fetch', '-branch', args.branch]
    print(' '.join(cmd))
    subprocess.run(cmd, check=True)
    print('\nItems merged. Run: python3 tools/forever/update_forever_data.py rebuild')


# ---------------------------------------------------------------------------

def rebuild_cmd(_args):
    steps = [
        ['python3', 'tools/forever/gen_talents.py'],
        ['python3', 'tools/forever/gen_spell_tuning.py'],
        ['python3', 'tools/forever/gen_forever_builds.py'],
        ['go', 'test', '--tags=with_db', './sim', '-run', 'TestForever', '-count=1'],
        ['make', 'wowsimsod'],
    ]
    for step in steps:
        print('\n$', ' '.join(step))
        subprocess.run(step, check=True)
    print('\nAll done. Start the sim with ./wowsimsod (or rebuild the Windows launcher).')


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest='cmd', required=True)
    sub.add_parser('talents').set_defaults(fn=talents_cmd)
    sub.add_parser('spells').set_defaults(fn=spells_cmd)
    it = sub.add_parser('items')
    it.add_argument('--branch', default='forever', help='Wowhead branch for Forever (wowhead.com/<branch>/item=...)')
    it.set_defaults(fn=items_cmd)
    sub.add_parser('rebuild').set_defaults(fn=rebuild_cmd)
    args = p.parse_args()
    try:
        args.fn(args)
    except urllib.error.HTTPError as e:
        sys.exit(f'{e.url}: HTTP {e.code}. The site may block scripts; open the page in a browser, save it, and send it to Claude.')


if __name__ == '__main__':
    main()
