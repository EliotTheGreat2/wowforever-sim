// Forever Sim: a three-step quick sim for WoW Forever.
// Pick class + spec, pick gear (pre-raid preset or addon paste), press Sim.
import './forever.css';

import specsJson from '../core/forever/specs.json';
import { RaidSimRequest, RaidSimResult, StatWeightsRequest } from '../core/proto/api';
import { EquipmentSpec, Race, SimDatabase, Stat } from '../core/proto/common';
import { ActionId } from '../core/proto_utils/action_id';
import { Database } from '../core/proto_utils/database';
import { nameToClass, nameToRace } from '../core/proto_utils/names';
import { SimSignals } from '../core/sim_signal_manager';
import { WorkerPool } from '../core/worker_pool';

type ForeverSpec = {
	key: string;
	classId: number;
	spec: string;
	role: 'Caster DPS' | 'Melee DPS' | 'Ranged DPS' | 'Tank' | 'Healer';
	talents: string;
	race: string;
	faction: string;
	supported: boolean;
	distance: number;
	gearSource: string;
	gearSpec: { items: Array<{ id?: number; enchant?: number }> };
	rotationJson: unknown;
	specOptions: Record<string, unknown>;
};

const SPECS = specsJson as unknown as ForeverSpec[];

type ClassInfo = { id: number; name: string; enumName: string; color: string; ink: string; icon: string; races: string[] };

// Class colors are the game's own. Races are vanilla combos plus Forever's Troll warlocks.
const CLASSES: ClassInfo[] = [
	{ id: 9, name: 'Warrior', enumName: 'ClassWarrior', color: '#C69B6D', ink: '#141a2e', icon: 'classicon_warrior', races: ['RaceOrc', 'RaceTauren', 'RaceTroll', 'RaceUndead', 'RaceHuman', 'RaceDwarf', 'RaceNightElf', 'RaceGnome'] },
	{ id: 4, name: 'Paladin', enumName: 'ClassPaladin', color: '#F48CBA', ink: '#141a2e', icon: 'classicon_paladin', races: ['RaceHuman', 'RaceDwarf'] },
	{ id: 2, name: 'Hunter', enumName: 'ClassHunter', color: '#AAD372', ink: '#141a2e', icon: 'classicon_hunter', races: ['RaceOrc', 'RaceTroll', 'RaceTauren', 'RaceDwarf', 'RaceNightElf'] },
	{ id: 6, name: 'Rogue', enumName: 'ClassRogue', color: '#FFF468', ink: '#141a2e', icon: 'classicon_rogue', races: ['RaceOrc', 'RaceUndead', 'RaceTroll', 'RaceHuman', 'RaceDwarf', 'RaceNightElf', 'RaceGnome'] },
	{ id: 5, name: 'Priest', enumName: 'ClassPriest', color: '#F2F2F2', ink: '#141a2e', icon: 'classicon_priest', races: ['RaceUndead', 'RaceTroll', 'RaceHuman', 'RaceDwarf', 'RaceNightElf'] },
	{ id: 7, name: 'Shaman', enumName: 'ClassShaman', color: '#3D95F0', ink: '#141a2e', icon: 'classicon_shaman', races: ['RaceOrc', 'RaceTroll', 'RaceTauren'] },
	{ id: 3, name: 'Mage', enumName: 'ClassMage', color: '#3FC7EB', ink: '#141a2e', icon: 'classicon_mage', races: ['RaceUndead', 'RaceTroll', 'RaceHuman', 'RaceGnome'] },
	{ id: 8, name: 'Warlock', enumName: 'ClassWarlock', color: '#9B9CF2', ink: '#141a2e', icon: 'classicon_warlock', races: ['RaceOrc', 'RaceUndead', 'RaceTroll', 'RaceHuman', 'RaceGnome'] },
	{ id: 1, name: 'Druid', enumName: 'ClassDruid', color: '#FF7C0A', ink: '#141a2e', icon: 'classicon_druid', races: ['RaceTauren', 'RaceNightElf'] },
];

