import { defineConfig, devices } from '@playwright/test'

// Two viewports and two colour schemes cover every defect these tests were
// written for. The four combinations are four projects rather than a loop
// inside each test, so a failure names the combination that broke.
const viewports = {
  desktop: { width: 1440, height: 900 },
  phone: { width: 390, height: 844 },
} as const

const schemes = ['light', 'dark'] as const

export default defineConfig({
  testDir: '.',
  testMatch: /.*\.spec\.ts$/,
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  // The server is shared by every project, so nothing may mutate it.
  // These tests only read geometry and computed style.
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  expect: { timeout: 5_000 },
  timeout: 60_000,
  use: {
    ...devices['Desktop Chrome'],
    // Loopback only: sbnn is never started with
    // --dangerously-allow-remote-access.
    baseURL: undefined,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: Object.entries(viewports).flatMap(([vp, viewport]) =>
    schemes.map((colorScheme) => ({
      name: `${vp}-${colorScheme}`,
      use: { ...devices['Desktop Chrome'], viewport, colorScheme },
    })),
  ),
})
