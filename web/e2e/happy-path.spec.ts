import { test, expect } from '@playwright/test'

const API_URL = 'http://localhost:8080'

test.describe('Happy Path E2E', () => {
  test('submit deposit → status shows FundsPosted', async ({ page }) => {
    // Login as Alice Johnson (ALPHA-001)
    await page.goto('/login')
    await expect(page.locator('h1')).toContainText('Investor Login')

    const accountSelect = page.locator('#account-select')
    await expect(accountSelect).toBeVisible()
    await accountSelect.selectOption('ALPHA-001')
    await page.locator('button[type="submit"]').click()

    // Should redirect to dashboard
    await page.waitForURL(/\/dashboard/)
    await expect(page.locator('text=Deposit a Check')).toBeVisible({ timeout: 5000 })

    // Navigate to deposit page
    await page.goto('/deposit')
    await expect(page.locator('h1')).toContainText('Deposit Check')

    // Select Clean Pass scenario
    const scenarioSelect = page.locator('#scenario')
    await expect(scenarioSelect).toBeVisible()
    await scenarioSelect.selectOption('clean_pass')

    // Enter amount
    const amountInput = page.locator('#amount')
    await amountInput.fill('500')

    // Submit
    await page.locator('button[type="submit"]').click()

    // Should redirect to /status/:id
    await page.waitForURL(/\/status\//)

    // Status page should show FundsPosted state
    await expect(page.locator('text=Funds available')).toBeVisible({ timeout: 10000 })
  })

  test('/admin/ledger shows correct balances', async ({ page }) => {
    await page.goto('/admin/ledger')
    await expect(page.locator('h1')).toContainText('Ledger')

    // Wait for data to load
    await expect(page.locator('table')).toBeVisible({ timeout: 5000 })

    // Check ALPHA-001 has a balance
    await expect(page.locator('text=ALPHA-001')).toBeVisible()

    // Check reconciliation indicator shows healthy
    await expect(page.locator('text=Reconciliation')).toBeVisible()
    // The checkmark character should be visible (healthy state)
    await expect(page.locator('text=$0.00')).toBeVisible()
  })

  test('/admin/flow shows transfer in live event stream', async ({ page }) => {
    await page.goto('/admin/flow')
    await expect(page.locator('h1')).toContainText('Flow Dashboard')

    // Submit a deposit via API to trigger events
    const idempKey = `e2e-flow-${Date.now()}`
    const res = await page.request.post(`${API_URL}/deposits`, {
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': idempKey,
      },
      data: { account_code: 'ALPHA-001', amount: 100, scenario: 'clean_pass' },
    })
    expect(res.ok()).toBeTruthy()

    // Wait for events to appear in the event stream
    await expect(page.locator('text=Event Stream')).toBeVisible()

    // Transfer card or event should appear (state_changed events)
    // Give SSE time to deliver
    await page.waitForTimeout(2000)

    // The event stream section should have content
    const eventLog = page.locator('text=FundsPosted')
    await expect(eventLog).toBeVisible({ timeout: 5000 })
  })
})
