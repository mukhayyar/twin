// Renders index.html frame by frame into out/twin-promo.mp4.
//   npm run render                   full film
//   node render.mjs --still 12.5     one frame to out/still.png (for checking)
//
// Expects media/ (see README in this folder): icons, demo-cli.mp4, and the
// app's scripted-demo frames in media/gui-light/ and media/gui-dark/.
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

const FPS = 30;
const here = path.dirname(new URL(import.meta.url).pathname);
// The app writes frames.json next to its frames: { frames, fps, ... }.
const sequence = (dir) => {
  try {
    const meta = JSON.parse(fs.readFileSync(path.join(here, 'media', dir, 'frames.json'), 'utf8'));
    return { count: meta.frames, fps: meta.fps };
  } catch { return { count: 0, fps: 30 }; }
};
const MEDIA = { gui: sequence('gui-light') };

const browser = await chromium.launch({ channel: 'chrome' }); // Chrome, not Chromium: it plays H.264
const page = await browser.newPage({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
await page.addInitScript((m) => { window.MEDIA = m; }, MEDIA);
await page.goto('file://' + path.join(here, 'index.html'));
await page.evaluate(() => window.ready);
fs.mkdirSync(path.join(here, 'out'), { recursive: true });

const still = process.argv.indexOf('--still');
if (still > 0) {
  for (const t of process.argv.slice(still + 1).map(Number)) {
    await page.evaluate((t) => window.seek(t), t);
    await page.screenshot({ path: path.join(here, 'out', `still-${t}.png`) });
  }
  await browser.close();
  process.exit(0);
}

const duration = await page.evaluate(() => window.duration);
const total = Math.round(duration * FPS);
const out = path.join(here, 'out', 'twin-promo.mp4');
const ffmpeg = spawn('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'image2pipe', '-framerate', String(FPS), '-i', '-',
  '-c:v', 'libx264', '-preset', 'slow', '-crf', '18', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', out],
  { stdio: ['pipe', 'inherit', 'inherit'] });

console.log(`gui: ${MEDIA.gui.count} frames @ ${MEDIA.gui.fps} fps · rendering ${total} frames`);
for (let i = 0; i < total; i++) {
  await page.evaluate((t) => window.seek(t), i / FPS);
  const png = await page.screenshot({ type: 'png' });
  if (!ffmpeg.stdin.write(png)) await new Promise(r => ffmpeg.stdin.once('drain', r));
  if (i % 150 === 0) console.log(`  ${i}/${total}`);
}
ffmpeg.stdin.end();
await new Promise(r => ffmpeg.on('close', r));
await browser.close();
console.log(`wrote ${out}`);
