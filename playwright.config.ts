import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['line'], ['html', { open: 'never' }]] : 'list',
  timeout: 60_000,
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:8080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    {
      name: 'mobile-chrome',
      use: {
        browserName: 'chromium',
        viewport: devices['iPhone 12'].viewport,
        userAgent: devices['iPhone 12'].userAgent,
        deviceScaleFactor: devices['iPhone 12'].deviceScaleFactor,
        isMobile: devices['iPhone 12'].isMobile,
        hasTouch: devices['iPhone 12'].hasTouch,
      },
      testMatch: /mobile\.spec\.ts/,
      timeout: 90_000,
    },
  ],
});
