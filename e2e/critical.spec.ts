import { expect, test } from '@playwright/test';

test.describe('critical UI flows', () => {
  test('home search shows client-side validation for invalid query', async ({ page }) => {
    await page.goto('/');
    await page.locator('#search-input').fill('abc');
    await page.locator('#search-icon').click();
    await expect(page.locator('#result-container')).toContainText(/Invalid search format/i);
  });

  test('login as dev admin then open portfolios section', async ({ page }) => {
    await page.goto('/');
    await page.locator('#login-btn').click();
    await expect(page.locator('#login-form')).toBeVisible();
    await page.locator('#login-username').fill('admin');
    await page.locator('#login-password').fill('admin123');
    await page.locator('#login-form').locator('button[type="submit"]').click();
    await expect(page.locator('#user-info')).toBeVisible({ timeout: 30_000 });
    await expect(page.locator('#username-display')).toContainText('admin');
    await page.locator('#portfolios-nav').click();
    await expect(page.locator('#portfolio-section')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Your Portfolios' })).toBeVisible();
    // Empty portfolio grid can have zero height; assert a control that is always present.
    await expect(page.locator('#create-portfolio-btn')).toBeVisible();
  });
});