const RACE_LABEL: Record<string, string> = {
	RaceOrc: 'Orc', RaceUndead: 'Undead', RaceTroll: 'Troll', RaceTauren: 'Tauren',
	RaceHuman: 'Human', RaceDwarf: 'Dwarf', RaceNightElf: 'Night Elf', RaceGnome: 'Gnome',
};

// Which talent tree points to which spec page, for addon imports.
const TREE_TO_SPEC: Record<number, string[]> = {
	1: ['balance_druid', 'feral_druid', 'restoration_druid'],
	2: ['hunter', 'hunter', 'hunter'],
	3: ['mage', 'mage', 'mage'],
	4: ['holy_paladin', 'protection_paladin', 'retribution_paladin'],
	5: ['healing_priest', 'healing_priest', 'shadow_priest'],
	6: ['rogue', 'rogue', 'rogue'],
	7: ['elemental_shaman', 'enhancement_shaman', 'restoration_shaman'],
	8: ['warlock', 'warlock', 'warlock'],
	9: ['warrior', 'warrior', 'tank_warrior'],
};

const SLOT_NAMES = ['Head', 'Neck', 'Shoulder', 'Back', 'Chest', 'Wrist', 'Hands', 'Waist', 'Legs', 'Feet', 'Ring', 'Ring', 'Trinket', 'Trinket', 'Main hand', 'Off hand', 'Ranged'];

const ITERATIONS = 3000;
const FIGHT_SECONDS = 180;

const ICON = (name: string) => `https://wow.zamimg.com/images/wow/icons/large/${name}.jpg`;

// ---------------------------------------------------------------------------
// State

type Character = { source: 'preset' | 'import'; imported?: boolean; gear: ForeverSpec['gearSpec']; talents: string; race: string; note?: string };

const state: {
	cls?: ClassInfo;
	spec?: ForeverSpec;
	character?: Character;
	running: boolean;
} = { running: false };

const pool = new WorkerPool(1);
let dbPromise: Promise<Database> | null = null;
const db = () => (dbPromise ??= Database.get());

// ---------------------------------------------------------------------------
// Rendering helpers

const app = document.getElementById('app')!;

function h(html: string): HTMLElement {
	const t = document.createElement('template');
	t.innerHTML = html.trim();
	return t.content.firstElementChild as HTMLElement;
}

function esc(s: string): string {
	return s.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
}

function setClassColor(cls?: ClassInfo) {
	document.documentElement.style.setProperty('--cls', cls?.color ?? '#c9b88a');
	document.documentElement.style.setProperty('--cls-ink', cls?.ink ?? '#141a2e');
}

function render() {
	app.innerHTML = '';
	app.append(
		h(`<header class="fs-top">
			<div class="fs-brand">Forever Sim <span>for World of Warcraft: Forever</span></div>
			<a class="fs-advanced" href="${state.spec ? `../${state.spec.key}/` : '../warlock/'}">Advanced sim</a>
		</header>`),
		h(`<section>
			<h1>How hard do you hit?</h1>
			<p class="fs-lede">Pick your class, sim, done. Uses WoW Forever's combat rules with a level 60 character against a raid boss.</p>
		</section>`),
		renderClassStep(),
		renderCharacterStep(),
		renderSimStep(),
		h(`<div id="fs-results"></div>`),
		h(`<p class="fs-foot">Preview: Forever's new talents and items aren't in the game data yet, so builds use vanilla talents and pre-raid gear. Healer sims are coming later.
			Built on <a href="https://github.com/wowsims/sod" target="_blank" rel="noopener">WoWSims</a> (MIT).</p>`),
	);
}

