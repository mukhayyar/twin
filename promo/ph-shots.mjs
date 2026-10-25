// Renders producthunt.html's slides to 1270x760 PNGs: node ph-shots.mjs <outdir>
import { chromium } from 'playwright';
import path from 'node:path';
const out = process.argv[2];
const slides = ['hero', 'app', 'icons', 'isolation', 'sidebyside', 'cli', 'install'];
const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1270, height: 760 }, deviceScaleFactor: 1 });
for (const [i, s] of slides.entries()) {
  await page.goto('file://' + path.resolve('producthunt.html') + '?s=' + s);
  await page.evaluate(() => Promise.all([...document.images].map(i => i.decode().catch(() => {}))));
  await page.screenshot({ path: path.join(out, `gallery-${i + 1}-${s}.png`) });
}
await browser.close();
