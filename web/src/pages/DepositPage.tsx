import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch, getAuthToken, BASE_URL } from '../api/client'
import DepositError from '../components/DepositError'

// Demo scenarios mapped to seed accounts whose UUIDs the VSS stub resolves to
// the matching scenario in test-scenarios/scenarios.yaml
const SCENARIOS = [
  { key: 'clean_pass', label: 'Clean Check', accountCode: 'ALPHA-001' },
  { key: 'iqa_fail_blur', label: 'IQA Blur', accountCode: 'ALPHA-002' },
  { key: 'iqa_fail_glare', label: 'IQA Glare', accountCode: 'ALPHA-003' },
  { key: 'duplicate_detected', label: 'Duplicate Check', accountCode: 'BETA-001' },
  { key: 'micr_failure', label: 'MICR Failure', accountCode: 'ALPHA-004' },
] as const

// Scenarios where the user enters a custom amount
const AMOUNT_SCENARIOS = new Set(['clean_pass'])

interface RejectedTransfer {
  id: string
  error_code: string
  user_msg?: string
  duplicate_original_tx_id?: string
}

interface ExistingTransfer {
  id: string
  amount: number
  state: string
  created_at: string
}

export default function DepositPage() {
  const navigate = useNavigate()

  // Mode: 'upload' (real images) or 'demo' (scenario-based)
  const [mode, setMode] = useState<'upload' | 'demo'>('demo')

  // Demo scenario state
  const [scenarioKey, setScenarioKey] = useState<string>(SCENARIOS[0].key)
  const [amount, setAmount] = useState('')

  // Duplicate dropdown
  const [existingTransfers, setExistingTransfers] = useState<ExistingTransfer[]>([])
  const [duplicateTarget, setDuplicateTarget] = useState<string>('')

  // Upload mode state
  const [frontImage, setFrontImage] = useState<File | null>(null)
  const [backImage, setBackImage] = useState<File | null>(null)

  // Shared state
  const [loading, setLoading] = useState(false)
  const [rejected, setRejected] = useState<RejectedTransfer | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Fetch existing deposits for duplicate dropdown
  useEffect(() => {
    if (scenarioKey !== 'duplicate_detected') return
    apiFetch<ExistingTransfer[]>('/deposits')
      .then(transfers => {
        const eligible = transfers.filter(t =>
          !['Rejected', 'Returned'].includes(t.state)
        )
        setExistingTransfers(eligible)
        if (eligible.length > 0 && !duplicateTarget) {
          setDuplicateTarget(eligible[0].id)
        }
      })
      .catch(() => {})
  }, [scenarioKey])

  const selectedScenario = SCENARIOS.find(s => s.key === scenarioKey) ?? SCENARIOS[0]
  const needsAmount = mode === 'upload' || AMOUNT_SCENARIOS.has(scenarioKey)

  function resetForm() {
    setRejected(null)
    setSubmitError(null)
    setAmount('')
    setFrontImage(null)
    setBackImage(null)
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    setRejected(null)
    setSubmitError(null)

    try {
      let transfer: {
        id: string
        state: string
        error_code?: string
        user_msg?: string
        duplicate_original_tx_id?: string
      }

      if (mode === 'upload') {
        // Multipart form upload with real images
        const formData = new FormData()
        formData.append('account_code', 'ALPHA-001')
        formData.append('amount', amount)
        if (frontImage) formData.append('front_image', frontImage)
        if (backImage) formData.append('back_image', backImage)

        const token = getAuthToken()
        const headers: Record<string, string> = {}
        if (token) headers['Authorization'] = `Bearer ${token}`

        const res = await fetch(`${BASE_URL}/deposits`, {
          method: 'POST',
          headers,
          body: formData,
        })
        if (!res.ok) {
          const body = await res.text().catch(() => '')
          throw new Error(body || `API error: ${res.status}`)
        }
        transfer = await res.json()
      } else {
        // Demo scenario with placeholder images
        const depositAmount = needsAmount
          ? parseFloat(amount)
          : 250 // default amount for non-amount scenarios

        transfer = await apiFetch<typeof transfer>('/deposits', {
          method: 'POST',
          body: JSON.stringify({
            account_code: selectedScenario.accountCode,
            amount: depositAmount,
          }),
        })
      }

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

  if (rejected) {
    return (
      <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
        <h1 style={{ fontSize: '1.5rem', marginBottom: '1.5rem' }}>Deposit Check</h1>
        <DepositError
          errorCode={rejected.error_code}
          userMsg={rejected.user_msg}
          duplicateRef={rejected.duplicate_original_tx_id}
          onRetry={resetForm}
        />
        {!['VSS_IQA_BLUR', 'VSS_IQA_GLARE'].includes(rejected.error_code) && (
          <button onClick={resetForm} style={outlineButtonStyle}>
            Start New Deposit
          </button>
        )}
      </div>
    )
  }

  return (
    <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1.5rem' }}>Deposit Check</h1>

      {/* Mode toggle */}
      <fieldset style={{ border: 'none', padding: 0, margin: '0 0 1.5rem' }}>
        <div style={{ display: 'flex', gap: '1.5rem' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', cursor: 'pointer' }}>
            <input
              type="radio"
              name="mode"
              checked={mode === 'upload'}
              onChange={() => setMode('upload')}
            />
            Upload check images
          </label>
          <label style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', cursor: 'pointer' }}>
            <input
              type="radio"
              name="mode"
              checked={mode === 'demo'}
              onChange={() => setMode('demo')}
            />
            Use demo scenario
          </label>
        </div>
      </fieldset>

      <form onSubmit={handleSubmit}>
        {mode === 'upload' ? (
          <>
            <div style={{ marginBottom: '1.25rem' }}>
              <label style={labelStyle}>Front of check</label>
              <input
                type="file"
                accept="image/*"
                required
                onChange={e => setFrontImage(e.target.files?.[0] ?? null)}
                style={inputStyle}
              />
            </div>
            <div style={{ marginBottom: '1.25rem' }}>
              <label style={labelStyle}>Back of check</label>
              <input
                type="file"
                accept="image/*"
                required
                onChange={e => setBackImage(e.target.files?.[0] ?? null)}
                style={inputStyle}
              />
            </div>
          </>
        ) : (
          <>
            <div style={{ marginBottom: '1.25rem' }}>
              <label htmlFor="scenario" style={labelStyle}>Demo Scenario</label>
              <select
                id="scenario"
                value={scenarioKey}
                onChange={e => setScenarioKey(e.target.value)}
                style={selectStyle}
              >
                {SCENARIOS.map(s => (
                  <option key={s.key} value={s.key}>{s.label}</option>
                ))}
              </select>
            </div>

            {scenarioKey === 'duplicate_detected' && (
              <div style={{ marginBottom: '1.25rem' }}>
                <label htmlFor="duplicate-target" style={labelStyle}>Deposit to duplicate</label>
                {existingTransfers.length === 0 ? (
                  <p style={{ fontSize: '0.85rem', color: '#666', margin: '0.25rem 0 0' }}>
                    No existing deposits found. Submit a clean check first.
                  </p>
                ) : (
                  <select
                    id="duplicate-target"
                    value={duplicateTarget}
                    onChange={e => setDuplicateTarget(e.target.value)}
                    style={selectStyle}
                  >
                    {existingTransfers.map(t => (
                      <option key={t.id} value={t.id}>
                        {t.id.slice(0, 8)}... — ${Number(t.amount).toFixed(2)} ({t.state})
                      </option>
                    ))}
                  </select>
                )}
              </div>
            )}
          </>
        )}

        {/* Amount — always shown for upload, only for clean_pass in demo mode */}
        {needsAmount && (
          <div style={{ marginBottom: '1.25rem' }}>
            <label htmlFor="amount" style={labelStyle}>Amount (USD)</label>
            <input
              id="amount"
              type="number"
              min="0.01"
              step="0.01"
              required
              value={amount}
              onChange={e => setAmount(e.target.value)}
              placeholder="e.g. 250.00"
              style={{ ...inputStyle, boxSizing: 'border-box' as const }}
            />
          </div>
        )}

        {submitError && (
          <p style={{ color: '#b02a37', marginBottom: '1rem', fontSize: '0.9rem' }}>{submitError}</p>
        )}

        <button type="submit" disabled={loading} style={submitButtonStyle(loading)}>
          {loading ? 'Submitting...' : 'Submit Deposit'}
        </button>
      </form>
    </div>
  )
}

// ---- Styles ----

const labelStyle: React.CSSProperties = {
  display: 'block',
  marginBottom: '0.4rem',
  fontWeight: 500,
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '0.5rem',
  borderRadius: '6px',
  border: '1px solid #ccc',
  fontSize: '0.95rem',
}

const selectStyle: React.CSSProperties = {
  width: '100%',
  padding: '0.5rem',
  borderRadius: '6px',
  border: '1px solid #ccc',
  fontSize: '0.95rem',
}

const outlineButtonStyle: React.CSSProperties = {
  marginTop: '1rem',
  padding: '0.45rem 1.1rem',
  background: 'transparent',
  color: '#1a1a2e',
  border: '1px solid #1a1a2e',
  borderRadius: '6px',
  cursor: 'pointer',
}

function submitButtonStyle(loading: boolean): React.CSSProperties {
  return {
    width: '100%',
    padding: '0.65rem',
    background: loading ? '#6c757d' : '#1a1a2e',
    color: '#fff',
    border: 'none',
    borderRadius: '6px',
    fontSize: '1rem',
    fontWeight: 600,
    cursor: loading ? 'not-allowed' : 'pointer',
  }
}