function renderClassStep(): HTMLElement {
	const step = h(`<section class="fs-step" aria-labelledby="s1">
		<div class="fs-step-num" aria-hidden="true">1</div>
		<div>
			<h2 id="s1">Pick your class</h2>
			<div class="fs-classes" role="group" aria-label="Class"></div>
			<div class="fs-specs" role="group" aria-label="Spec"></div>
		</div>
	</section>`);
	const grid = step.querySelector('.fs-classes')!;
	for (const cls of CLASSES) {
		const btn = h(`<button type="button" class="fs-class" style="--c:${cls.color}" aria-pressed="${state.cls?.id === cls.id}">
			<img src="${ICON(cls.icon)}" alt="" loading="lazy" />${cls.name}
		</button>`);
		btn.addEventListener('click', () => pickClass(cls));
		grid.append(btn);
	}
	if (state.cls) {
		const specs = step.querySelector('.fs-specs')!;
		for (const spec of SPECS.filter(s => s.classId === state.cls!.id)) {
			const label = spec.supported ? spec.role : 'coming soon';
			const btn = h(`<button type="button" class="fs-spec" aria-pressed="${state.spec?.key === spec.key}" ${spec.supported ? '' : 'disabled'}>
				${esc(spec.spec)}<small>${esc(label)}</small></button>`) as HTMLButtonElement;
			btn.addEventListener('click', () => pickSpec(spec));
			specs.append(btn);
		}
	}
	return step;
}

function renderCharacterStep(): HTMLElement {
	const locked = !state.spec;
	const step = h(`<section class="fs-step ${locked ? 'is-locked' : ''}" aria-labelledby="s2">
		<div class="fs-step-num" aria-hidden="true">2</div>
		<div>
			<h2 id="s2">Set up your character</h2>
			<div class="fs-choice" role="group" aria-label="Character source">
				<button type="button" class="fs-spec" data-src="preset" aria-pressed="${state.character?.source !== 'import'}">Pre-raid gear</button>
				<button type="button" class="fs-spec" data-src="import" aria-pressed="${state.character?.source === 'import'}">Paste from addon</button>
			</div>
			<div class="fs-char-body"></div>
		</div>
	</section>`);
	if (locked) return step;

	step.querySelectorAll<HTMLButtonElement>('[data-src]').forEach(b =>
		b.addEventListener('click', () => {
			if (b.dataset.src === 'preset') usePreset();
			else state.character = { ...state.character!, source: 'import' };
			render();
		}),
	);

	const body = step.querySelector('.fs-char-body')!;
	if (state.character?.source === 'import' && state.character.imported) {
		const summary = h(`<div>
			<p class="fs-help">${esc(state.character.note ?? '')}</p>
			<button type="button" class="fs-ghost">Paste a different export</button>
		</div>`);
		summary.querySelector('button')!.addEventListener('click', () => {
			state.character = { ...state.character!, imported: false };
			render();
		});
		body.append(summary, renderGearList());
		return step;
	}
	if (state.character?.source === 'import') {
		const box = h(`<div>
			<textarea aria-label="Addon export" placeholder="Paste the text from the WoWSims Exporter addon here"></textarea>
			<p class="fs-help">In game, type <strong>/wse export</strong>, copy the text, and paste it here. Gear, race and talents come from your character.</p>
			<p class="fs-error" hidden></p>
		</div>`);
		const ta = box.querySelector('textarea')!;
		const err = box.querySelector('.fs-error') as HTMLElement;
		ta.addEventListener('input', () => {
			if (!ta.value.trim()) return;
			try {
				importAddon(ta.value);
				render();
			} catch (e) {
				err.hidden = false;
				err.textContent = (e as Error).message;
			}
		});
		body.append(box);
		return step;
	}

	const row = h(`<div class="fs-row">
		<label class="fs-field">Race<select id="fs-race"></select></label>
	</div>`);
	const sel = row.querySelector('select')!;
	for (const r of state.cls!.races) {
		sel.append(h(`<option value="${r}" ${state.character?.race === r ? 'selected' : ''}>${RACE_LABEL[r]}</option>`));
	}
	sel.addEventListener('change', () => (state.character!.race = sel.value));
	body.append(row);
	if (state.character?.note) body.append(h(`<p class="fs-help">${esc(state.character.note)}</p>`));
	body.append(renderGearList());
	return step;
}

function renderGearList(): HTMLElement {
	const details = h(`<details class="fs-gear"><summary>See the gear (${esc(state.character?.source === 'import' ? 'from your addon export' : 'pre-raid dungeon and crafted gear')})</summary><ul></ul></details>`);
	const ul = details.querySelector('ul')!;
	details.addEventListener(
		'toggle',
		async () => {
			if (ul.childElementCount) return;
			const database = await db();
			state.character!.gear.items.forEach((it, i) => {
				if (!it.id) return;
				const item = database.getItemById(it.id);
				ul.append(h(`<li><span>${SLOT_NAMES[i] ?? ''}</span>${esc(item?.name ?? `Item ${it.id}`)}</li>`));
			});
		},
		{ once: false },
	);
	return details;
}

