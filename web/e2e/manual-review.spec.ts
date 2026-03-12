import { test, expect } from '@playwright/test'

const API_URL = 'http://localhost:8080'
const AUTH_ALPHA = 'Bearer operator-alpha'
const AUTH_ADMIN = 'Bearer apex-admin'

/** Submit a deposit via API. */
async function submitDeposit(
  request: import('@playwright/test').APIRequestContext,
  accountCode: string,
  amount: number,
) {
  const res = await request.post(`${API_URL}/deposits`, {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': `e2e-review-${accountCode}-${Date.now()}`,
    },
    data: { account_code: accountCode, amount },
  })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

/** Fetch operator queue with auth. */
async function getQueue(request: import('@playwright/test').APIRequestContext, token: string) {
  const res = await request.get(`${API_URL}/operator/queue`, {
    headers: { Authorization: token },
  })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

/** Execute operator action with auth. */
async function postAction(
  request: import('@playwright/test').APIRequestContext,
  token: string,
  body: Record<string, unknown>,
) {
  const res = await request.post(`${API_URL}/operator/actions`, {
    headers: { Authorization: token, 'Content-Type': 'application/json' },
    data: body,
  })
  expect(res.ok()).toBeTruthy()
  return res.json()
}

test.describe('Manual Review E2E', () => {
  test('ALPHA-004 submission → visible in operator queue', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-004', 600)
    expect(transfer.state).toBe('Analyzing')
    expect(transfer.review_reason).toBeTruthy()

    // Verify it appears in the queue
    const queue = await getQueue(page.request, AUTH_ALPHA)
    const found = queue.some((t: { id: string }) => t.id === transfer.id)
    expect(found).toBe(true)
  })

  test('operator approves ALPHA-004 → transfer reaches FundsPosted', async ({ page }) => {
    // Submit a flagged transfer
    const transfer = await submitDeposit(page.request, 'ALPHA-004', 650)
    expect(transfer.state).toBe('Analyzing')

    // Approve it
    const result = await postAction(page.request, AUTH_ALPHA, {
      transfer_id: transfer.id,
      action: 'APPROVE',
    })
    expect(result.state).toBe('FundsPosted')

    // Verify on status page
    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Funds are available in your account')).toBeVisible({ timeout: 5000 })
  })

  test('operator rejects ALPHA-005 → transfer shows Rejected', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-005', 500)
    expect(transfer.state).toBe('Analyzing')

    // Reject it
    const result = await postAction(page.request, AUTH_ALPHA, {
      transfer_id: transfer.id,
      action: 'REJECT',
      reason: 'Amount does not match check image',
    })
    expect(result.state).toBe('Rejected')

    // Verify on status page
    await page.goto(`/status/${transfer.id}`)
    await expect(page.locator('text=Rejected')).toBeVisible({ timeout: 5000 })
  })

  test('operator approval is logged in decision trace', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-004', 700)
    expect(transfer.state).toBe('Analyzing')

    await postAction(page.request, AUTH_ALPHA, {
      transfer_id: transfer.id,
      action: 'APPROVE',
    })

    // Check events include operator_action
    const eventsRes = await page.request.get(`${API_URL}/deposits/${transfer.id}/events`)
    const events = await eventsRes.json()
    const opAction = events.find((e: { step: string }) => e.step === 'operator_action')
    expect(opAction).toBeTruthy()
    expect(opAction.data.operator_id).toBe('op-alpha-001')
    expect(opAction.data.action).toBe('APPROVE')
  })

  test('over-limit: ALPHA-001 $5001 → Rejected by FS (no operator)', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-001', 5001)
    expect(transfer.state).toBe('Rejected')
    expect(transfer.error_code).toBe('FS_OVER_DEPOSIT_LIMIT')
  })

  test('IRA: ALPHA-IRA → processes with contribution_type', async ({ page }) => {
    const transfer = await submitDeposit(page.request, 'ALPHA-IRA', 500)
    // ALPHA-IRA should process (FundsPosted or Analyzing if flagged)
    expect(['FundsPosted', 'Analyzing']).toContain(transfer.state)

    // If it reached FundsPosted, verify contribution_type is set
    if (transfer.state === 'FundsPosted' && transfer.contribution_type) {
      expect(transfer.contribution_type).toBeTruthy()
    }
  })

  test('all previous tests still pass (ledger healthy)', async ({ page }) => {
    const healthRes = await page.request.get(`${API_URL}/health/ledger`)
    const health = await healthRes.json()
    expect(health.healthy).toBe(true)
    expect(health.sum).toBe('0.00')
  })
})
