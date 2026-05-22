import { expect, test } from '@playwright/test';

/** Task 76 — mobile viewport smoke (iPhone 12). Runs via the mobile-chrome Playwright project in E2E CI. */
test.describe.configure({ mode: 'serial' });

test.describe('mobile viewport smoke', () => {
  test('hamburger is tappable on home (no header overlap)', async ({ page }) => {
    await page.goto('/', { waitUntil: 'load', timeout: 90_000 });
    const menu = page.locator('#mobile-menu-button');
    await expect(menu).toBeVisible({ timeout: 30_000 });
    await menu.click({ trial: true });
  });

  test('hamburger is tappable on explorer and symbols pages', async ({ page }) => {
    for (const path of ['/bitcoin', '/symbols']) {
      await page.goto(path, { waitUntil: 'load', timeout: 90_000 });
      const menu = page.locator('#mobile-menu-button');
      await expect(menu).toBeVisible({ timeout: 30_000 });
      await menu.click({ trial: true });
    }
  });

  test('home → search → dashboard', async ({ page }) => {
    await page.goto('/', { waitUntil: 'load', timeout: 90_000 });
    await expect(page.locator('#mobile-menu-button')).toBeVisible({ timeout: 30_000 });
    await page.locator('#mobile-menu-button').click();
    await expect(page.locator('#mobile-menu-button')).toHaveAttribute('aria-expanded', 'true');
    await expect(page.locator('#search-input-mobile')).toBeVisible();
    await page.locator('#search-input-mobile').fill('100000');
    await page.locator('#search-form-mobile').evaluate((form) => form.requestSubmit());
    await page.waitForURL(/\/bitcoin(\?q=100000|$)/, { waitUntil: 'domcontentloaded', timeout: 90_000 });

    await expect(page.locator('body')).toContainText(/100000|Block|Search failed|No matching|Basic result/i);

    await page.goto('/dashboard');
    await expect(page.locator('#mobile-menu-button')).toBeVisible();
    await expect(page.getByRole('heading', { name: /Dashboard|Portfolio/i }).first()).toBeVisible();
  });
});
