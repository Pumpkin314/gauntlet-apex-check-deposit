import { useState, useEffect, type FormEvent } from 'react'
import { apiFetch } from '../api/client'

interface Account {
  id: string
  code: string
  type: string
  correspondent_id?: string
}

// Map account codes to display names for the demo
const INVESTOR_NAMES: Record<string, string> = {
  'ALPHA-001': 'Alice Johnson',
  'ALPHA-002': 'Bob Smith',
  'ALPHA-003': 'Carol Davis',
  'ALPHA-004': 'Dan Wilson',
  'ALPHA-005': 'Eve Martinez',
  'ALPHA-IRA': 'Alice Johnson (IRA)',
  'BETA-001': 'Frank Lee',
  'BETA-002': 'Grace Kim',
  'BETA-IRA': 'Frank Lee (IRA)',
}

export function getInvestorName(code: string): string {
  return INVESTOR_NAMES[code] ?? code
}

interface Props {
  onLogin: (account: { id: string; code: string; type: string; name: string }) => void
}

export default function InvestorLoginPage({ onLogin }: Props) {
  const [mode, setMode] = useState<'select' | 'manual'>('select')
  const [accounts, setAccounts] = useState<Account[]>([])
  const [selectedCode, setSelectedCode] = useState('')
  const [manualCode, setManualCode] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    apiFetch<Account[]>('/accounts')
      .then(accts => {
        setAccounts(accts)
        if (accts.length > 0) setSelectedCode(accts[0].code)
      })
      .catch(() => {})
  }, [])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)

    const code = mode === 'select' ? selectedCode : manualCode.trim()
    if (!code) {
      setError('Please enter an account code.')
      setLoading(false)
      return
    }

    try {
      const acct = await apiFetch<{ id: string; code: string; type: string }>(`/accounts/${code}`)
      onLogin({
        id: acct.id,
        code: acct.code,
        type: acct.type,
        name: getInvestorName(acct.code),
      })
    } catch {
      setError(`Account "${code}" not found.`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ padding: '3rem', maxWidth: '420px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '0.5rem' }}>Investor Login</h1>
      <p style={{ color: '#666', fontSize: '0.9rem', marginBottom: '1.5rem' }}>
        Sign in to view your account and deposit checks.
      </p>

      <fieldset style={{ border: 'none', padding: 0, margin: '0 0 1.25rem' }}>
        <div style={{ display: 'flex', gap: '1.5rem' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', cursor: 'pointer' }}>
            <input type="radio" name="login-mode" checked={mode === 'select'} onChange={() => setMode('select')} />
            Demo account
          </label>
          <label style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', cursor: 'pointer' }}>
            <input type="radio" name="login-mode" checked={mode === 'manual'} onChange={() => setMode('manual')} />
            Enter account ID
          </label>
        </div>
      </fieldset>

      <form onSubmit={handleSubmit}>
        {mode === 'select' ? (
          <div style={{ marginBottom: '1.25rem' }}>
            <label htmlFor="account-select" style={{ display: 'block', marginBottom: '0.4rem', fontWeight: 500 }}>
              Account
            </label>
            <select
              id="account-select"
              value={selectedCode}
              onChange={e => setSelectedCode(e.target.value)}
              style={{
                width: '100%', padding: '0.5rem', borderRadius: '6px',
                border: '1px solid #ccc', fontSize: '0.95rem',
              }}
            >
              {accounts.map(a => (
                <option key={a.code} value={a.code}>
                  {getInvestorName(a.code)} — {a.code} ({a.type})
                </option>
              ))}
            </select>
          </div>
        ) : (
          <div style={{ marginBottom: '1.25rem' }}>
            <label htmlFor="account-manual" style={{ display: 'block', marginBottom: '0.4rem', fontWeight: 500 }}>
              Account Code
            </label>
            <input
              id="account-manual"
              type="text"
              value={manualCode}
              onChange={e => setManualCode(e.target.value)}
              placeholder="e.g. ALPHA-001"
              style={{
                width: '100%', padding: '0.5rem', borderRadius: '6px',
                border: '1px solid #ccc', fontSize: '0.95rem', boxSizing: 'border-box',
              }}
            />
          </div>
        )}

        {error && (
          <p style={{ color: '#b02a37', marginBottom: '1rem', fontSize: '0.9rem' }}>{error}</p>
        )}

        <button
          type="submit"
          disabled={loading}
          style={{
            width: '100%', padding: '0.65rem',
            background: loading ? '#6c757d' : '#1a1a2e',
            color: '#fff', border: 'none', borderRadius: '6px',
            fontSize: '1rem', fontWeight: 600,
            cursor: loading ? 'not-allowed' : 'pointer',
          }}
        >
          {loading ? 'Signing in...' : 'Sign In'}
        </button>
      </form>
    </div>
  )
}