function renderSimStep(): HTMLElement {
	const ready = !!state.spec && !!state.character && !(state.character.source === 'import' && !state.character.imported);
	const step = h(`<section class="fs-step ${ready ? '' : 'is-locked'}" aria-labelledby="s3">
		<div class="fs-step-num" aria-hidden="true">3</div>
		<div>
			<h2 id="s3">Sim it</h2>
			<button type="button" class="fs-go" ${state.running ? 'disabled' : ''}>${state.running ? 'Simming…' : 'Sim my DPS'}</button>
			<div class="fs-progress" ${state.running ? '' : 'hidden'}><div></div></div>
		</div>
	</section>`);
	step.querySelector('.fs-go')!.addEventListener('click', runSim);
	return step;
}

// ---------------------------------------------------------------------------
// Actions

function pickClass(cls: ClassInfo) {
	state.cls = cls;
	setClassColor(cls);
	const specs = SPECS.filter(s => s.classId === cls.id && s.supported);
	state.spec = specs.length === 1 ? specs[0] : undefined;
	state.character = undefined;
	if (state.spec) usePreset();
	render();
}

function pickSpec(spec: ForeverSpec) {
	state.spec = spec;
	if (state.character?.source !== 'import') usePreset();
	render();
}

function usePreset() {
	const spec = state.spec!;
	const race = state.cls!.races.includes(spec.race) ? spec.race : state.cls!.races[0];
	state.character = { source: 'preset', gear: spec.gearSpec, talents: spec.talents, race, note: undefined };
}

function importAddon(text: string) {
	let data: any;
	try {
		data = JSON.parse(text);
	} catch {
		throw new Error("That doesn't look like an addon export. Copy the whole text from /wse export and paste it again.");
	}
	const classId = nameToClass(String(data.class ?? ''));
	const cls = CLASSES.find(c => c.id === classId);
	if (!cls) throw new Error('The export has no class. Make sure you copied the whole text.');
	const raceEnum = nameToRace(String(data.race ?? ''));
	const raceName = raceEnum ? Race[raceEnum] : undefined;
	const talents = String(data.talents ?? '');
	const trees = [0, 1, 2].map(i => [...(talents.split('-')[i] ?? '')].reduce((a, d) => a + Number(d || 0), 0));
	const tree = trees.indexOf(Math.max(...trees, 0));
	let specKey = TREE_TO_SPEC[cls.id][Math.max(tree, 0)];
	let spec = SPECS.find(s => s.key === specKey);
	if (!spec?.supported) spec = SPECS.find(s => s.classId === cls.id && s.supported);
	if (!spec) throw new Error(`${cls.name} sims aren't available yet.`);

	// Keep only the fields the sim understands; drop empty ones so the request stays valid JSON for the proto.
	const items = ((data.gear?.items ?? []) as Array<any>).map(it => {
		if (!it?.id) return {};
		return { id: Number(it.id), ...(it.enchant ? { enchant: Number(it.enchant) } : {}), ...(it.randomSuffix ? { randomSuffix: Number(it.randomSuffix) } : {}) };
	});
	const level = Number(data.level ?? 60);
	state.cls = cls;
	state.spec = spec;
	setClassColor(cls);
	state.character = {
		source: 'import',
		imported: true,
		gear: { items },
		talents: talents || spec.talents,
		race: raceName ?? spec.race,
		note: `Imported ${RACE_LABEL[raceName ?? spec.race]} ${cls.name}, talents ${trees.join('/')}.` + (level !== 60 ? ` Your character is level ${level}; the sim uses level 60.` : ''),
	};
}

// ---------------------------------------------------------------------------
// Sim

const signals = { abort: { onTrigger: () => {} } } as unknown as SimSignals;

// Blunt weapons take weightstones; blades take sharpening stones. (WeaponType: 3 fist, 4 mace, 8 staff)
const BLUNT = new Set([3, 4, 8]);

