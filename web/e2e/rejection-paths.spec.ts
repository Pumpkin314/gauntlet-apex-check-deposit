import { test, expect } from '@playwright/test'

const API_URL = 'http://localhost:8080'

/**
 * Submit a deposit via API and return the response JSON.
 * Uses account_code to trigger scenario-specific VSS responses.
 */
async function submitDeposit(
  request: import('@playwright/test').APIRequestContext,
  accountCode: string,
  amount: number,
  scenario?: string,
) {
  const res = await request.post(`${API_URL}/deposits`, {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': `e2e-${accountCode}-${Date.now()}`,
    },
    data: { account_code: accountCode, amount, ...(scenario ? { scenario } : {}) },
  })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

test.describe('Rejection Paths E2E', () => {
  test('ALPHA-002 → blur error message displayed', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 500, 'iqa_fail_blur')
    expect(transfer.state).toBe('Rejected')
    expect(transfer.error_code).toBe('VSS_IQA_BLUR')

    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Deposit not accepted')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Image too blurry')).toBeVisible({ timeout: 5000 })
  })

  test('ALPHA-003 → glare error message displayed', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 500, 'iqa_fail_glare')
    expect(transfer.state).toBe('Rejected')
    expect(transfer.error_code).toBe('VSS_IQA_GLARE')

    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Deposit not accepted')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Glare detected')).toBeVisible({ timeout: 5000 })
  })

  test('BETA-001 → duplicate error displayed', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'BETA-001', 500, 'duplicate_detected')
    expect(transfer.state).toBe('Rejected')
    expect(transfer.error_code).toBe('VSS_DUPLICATE_DETECTED')

    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Deposit not accepted')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=already been deposited')).toBeVisible({ timeout: 5000 })
  })

  test('over-limit $5001 → FS rejection displayed', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 5001, 'clean_pass')
    expect(transfer.state).toBe('Rejected')
    expect(transfer.error_code).toBe('FS_OVER_DEPOSIT_LIMIT')

    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Deposit not accepted')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=Maximum single deposit')).toBeVisible({ timeout: 5000 })
  })

  test('rejected transfers produce no ledger entries', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 500, 'iqa_fail_blur')
    expect(transfer.state).toBe('Rejected')

    // Check events — no ledger_posted
    const eventsRes = await page.request.get(`${API_URL}/deposits/${transfer.id}/events`)
    const events = await eventsRes.json()
    const hasLedger = events.some((e: { step: string }) => e.step === 'ledger_posted')
    expect(hasLedger).toBe(false)

    // Ledger health still clean
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')
  })
})
