import { expect, test } from '@playwright/test';

test.describe('critical UI flows', () => {
  test('home search shows client-side validation for invalid query', async ({ page }) => {
    await page.goto('/', { waitUntil: 'load' });
    const searchInput = page.locator('#search-input');
    await expect(searchInput).toBeVisible({ timeout: 30_000 });
    await searchInput.fill('abc');
    await page.locator('#search-icon').click();
    await expect(page.locator('#result-container')).toContainText(
      /Invalid search format|Formato de búsqueda no válido|تنسيق بحث غير صالح/i,
      { timeout: 15_000 },
    );
  });

  test('login as dev admin then open portfolios section', async ({ page }) => {
    await page.goto('/', { waitUntil: 'load' });
    await expect(page.locator('#login-btn')).toBeVisible({ timeout: 30_000 });
    await page.locator('#login-btn').click();
    await expect(page.locator('#login-form')).toBeVisible();
    await page.locator('#login-username').fill('admin');
    await page.locator('#login-password').fill('admin123');
    await Promise.all([
      page.waitForResponse((r) => r.url().includes('/api/v1/login') && r.ok()),
      page.locator('#login-form').locator('button[type="submit"]').click(),
    ]);
    await expect(page.locator('#user-info')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#username-display')).toContainText('admin');
    await expect(page.locator('#portfolios-nav')).toBeVisible({ timeout: 15_000 });
    await page.locator('#portfolios-nav').click();
    await expect(page.locator('#portfolio-section')).toBeVisible();
    await expect(page.getByRole('heading', { name: /Your Portfolios/i })).toBeVisible();
    await expect(page.locator('#create-portfolio-btn')).toBeVisible();
  });
});