function weaponImbue(spec: ForeverSpec, weaponType: number | undefined): string | undefined {
	if (weaponType === undefined) return undefined;
	if (spec.classId === 7) return 'WindfuryWeapon';
	if (spec.role === 'Ranged DPS') return undefined;
	return BLUNT.has(weaponType) ? 'DenseWeightstone' : 'DenseSharpeningStone';
}

function consumesFor(spec: ForeverSpec, mainHandType?: number, offHandType?: number): Record<string, unknown> {
	if (spec.role === 'Caster DPS') {
		return { defaultPotion: 'MajorManaPotion', spellPowerBuff: 'GreaterArcaneElixir', mainHandImbue: 'BrilliantWizardOil', ...(spec.classId === 8 ? { defaultConjured: 'ConjuredDemonicRune' } : {}) };
	}
	const mh = weaponImbue(spec, mainHandType);
	const oh = weaponImbue(spec, offHandType);
	return {
		agilityElixir: 'ElixirOfTheMongoose',
		strengthBuff: 'ElixirOfGiants',
		...(mh ? { mainHandImbue: mh } : {}),
		...(oh && offHandType !== 7 && offHandType !== 5 ? { offHandImbue: oh === 'WindfuryWeapon' ? 'RockbiterWeapon' : oh } : {}),
	};
}

async function buildRequest(forever: boolean, iterations: number): Promise<RaidSimRequest> {
	const spec = state.spec!;
	const ch = state.character!;
	const cls = state.cls!;
	const physical = spec.role !== 'Caster DPS';
	const toCamel = (k: string) => k.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
	// The sim engine has no item list of its own: send the equipped items' stats with the request.
	const database = await db();
	const missing = ch.gear.items.filter(it => it.id && !database.getItemById(it.id)).map(it => it.id);
	if (missing.length) {
		throw new Error(`these items aren't in the Forever database yet: ${missing.join(', ')}. Remove them in the advanced sim, or use the pre-raid gear`);
	}
	const gear = database.lookupEquipmentSpec(EquipmentSpec.fromJson(ch.gear as any));
	const player = {
		name: 'You',
		race: ch.race,
		class: cls.enumName,
		level: 60,
		equipment: ch.gear,
		consumes: consumesFor(
			spec,
			ch.gear.items[14]?.id ? database.getItemById(ch.gear.items[14].id!)?.weaponType : undefined,
			ch.gear.items[15]?.id ? database.getItemById(ch.gear.items[15].id!)?.weaponType : undefined,
		),
		buffs: physical
			? { blessingOfKings: true, blessingOfMight: 'TristateEffectImproved' }
			: { blessingOfKings: true, blessingOfWisdom: 'TristateEffectImproved' },
		talentsString: ch.talents,
		rotation: spec.rotationJson,
		distanceFromTarget: spec.distance,
		reactionTimeMs: 150,
		channelClipDelayMs: 50,
		foreverRuleset: forever,
		[toCamel(spec.key)]: spec.specOptions,
		database: SimDatabase.toJson(gear.toDatabase()),
	};
	const target = (await db()).getPresetTarget('SoD/Level 60')?.target;
	return RaidSimRequest.fromJson({
		raid: {
			parties: [{ players: [player] }],
			buffs: {
				arcaneBrilliance: true,
				giftOfTheWild: 'TristateEffectImproved',
				powerWordFortitude: 'TristateEffectImproved',
				divineSpirit: true,
				...(physical ? { battleShout: 'TristateEffectImproved' } : {}),
			},
			debuffs: {},
			tanks: spec.role === 'Tank' ? [{ type: 'Player', index: 0 }] : [],
		},
		encounter: {
			duration: FIGHT_SECONDS,
			durationVariation: 15,
			executeProportion20: 0.2,
			executeProportion25: 0.25,
			executeProportion35: 0.35,
			targets: [target ? (target as any) : {}],
		} as any,
		simOptions: { iterations, randomSeed: String(Math.floor(Math.random() * 1e9)) },
	} as any);
}

