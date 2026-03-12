import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

interface Scenario {
  name: string
  description: string
  trigger_account: string
}

export default function DepositPage() {
  const navigate = useNavigate()
  const [scenarios, setScenarios] = useState<Scenario[]>([])
  const [selectedScenario, setSelectedScenario] = useState('')
  const [accountCode, setAccountCode] = useState('')
  const [amount, setAmount] = useState('')
  const [frontImage, setFrontImage] = useState<File | null>(null)
  const [backImage, setBackImage] = useState<File | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    fetch(`${API_URL}/scenarios`)
      .then(r => r.json())
      .then(data => setScenarios(data))
      .catch(() => {})
  }, [])

  const handleScenarioChange = (name: string) => {
    setSelectedScenario(name)
    const scenario = scenarios.find(s => s.name === name)
    if (scenario) {
      setAccountCode(scenario.trigger_account)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setSubmitting(true)

    try {
      const idempotencyKey = `deposit-${Date.now()}-${Math.random().toString(36).slice(2)}`

      let res: Response
      if (frontImage || backImage) {
        const formData = new FormData()
        formData.append('account_code', accountCode)
        formData.append('amount', amount)
        if (frontImage) formData.append('front_image', frontImage)
        if (backImage) formData.append('back_image', backImage)
        res = await fetch(`${API_URL}/deposits`, {
          method: 'POST',
          headers: { 'Idempotency-Key': idempotencyKey },
          body: formData,
        })
      } else {
        res = await fetch(`${API_URL}/deposits`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Idempotency-Key': idempotencyKey,
          },
          body: JSON.stringify({ account_code: accountCode, amount: parseFloat(amount) }),
        })
      }

      const data = await res.json()
      if (!res.ok) {
        setError(data.error || 'Deposit failed')
        return
      }
      navigate(`/status/${data.id}`)
    } catch (err) {
      setError('Network error. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1.5rem' }}>Deposit Check</h1>

      <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
        {/* Scenario Selector */}
        <div>
          <label htmlFor="scenario" style={{ display: 'block', marginBottom: '0.25rem', fontWeight: 500 }}>
            Scenario
          </label>
          <select
            id="scenario"
            value={selectedScenario}
            onChange={e => handleScenarioChange(e.target.value)}
            style={{
              width: '100%',
              padding: '0.75rem',
              borderRadius: '6px',
              border: '1px solid #ccc',
              fontSize: '1rem',
              minHeight: '44px',
            }}
          >
            <option value="">Select a scenario...</option>
            {scenarios.map(s => (
              <option key={s.name} value={s.name}>
                {s.description || s.name} ({s.trigger_account})
              </option>
            ))}
          </select>
        </div>

        {/* Account Code */}
        <div>
          <label htmlFor="account_code" style={{ display: 'block', marginBottom: '0.25rem', fontWeight: 500 }}>
            Account Code
          </label>
          <input
            id="account_code"
            type="text"
            value={accountCode}
            onChange={e => setAccountCode(e.target.value)}
            placeholder="e.g. ALPHA-001"
            required
            style={{
              width: '100%',
              padding: '0.75rem',
              borderRadius: '6px',
              border: '1px solid #ccc',
              fontSize: '1rem',
              minHeight: '44px',
              boxSizing: 'border-box',
            }}
          />
        </div>

        {/* Amount */}
        <div>
          <label htmlFor="amount" style={{ display: 'block', marginBottom: '0.25rem', fontWeight: 500 }}>
            Amount ($)
          </label>
          <input
            id="amount"
            type="number"
            step="0.01"
            min="0.01"
            value={amount}
            onChange={e => setAmount(e.target.value)}
            placeholder="500.00"
            required
            style={{
              width: '100%',
              padding: '0.75rem',
              borderRadius: '6px',
              border: '1px solid #ccc',
              fontSize: '1rem',
              minHeight: '44px',
              boxSizing: 'border-box',
            }}
          />
        </div>

        {/* Front Image */}
        <div>
          <label htmlFor="front_image" style={{ display: 'block', marginBottom: '0.25rem', fontWeight: 500 }}>
            Front of Check
          </label>
          <input
            id="front_image"
            type="file"
            accept="image/*"
            capture="environment"
            onChange={e => setFrontImage(e.target.files?.[0] || null)}
            style={{
              width: '100%',
              padding: '0.75rem',
              minHeight: '44px',
              fontSize: '1rem',
            }}
          />
        </div>

        {/* Back Image */}
        <div>
          <label htmlFor="back_image" style={{ display: 'block', marginBottom: '0.25rem', fontWeight: 500 }}>
            Back of Check
          </label>
          <input
            id="back_image"
            type="file"
            accept="image/*"
            capture="environment"
            onChange={e => setBackImage(e.target.files?.[0] || null)}
            style={{
              width: '100%',
              padding: '0.75rem',
              minHeight: '44px',
              fontSize: '1rem',
            }}
          />
        </div>

        {error && (
          <div style={{
            padding: '0.75rem',
            background: '#ffebee',
            color: '#c62828',
            borderRadius: '6px',
            fontSize: '0.9rem',
          }}>
            {error}
          </div>
        )}

        <button
          type="submit"
          disabled={submitting || !accountCode || !amount}
          style={{
            padding: '0.75rem',
            background: submitting ? '#999' : '#1976d2',
            color: '#fff',
            border: 'none',
            borderRadius: '6px',
            fontSize: '1rem',
            fontWeight: 'bold',
            cursor: submitting ? 'not-allowed' : 'pointer',
            minHeight: '44px',
          }}
        >
          {submitting ? 'Submitting...' : 'Submit Deposit'}
        </button>
      </form>
    </div>
  )
}
