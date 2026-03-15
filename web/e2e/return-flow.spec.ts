import { test, expect } from '@playwright/test'
import { execSync } from 'child_process'

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
      'Idempotency-Key': `e2e-return-${accountCode}-${Date.now()}`,
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

/** Simulate a return via the admin endpoint (proxies through settlement stub). */
async function simulateReturn(
  request: import('@playwright/test').APIRequestContext,
  transferId: string,
  reasonCode = 'R01',
) {
  const res = await request.post(`${API_URL}/admin/simulate-return`, {
    headers: { 'Content-Type': 'application/json' },
    data: { transfer_id: transferId, reason_code: reasonCode },
  })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

test.describe('Return Flow E2E', () => {
  test.beforeEach(async () => {
    // Reset ALPHA-001 to ACTIVE (may be COLLECTIONS from a prior return test)
    execSync(
      `docker compose exec -T postgres psql -U apex -d apex_check_deposit -c "UPDATE accounts SET status = 'ACTIVE' WHERE code = 'ALPHA-001';"`,
      { cwd: '..', stdio: 'ignore' },
    )
  })

  test('full return flow: submit → settle → return → status shows Deposit reversed', async ({ page }) => {
    // Submit a clean deposit
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 500)
    expect(transfer.state).toBe('FundsPosted')

    // Trigger settlement → Completed
    await triggerSettlement(page.request)
    const settled = await (await page.request.get(`${API_URL}/deposits/${transfer.id}`)).json()
    expect(settled.state).toBe('Completed')

    // Trigger return → Returned
    const returned = await simulateReturn(page.request, transfer.id)
    expect(returned.state).toBe('Returned')

    // Verify status page shows "Deposit reversed"
    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Deposit reversed')).toBeVisible({ timeout: 10000 })
    await expect(page.locator('text=$30.00 returned check fee')).toBeVisible()
  })

  test('/admin/ledger shows 6 entries for returned transfer, reconciliation = 0', async ({ page }) => {
    // Submit, settle, return
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 300)
    expect(transfer.state).toBe('FundsPosted')
    await triggerSettlement(page.request)
    await simulateReturn(page.request, transfer.id)

    // Check ledger entries via API
    const entriesRes = await page.request.get(`${API_URL}/ledger/entries?transfer_id=${transfer.id}`)
    expect(entriesRes.ok()).toBeTruthy()
    const entries = await entriesRes.json()
    expect(entries.length).toBe(6) // 2 deposit + 2 reversal + 2 fee

    // Check reconciliation via API
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')

    // Verify on admin ledger page
    await page.goto('/admin/ledger')
    await expect(page.locator('text=Reconciliation')).toBeVisible({ timeout: 5000 })
    await expect(page.getByText('Reconciliation: $')).toBeVisible()
  })

  test('return from FundsPosted (pre-settlement return)', async ({ page }) => {
    // Submit deposit — do NOT trigger settlement
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 200)
    expect(transfer.state).toBe('FundsPosted')

    // Return directly from FundsPosted
    const returned = await simulateReturn(page.request, transfer.id, 'R08')
    expect(returned.state).toBe('Returned')

    // Verify ledger entries
    const entriesRes = await page.request.get(`${API_URL}/ledger/entries?transfer_id=${transfer.id}`)
    const entries = await entriesRes.json()
    expect(entries.length).toBe(6)

    // Verify reconciliation
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')

    // Status page shows Returned
    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Deposit reversed')).toBeVisible({ timeout: 10000 })
  })

  test('/admin/flow shows Returned state in dashboard', async ({ page }) => {
    // Submit and settle
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 150)
    expect(transfer.state).toBe('FundsPosted')
    await triggerSettlement(page.request)

    // Navigate to flow page before triggering return
    await page.goto('/admin/flow')
    await expect(page.locator('h1')).toContainText('Flow Dashboard')

    // Trigger return
    await simulateReturn(page.request, transfer.id)

    // Verify the transfer is Returned via API
    const getRes = await page.request.get(`${API_URL}/deposits/${transfer.id}`)
    const updated = await getRes.json()
    expect(updated.state).toBe('Returned')

    // Switch filter to "All States" to see Returned transfers
    await page.locator('select').last().selectOption('Returned')
    await page.waitForTimeout(2000)

    // The transfer card should show Returned
    await expect(page.getByText('Returned').first()).toBeVisible({ timeout: 5000 })
  })

  test('notification appears for investor after return', async ({ page }) => {
    // Submit, settle, return
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 400)
    expect(transfer.state).toBe('FundsPosted')
    await triggerSettlement(page.request)
    await simulateReturn(page.request, transfer.id)

    // Check notification via API (investor-alpha token maps to ALPHA-001 account)
    const notifsRes = await page.request.get(`${API_URL}/notifications`, {
      headers: { Authorization: 'Bearer investor-alpha' },
    })
    expect(notifsRes.ok()).toBeTruthy()
    const notifs = await notifsRes.json()
    const returnNotifs = notifs.filter(
      (n: { type: string }) => n.type === 'RETURN_RECEIVED',
    )
    expect(returnNotifs.length).toBeGreaterThanOrEqual(1)
  })

  test('return events appear in audit trail', async ({ page }) => {
    // Submit, settle, return
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 350)
    expect(transfer.state).toBe('FundsPosted')
    await triggerSettlement(page.request)
    await simulateReturn(page.request, transfer.id)

    // Check events
    const eventsRes = await page.request.get(`${API_URL}/deposits/${transfer.id}/events`)
    const events = await eventsRes.json()

    const returnReceived = events.find(
      (e: { step: string }) => e.step === 'return_received',
    )
    expect(returnReceived).toBeTruthy()

    const returnProcessed = events.find(
      (e: { step: string }) => e.step === 'return_processed',
    )
    expect(returnProcessed).toBeTruthy()
  })

  test('all previous tests still pass: ledger healthy after return operations', async ({ page }) => {
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')
  })
})