async function runSim() {
	if (state.running || !state.spec || !state.character) return;
	state.running = true;
	render();
	const bar = app.querySelector<HTMLElement>('.fs-progress div')!;
	try {
		await db();
		const iterations = ITERATIONS;
		const onProgress = (p: { completedIterations: number; totalIterations: number }) => {
			if (p.totalIterations) bar.style.width = `${(50 * p.completedIterations) / p.totalIterations}%`;
		};
		const forever = await pool.raidSimAsync(await buildRequest(true, iterations), onProgress as any, signals);
		const vanilla = await pool.raidSimAsync(
			await buildRequest(false, iterations),
			((p: any) => p.totalIterations && (bar.style.width = `${50 + (50 * p.completedIterations) / p.totalIterations}%`)) as any,
			signals,
		);
		state.running = false;
		render();
		await showResults(document.getElementById('fs-results')!, forever, vanilla);
	} catch (e) {
		state.running = false;
		render();
		document.getElementById('fs-results')!.replaceChildren(h(`<p class="fs-error">The sim stopped: ${esc((e as Error).message ?? String(e))}. Try again, or open the advanced sim to check your setup.</p>`));
	}
}

async function showResults(el: HTMLElement, res: RaidSimResult, vanilla: RaidSimResult) {
	if (res.error) {
		el.replaceChildren(h(`<p class="fs-error">The sim stopped: ${esc(res.error.message)}</p>`));
		return;
	}
	const spec = state.spec!;
	const p = res.raidMetrics!.parties[0].players[0];
	const vp = vanilla.raidMetrics?.parties[0].players[0];
	const tank = spec.role === 'Tank';
	const main = tank ? p.threat!.avg : p.dps!.avg;
	const mainVanilla = tank ? vp?.threat?.avg ?? 0 : vp?.dps?.avg ?? 0;
	const delta = mainVanilla ? (100 * (main - mainVanilla)) / mainVanilla : 0;
	const stdev = tank ? p.threat!.stdev : p.dps!.stdev;

	const card = h(`<section class="fs-result" aria-live="polite">
		<div class="fs-result-head">
			<div class="fs-big">${Math.round(main).toLocaleString()}<small>${tank ? 'threat per second' : 'damage per second'}</small></div>
		</div>
		<p class="fs-sub">${esc(spec.spec)} ${esc(state.cls!.name)}${tank ? `, ${Math.round(p.dps!.avg)} DPS` : ''}. Most fights land between
			<strong>${Math.round(main - stdev)}</strong> and <strong>${Math.round(main + stdev)}</strong>.</p>
		<p class="fs-sub">WoW Forever's combat changes are worth <strong>${delta >= 0 ? '+' : ''}${delta.toFixed(1)}%</strong> for you compared with Classic rules.</p>
		<h3>Where your damage comes from</h3>
		<div class="fs-bars"></div>
		<h3>Which stat should you get next?</h3>
		<button type="button" class="fs-ghost">Find my best stat</button>
		<ul class="fs-stats"></ul>
	</section>`);
	el.replaceChildren(card);

	// Abilities grouped by their displayed name (so a DoT's direct and periodic parts
	// combine, while main-hand and off-hand swings stay separate), sorted by damage.
	const entries: Array<{ id: any; dmg: number; owner?: string }> = [];
	const addActions = (actions: typeof p.actions, owner?: string) => {
		for (const a of actions) {
			const dmg = a.targets.reduce((sum, t) => sum + t.damage, 0);
			if (dmg > 0) entries.push({ id: a.id, dmg, owner });
		}
	};
	addActions(p.actions);
	p.pets.forEach(pet => addActions(pet.actions, pet.name));

	const grouped = new Map<string, { name: string; icon?: string; dmg: number }>();
	for (const entry of entries) {
		// Names come from the local database; anything missing falls back to Wowhead, capped so a slow lookup never blocks results.
		const actionId = await Promise.race([
			ActionId.fromProto(entry.id).fill().catch(() => null),
			new Promise<null>(resolve => setTimeout(() => resolve(null), 1500)),
		]);
		const name = [entry.owner, actionId?.name].filter(Boolean).join(': ') || 'Other';
		const cur = grouped.get(name) ?? { name, icon: actionId?.iconUrl, dmg: 0 };
		cur.dmg += entry.dmg;
		grouped.set(name, cur);
	}
	const rows = [...grouped.values()].sort((a, b) => b.dmg - a.dmg).slice(0, 8);
	const top = rows[0]?.dmg ?? 1;
	const bars = card.querySelector('.fs-bars')!;
	for (const row of rows) {
		const dps = row.dmg / ITERATIONS / FIGHT_SECONDS;
		bars.append(
			h(`<div class="fs-bar">
				<img src="${row.icon ?? ICON('inv_misc_questionmark')}" alt="" loading="lazy" />
				<span class="fs-bar-name">${esc(row.name)}</span>
				<div class="fs-bar-track"><div class="fs-bar-fill" style="width:${((100 * row.dmg) / top).toFixed(1)}%"></div></div>
				<span class="fs-bar-val">${dps.toFixed(0)} dps</span>
			</div>`),
		);
	}

	const btn = card.querySelector<HTMLButtonElement>('.fs-ghost')!;
	btn.addEventListener('click', async () => {
		btn.disabled = true;
		btn.textContent = 'Testing each stat…';
		try {
			await showStatWeights(card.querySelector('.fs-stats')!);
			btn.remove();
		} catch (e) {
			btn.disabled = false;
			btn.textContent = 'Find my best stat';
			card.querySelector('.fs-stats')!.replaceChildren(h(`<li class="fs-error">Couldn't test stats: ${esc((e as Error).message)}</li>`));
		}
	});
}

