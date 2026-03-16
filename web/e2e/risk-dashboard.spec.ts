import { test, expect } from '@playwright/test'

test.describe('Risk Dashboard', () => {
  test('admin can view risk dashboard with metric sections', async ({ page }) => {
    // Navigate as admin (default user is operator-alpha, which should get 403)
    // Use the admin-accessible route
    await page.goto('/admin/risk')
    await expect(page.locator('h1')).toContainText('Risk Dashboard')

    // Wait for data to load (may show loading first)
    await page.waitForTimeout(2000)

    // Check all metric sections are present
    await expect(page.locator('text=Rejection Rate')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Float Exposure')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Return Rate')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Top Investors')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Avg Processing')).toBeVisible({ timeout: 5000 })
  })

  test('risk dashboard has navigation link in sidebar', async ({ page }) => {
    await page.goto('/admin/flow')
    await expect(page.locator('a[href="/admin/risk"]')).toBeVisible()
    await expect(page.locator('a[href="/admin/risk"]')).toContainText('Risk')
  })
})
