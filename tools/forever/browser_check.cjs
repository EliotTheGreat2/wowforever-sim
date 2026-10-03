// Cross-browser smoke test for the WoW Forever site: loads the Quick Sim page in Chromium,
// Firefox and WebKit (Safari's engine), plus phone-sized Chrome and Safari, and runs one sim
// in each. Used by .github/workflows/forever_pages.yml before publishing.
//
//   node tools/forever/browser_check.cjs http://localhost:8000/<repo>/forever/
const { chromium, firefox, webkit, devices } = require('playwright');

const url = process.argv[2];
const targets = [
	{ name: 'Chrome / Edge / Opera (Chromium)', type: chromium, options: {} },
	{ name: 'Firefox', type: firefox, options: {} },
	{ name: 'Safari (WebKit)', type: webkit, options: {} },
	{ name: 'iPhone Safari', type: webkit, options: devices['iPhone 13'] },
	{ name: 'Android Chrome', type: chromium, options: devices['Pixel 7'] },
];

(async () => {
	let failed = 0;
	for (const t of targets) {
		const browser = await t.type.launch();
		const page = await browser.newPage(t.options);
		const errors = [];
		page.on('pageerror', e => errors.push(e.message));
		try {
			await page.goto(url);
			await page.locator('button.fs-class', { hasText: 'Mage' }).click({ timeout: 30000 });
			await page.locator('button.fs-spec:not([disabled]):not([data-src])').first().click();
			await page.locator('button.fs-go').click();
			await page.waitForFunction(
				() => /damage per second|threat per second/i.test(document.querySelector('#fs-results')?.textContent || ''),
				null,
				{ timeout: 180000 },
			);
			const result = (await page.locator('#fs-results').innerText()).replace(/\s+/g, ' ').slice(0, 40);
			if (errors.length) throw new Error('page errors: ' + errors.join(' | '));
			console.log(`::notice title=${t.name}::OK - ${result}`);
		} catch (e) {
			failed++;
			console.log(`::error title=${t.name}::${String(e.message || e).split('\n')[0]} ${errors.join(' | ')}`);
		}
		await browser.close();
	}
	process.exit(failed ? 1 : 0);
})();
