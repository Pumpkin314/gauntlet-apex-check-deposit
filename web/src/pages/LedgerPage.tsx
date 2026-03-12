import { useState, useEffect } from 'react'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

interface BalanceEntry {
  account_id: string
  code: string
  type: string
  balance: string
}

interface HealthStatus {
  healthy: boolean
  sum: string
}

export default function LedgerPage() {
  const [balances, setBalances] = useState<BalanceEntry[]>([])
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [loading, setLoading] = useState(true)

  const fetchData = () => {
    Promise.all([
      fetch(`${API_URL}/ledger/balances`).then(r => r.json()),
      fetch(`${API_URL}/health/ledger`).then(r => r.json()),
    ])
      .then(([b, h]) => {
        setBalances(b)
        setHealth(h)
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 5000)
    return () => clearInterval(interval)
  }, [])

  if (loading) {
    return (
      <div>
        <h1 style={{ fontSize: '1.5rem' }}>Ledger</h1>
        <p>Loading...</p>
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.5rem', flexWrap: 'wrap' }}>
        <h1 style={{ fontSize: '1.5rem', margin: 0 }}>Ledger</h1>

        {/* Reconciliation indicator */}
        {health && (
          <div style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '0.5rem',
            padding: '0.4rem 0.75rem',
            borderRadius: '20px',
            background: health.healthy ? '#e8f5e9' : '#ffebee',
            border: `1px solid ${health.healthy ? '#4caf50' : '#f44336'}`,
            fontSize: '0.9rem',
            fontWeight: 500,
          }}>
            <span style={{ color: health.healthy ? '#2e7d32' : '#c62828' }}>
              {health.healthy ? '\u2713' : '\u2717'}
            </span>
            <span style={{ color: health.healthy ? '#2e7d32' : '#c62828' }}>
              Reconciliation: ${health.sum}
            </span>
          </div>
        )}
      </div>

      {/* Balance Table */}
      <div style={{ overflowX: 'auto' }}>
        <table style={{
          width: '100%',
          borderCollapse: 'collapse',
          fontSize: '0.9rem',
        }}>
          <thead>
            <tr style={{ borderBottom: '2px solid #333' }}>
              <th style={{ textAlign: 'left', padding: '0.6rem 0.5rem' }}>Account Code</th>
              <th style={{ textAlign: 'left', padding: '0.6rem 0.5rem' }}>Type</th>
              <th style={{ textAlign: 'right', padding: '0.6rem 0.5rem' }}>Balance</th>
            </tr>
          </thead>
          <tbody>
            {balances.map(b => {
              const bal = parseFloat(b.balance)
              return (
                <tr key={b.account_id} style={{ borderBottom: '1px solid #eee' }}>
                  <td style={{ padding: '0.5rem' }}>
                    <code>{b.code}</code>
                  </td>
                  <td style={{ padding: '0.5rem' }}>
                    <span style={{
                      display: 'inline-block',
                      padding: '0.15rem 0.4rem',
                      borderRadius: '3px',
                      background: b.type === 'OMNIBUS' ? '#e3f2fd' :
                                  b.type === 'FEE' ? '#fff3e0' :
                                  b.type === 'IRA' ? '#f3e5f5' : '#f5f5f5',
                      fontSize: '0.8rem',
                    }}>
                      {b.type}
                    </span>
                  </td>
                  <td style={{
                    padding: '0.5rem',
                    textAlign: 'right',
                    fontFamily: 'monospace',
                    color: bal > 0 ? '#2e7d32' : bal < 0 ? '#c62828' : '#666',
                    fontWeight: bal !== 0 ? 'bold' : 'normal',
                  }}>
                    ${b.balance}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
