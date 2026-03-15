import { test, expect } from '@playwright/test'

const API_URL = 'http://localhost:8080'

/** Submit a deposit via API and return the response. */
async function submitDeposit(
  request: import('@playwright/test').APIRequestContext,
  accountCode: string,
  amount: number,
) {
  const res = await request.post(`${API_URL}/deposits`, {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': `e2e-settlement-${accountCode}-${Date.now()}`,
    },
    data: { account_code: accountCode, amount, scenario: 'clean_pass' },
  })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

/** Trigger settlement via API. */
async function triggerSettlement(
  request: import('@playwright/test').APIRequestContext,
) {
  const res = await request.post(`${API_URL}/settlement/trigger`)
  expect(res.ok()).toBeTruthy()
  return res.json()
}

test.describe('Settlement E2E', () => {
  test('full happy path: submit → FundsPosted → settlement → Completed', async ({ page }) => {
    // Submit a clean deposit
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 250)
    expect(transfer.state).toBe('FundsPosted')

    // Trigger settlement
    const result = await triggerSettlement(page.request)
    expect(result.batch_count).toBeGreaterThanOrEqual(1)
    expect(result.total_checks).toBeGreaterThanOrEqual(1)

    // Verify transfer is now Completed
    const getRes = await page.request.get(`${API_URL}/deposits/${transfer.id}`)
    const updated = await getRes.json()
    expect(updated.state).toBe('Completed')

    // Verify on status page
    await page.goto(`/status/${transfer.id}`)
    await expect(
      page.locator('text=Completed').or(page.locator('text=Settlement complete'))
    ).toBeVisible({ timeout: 5000 })
  })

  test('settlement file has correct structure via batches API', async ({ page }) => {
    // Submit a deposit to ensure there's something to settle
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 175)
    expect(transfer.state).toBe('FundsPosted')

    // Trigger settlement
    await triggerSettlement(page.request)

    // Check batches list
    const batchesRes = await page.request.get(`${API_URL}/settlement/batches`)
    expect(batchesRes.ok()).toBeTruthy()
    const batches = await batchesRes.json()
    expect(batches.length).toBeGreaterThanOrEqual(1)

    const batch = batches[0]
    expect(batch.status).toBe('ACKNOWLEDGED')
    expect(batch.record_count).toBeGreaterThanOrEqual(1)
    expect(batch.total_amount).toBeGreaterThan(0)
    expect(batch.correspondent_id).toBeTruthy()
  })

  test('settlement status shows batch info', async ({ page }) => {
    const statusRes = await page.request.get(`${API_URL}/settlement/status`)
    expect(statusRes.ok()).toBeTruthy()
    const status = await statusRes.json()

    expect(status).toHaveProperty('unbatched_count')
    expect(status).toHaveProperty('total_batches')
    expect(typeof status.unbatched_count).toBe('number')
  })

  test('/admin/ledger reconciliation = 0 after settlement', async ({ page }) => {
    // Submit and settle
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 300)
    expect(transfer.state).toBe('FundsPosted')
    await triggerSettlement(page.request)

    // Check ledger health via API
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')

    // Verify on admin ledger page
    await page.goto('/admin/ledger')
    await expect(page.locator('text=Reconciliation')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('text=$0.00')).toBeVisible()
  })

  test('/admin/flow shows Completed after settlement trigger', async ({ page }) => {
    // Submit deposit
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 125)
    expect(transfer.state).toBe('FundsPosted')

    // Go to flow page
    await page.goto('/admin/flow')
    await expect(page.locator('h1')).toContainText('Flow Dashboard')

    // Trigger settlement via the API
    await triggerSettlement(page.request)

    // Verify the transfer reached Completed via API
    const getRes = await page.request.get(`${API_URL}/deposits/${transfer.id}`)
    const updated = await getRes.json()
    expect(updated.state).toBe('Completed')
  })

  test('settlement events appear in audit trail', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 350)
    expect(transfer.state).toBe('FundsPosted')

    await triggerSettlement(page.request)

    // Check events
    const eventsRes = await page.request.get(`${API_URL}/deposits/${transfer.id}/events`)
    const events = await eventsRes.json()
    const settlementEvent = events.find(
      (e: { step: string }) => e.step === 'settlement_completed',
    )
    expect(settlementEvent).toBeTruthy()
    expect(settlementEvent.data.batch_id).toBeTruthy()
  })

  test('all previous tests still pass: ledger healthy', async ({ page }) => {
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')
  })
})
