// Screenshots a page with a transparent background: node shot.mjs <page.html> <out.png>
import { chromium } from 'playwright';
import path from 'node:path';
const [page, out] = process.argv.slice(2);
const browser = await chromium.launch({ channel: 'chrome' });
const p = await browser.newPage({ viewport: { width: 1600, height: 400 }, deviceScaleFactor: 1 });
await p.goto('file://' + path.resolve(page));
await p.evaluate(() => Promise.all([...document.images].map(i => i.decode())));
await p.locator('body').screenshot({ path: out, omitBackground: true });
await browser.close();
