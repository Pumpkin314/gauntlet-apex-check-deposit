import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '../api/client'
import DepositError from '../components/DepositError'

// Demo scenarios mapped to seed account codes (from db/seed.sql)
const SCENARIOS = [
  {
    key: 'clean_pass',
    label: 'Alpha — Clean Pass (ALPHA-001)',
    accountCode: 'ALPHA-001',
    scenarioName: 'clean_pass',
  },
  {
    key: 'iqa_fail_blur',
    label: 'Alpha — IQA Blur (ALPHA-002)',
    accountCode: 'ALPHA-002',
    scenarioName: 'iqa_fail_blur',
  },
  {
    key: 'iqa_fail_glare',
    label: 'Alpha — IQA Glare (ALPHA-003)',
    accountCode: 'ALPHA-003',
    scenarioName: 'iqa_fail_glare',
  },
  {
    key: 'duplicate_detected',
    label: 'Beta — Duplicate Check (BETA-001)',
    accountCode: 'BETA-001',
    scenarioName: 'duplicate_detected',
  },
  {
    key: 'over_limit',
    label: 'Alpha — Over Limit ($5001)',
    accountCode: 'ALPHA-001',
    scenarioName: 'clean_pass',
    defaultAmount: '5001',
  },
  {
    key: 'micr_failure',
    label: 'Alpha — MICR Failure (ALPHA-004)',
    accountCode: 'ALPHA-004',
    scenarioName: 'micr_failure',
  },
] as const

interface RejectedTransfer {
  id: string
  error_code: string
  user_msg?: string
  duplicate_original_tx_id?: string
}

// Minimal placeholder images for the demo (the API requires front/back image fields)
const PLACEHOLDER_IMAGE = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=='

export default function DepositPage() {
  const navigate = useNavigate()
  const [scenarioKey, setScenarioKey] = useState<string>(SCENARIOS[0].key)
  const [amount, setAmount] = useState('')
  const [loading, setLoading] = useState(false)
  const [rejected, setRejected] = useState<RejectedTransfer | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const selectedScenario = SCENARIOS.find(s => s.key === scenarioKey) ?? SCENARIOS[0]

  function resetForm() {
    setRejected(null)
    setSubmitError(null)
    setAmount('')
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    setRejected(null)
    setSubmitError(null)

    try {
      const transfer = await apiFetch<{
        id: string
        state: string
        error_code?: string
        user_msg?: string
        duplicate_original_tx_id?: string
      }>('/deposits', {
        method: 'POST',
        headers: {
          'X-Scenario': selectedScenario.scenarioName,
        },
        body: JSON.stringify({
          account_code: selectedScenario.accountCode,
          amount: parseFloat(amount),
          front_image: PLACEHOLDER_IMAGE,
          back_image: PLACEHOLDER_IMAGE,
        }),
      })

      if (transfer.state === 'Rejected') {
        setRejected({
          id: transfer.id,
          error_code: transfer.error_code ?? 'UNKNOWN',
          user_msg: transfer.user_msg,
          duplicate_original_tx_id: transfer.duplicate_original_tx_id,
        })
      } else {
        navigate(`/status/${transfer.id}`)
      }
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Submission failed. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1.5rem' }}>Deposit Check</h1>

      {rejected ? (
        <>
          <DepositError
            errorCode={rejected.error_code}
            userMsg={rejected.user_msg}
            duplicateRef={rejected.duplicate_original_tx_id}
            onRetry={resetForm}
          />
          {/* For non-retryable errors show a plain back link */}
          {!['VSS_IQA_BLUR', 'VSS_IQA_GLARE'].includes(rejected.error_code) && (
            <button
              onClick={resetForm}
              style={{
                marginTop: '1rem',
                padding: '0.45rem 1.1rem',
                background: 'transparent',
                color: '#1a1a2e',
                border: '1px solid #1a1a2e',
                borderRadius: '6px',
                cursor: 'pointer',
              }}
            >
              Start New Deposit
            </button>
          )}
        </>
      ) : (
        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: '1.25rem' }}>
            <label htmlFor="scenario" style={{ display: 'block', marginBottom: '0.4rem', fontWeight: 500 }}>
              Demo Scenario
            </label>
            <select
              id="scenario"
              value={scenarioKey}
              onChange={(e) => {
                const key = e.target.value
                setScenarioKey(key)
                const scenario = SCENARIOS.find(s => s.key === key)
                if (scenario && 'defaultAmount' in scenario && scenario.defaultAmount) {
                  setAmount(scenario.defaultAmount)
                }
              }}
              style={{
                width: '100%',
                padding: '0.5rem',
                borderRadius: '6px',
                border: '1px solid #ccc',
                fontSize: '0.95rem',
              }}
            >
              {SCENARIOS.map((s) => (
                <option key={s.key} value={s.key}>
                  {s.label}
                </option>
              ))}
            </select>
          </div>

          <div style={{ marginBottom: '1.25rem' }}>
            <label htmlFor="amount" style={{ display: 'block', marginBottom: '0.4rem', fontWeight: 500 }}>
              Amount (USD)
            </label>
            <input
              id="amount"
              type="number"
              min="0.01"
              step="0.01"
              required
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="e.g. 250.00"
              style={{
                width: '100%',
                padding: '0.5rem',
                borderRadius: '6px',
                border: '1px solid #ccc',
                fontSize: '0.95rem',
                boxSizing: 'border-box',
              }}
            />
          </div>

          {submitError && (
            <p style={{ color: '#b02a37', marginBottom: '1rem', fontSize: '0.9rem' }}>{submitError}</p>
          )}

          <button
            type="submit"
            disabled={loading}
            style={{
              width: '100%',
              padding: '0.65rem',
              background: loading ? '#6c757d' : '#1a1a2e',
              color: '#fff',
              border: 'none',
              borderRadius: '6px',
              fontSize: '1rem',
              fontWeight: 600,
              cursor: loading ? 'not-allowed' : 'pointer',
            }}
          >
            {loading ? 'Submitting…' : 'Submit Deposit'}
          </button>
        </form>
      )}
    </div>
  )
}