type StatChoice = { stat: Stat; label: string };

function statChoices(spec: ForeverSpec): { ref: StatChoice; others: StatChoice[]; useTps: boolean } {
	if (spec.role === 'Caster DPS') {
		return {
			ref: { stat: Stat.StatSpellPower, label: 'Spell Power' },
			others: [
				{ stat: Stat.StatSpellHit, label: '1% Spell Hit' },
				{ stat: Stat.StatSpellCrit, label: '1% Spell Crit' },
				{ stat: Stat.StatIntellect, label: 'Intellect' },
				{ stat: Stat.StatSpirit, label: 'Spirit' },
				{ stat: Stat.StatMP5, label: 'Mana per 5 sec' },
			],
			useTps: false,
		};
	}
	const others: StatChoice[] = [
		{ stat: Stat.StatMeleeHit, label: '1% Hit' },
		{ stat: Stat.StatMeleeCrit, label: '1% Crit' },
		{ stat: Stat.StatAgility, label: 'Agility' },
		{ stat: Stat.StatStrength, label: 'Strength' },
	];
	if (spec.role === 'Ranged DPS') {
		return { ref: { stat: Stat.StatRangedAttackPower, label: 'Ranged Attack Power' }, others: others.filter(o => o.stat !== Stat.StatStrength), useTps: false };
	}
	return { ref: { stat: Stat.StatAttackPower, label: 'Attack Power' }, others, useTps: spec.role === 'Tank' };
}

async function showStatWeights(list: HTMLElement) {
	const spec = state.spec!;
	const base = await buildRequest(true, 2000);
	const { ref, others, useTps } = statChoices(spec);
	const req = StatWeightsRequest.create({
		player: base.raid!.parties[0].players[0],
		raidBuffs: base.raid!.buffs,
		partyBuffs: base.raid!.parties[0].buffs,
		debuffs: base.raid!.debuffs,
		encounter: base.encounter,
		simOptions: base.simOptions,
		tanks: base.raid!.tanks,
		statsToWeigh: [ref.stat, ...others.map(o => o.stat)],
		epReferenceStat: ref.stat,
	});
	const res = await pool.statWeightsAsync(req, () => {}, signals);
	const values = (useTps ? res.tps : res.dps)?.epValues?.stats ?? [];
	const ranked = others.map(o => ({ ...o, ep: values[o.stat] ?? 0 })).sort((a, b) => b.ep - a.ep);
	list.replaceChildren(
		...ranked.map(r =>
			h(`<li><span>${esc(r.label)}</span><span>${r.ep >= 0.5 ? `worth ${r.ep.toFixed(1)} ${esc(ref.label)}` : 'barely matters'}</span></li>`),
		),
	);
}

// ---------------------------------------------------------------------------

render();
db();
