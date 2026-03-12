import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  retries: 1,
  use: {
    baseURL: 'http://localhost:5173',
    headless: true,
    screenshot: 'only-on-failure',
  },
  reporter: [
    ['html', { outputFolder: '../reports/playwright' }],
    ['list'],
  ],
  webServer: {
    command: 'echo "using existing dev server"',
    url: 'http://localhost:5173',
    reuseExistingServer: true,
    timeout: 5000,
  },
})
