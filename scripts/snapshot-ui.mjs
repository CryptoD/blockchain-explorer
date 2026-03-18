import fs from 'node:fs/promises';
import path from 'node:path';
import { chromium, request as pwRequest } from 'playwright';

const BASE = process.env.UI_BASE_URL || 'http://localhost:8080';
const OUT_DIR = process.env.UI_SNAPSHOT_DIR || path.resolve('ui-snapshots');

async function ensureDir(p) {
  await fs.mkdir(p, { recursive: true });
}

function safeName(s) {
  return s.replace(/[^a-zA-Z0-9._-]+/g, '_');
}

async function registerAndLogin() {
  const api = await pwRequest.newContext({ baseURL: BASE });
  // Use the dev admin account for screenshots to avoid triggering registration rate limits.
  const username = process.env.UI_LOGIN_USERNAME || 'admin';
  const password = process.env.UI_LOGIN_PASSWORD || 'admin123';

  const login = await api.post('/api/v1/login', { data: { username, password } });
  if (!login.ok()) throw new Error(`login failed: ${login.status()}`);

  const cookies = await api.storageState();
  await api.dispose();
  return { username, cookies };
}

async function screenshotPage(page, urlPath, filename, actions) {
  const url = urlPath.startsWith('http') ? urlPath : `${BASE}${urlPath}`;
  await page.goto(url, { waitUntil: 'networkidle' });
  if (actions) await actions(page);
  await page.waitForTimeout(500);
  await page.screenshot({ path: path.join(OUT_DIR, filename), fullPage: true });
}

async function main() {
  await ensureDir(OUT_DIR);

  const browser = await chromium.launch();

  // Public screenshots
  {
    const context = await browser.newContext();
    const page = await context.newPage();
    await screenshotPage(page, '/', '01-home.png');
    await screenshotPage(page, '/symbols', '02-symbols.png');
    await screenshotPage(page, '/dashboard', '03-dashboard-signed-out.png');
    await screenshotPage(page, '/profile', '04-profile-signed-out.png');
    await context.close();
  }

  // Authenticated screenshots
  const { username, cookies } = await registerAndLogin();
  {
    const context = await browser.newContext({ storageState: cookies });
    const page = await context.newPage();
    await screenshotPage(page, '/dashboard', '05-dashboard-signed-in.png', async (p) => {
      // open notification center if present
      const btn = await p.$('#notif-btn');
      if (btn) await btn.click();
    });
    await screenshotPage(page, '/profile', '06-profile-signed-in.png');
    await screenshotPage(page, '/symbols', '07-symbols-news-panel.png', async (p) => {
      // Try to click first row to open news panel
      const row = await p.$('tbody#results-body tr[data-symbol]');
      if (row) await row.click();
    });
    await context.close();
  }

  await browser.close();

  console.log(`Saved UI screenshots to ${OUT_DIR}`);
  console.log(`Authenticated user used: ${username}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});

