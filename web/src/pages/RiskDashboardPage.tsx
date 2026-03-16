import { useState, useEffect } from 'react'

const API_URL = import.meta.env.VITE_API_URL || '/api'

interface CorrespondentMetric {
  correspondent_id: string
  rate?: number
  total?: number
  rejected?: number
  completed?: number
  returned?: number
  amount?: number
}

interface InvestorMetric {
  account_id: string
  amount: number
}

interface RiskData {
  rejection_rate: CorrespondentMetric[]
  float_exposure: CorrespondentMetric[]
  return_rate: CorrespondentMetric[]
  top_investors: InvestorMetric[]
  avg_processing_time_secs: number
}

export default function RiskDashboardPage() {
  const [data, setData] = useState<RiskData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchData = () => {
    const token = localStorage.getItem('auth_token') || 'apex-admin'
    fetch(`${API_URL}/admin/risk-dashboard`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then(d => {
        setData(d)
        setLoading(false)
        setError(null)
      })
      .catch(e => {
        setError(e.message)
        setLoading(false)
      })
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 10000)
    return () => clearInterval(interval)
  }, [])

  if (loading) {
    return (
      <div>
        <h1 style={{ fontSize: '1.5rem' }}>Risk Dashboard</h1>
        <p>Loading...</p>
      </div>
    )
  }

  if (error) {
    return (
      <div>
        <h1 style={{ fontSize: '1.5rem' }}>Risk Dashboard</h1>
        <p style={{ color: '#c62828' }}>Error: {error}</p>
      </div>
    )
  }

  if (!data) return null

  const tableStyle: React.CSSProperties = {
    width: '100%',
    borderCollapse: 'collapse',
    fontSize: '0.9rem',
    marginBottom: '1.5rem',
  }
  const thStyle: React.CSSProperties = { textAlign: 'left', padding: '0.6rem 0.5rem', borderBottom: '2px solid #333' }
  const tdStyle: React.CSSProperties = { padding: '0.5rem', borderBottom: '1px solid #eee' }
  const tdRight: React.CSSProperties = { ...tdStyle, textAlign: 'right', fontFamily: 'monospace' }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.5rem', flexWrap: 'wrap' }}>
        <h1 style={{ fontSize: '1.5rem', margin: 0 }}>Risk Dashboard</h1>
        <div style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.5rem',
          padding: '0.4rem 0.75rem',
          borderRadius: '20px',
          background: '#e3f2fd',
          border: '1px solid #1976d2',
          fontSize: '0.85rem',
        }}>
          Avg Processing: {data.avg_processing_time_secs.toFixed(1)}s
        </div>
      </div>

      {/* Rejection Rate */}
      <h2 style={{ fontSize: '1.1rem', marginBottom: '0.5rem' }}>Rejection Rate (30 days)</h2>
      <table style={tableStyle}>
        <thead>
          <tr>
            <th style={thStyle}>Correspondent</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Total</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Rejected</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Rate</th>
          </tr>
        </thead>
        <tbody>
          {data.rejection_rate.map(m => (
            <tr key={m.correspondent_id}>
              <td style={tdStyle}><code>{m.correspondent_id.slice(0, 8)}...</code></td>
              <td style={tdRight}>{m.total}</td>
              <td style={tdRight}>{m.rejected}</td>
              <td style={tdRight}>{(m.rate ?? 0).toFixed(1)}%</td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Float Exposure */}
      <h2 style={{ fontSize: '1.1rem', marginBottom: '0.5rem' }}>Float Exposure</h2>
      <table style={tableStyle}>
        <thead>
          <tr>
            <th style={thStyle}>Correspondent</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Exposure</th>
          </tr>
        </thead>
        <tbody>
          {data.float_exposure.map(m => (
            <tr key={m.correspondent_id}>
              <td style={tdStyle}><code>{m.correspondent_id.slice(0, 8)}...</code></td>
              <td style={{ ...tdRight, color: (m.amount ?? 0) > 0 ? '#c62828' : '#2e7d32', fontWeight: 'bold' }}>
                ${(m.amount ?? 0).toFixed(2)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Return Rate */}
      <h2 style={{ fontSize: '1.1rem', marginBottom: '0.5rem' }}>Return Rate (30 days)</h2>
      <table style={tableStyle}>
        <thead>
          <tr>
            <th style={thStyle}>Correspondent</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Completed</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Returned</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Rate</th>
          </tr>
        </thead>
        <tbody>
          {data.return_rate.map(m => (
            <tr key={m.correspondent_id}>
              <td style={tdStyle}><code>{m.correspondent_id.slice(0, 8)}...</code></td>
              <td style={tdRight}>{m.completed}</td>
              <td style={tdRight}>{m.returned}</td>
              <td style={tdRight}>{(m.rate ?? 0).toFixed(1)}%</td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Top Investors */}
      <h2 style={{ fontSize: '1.1rem', marginBottom: '0.5rem' }}>Top Investors by Float</h2>
      <table style={tableStyle}>
        <thead>
          <tr>
            <th style={thStyle}>Account</th>
            <th style={{ ...thStyle, textAlign: 'right' }}>Float</th>
          </tr>
        </thead>
        <tbody>
          {data.top_investors.map(m => (
            <tr key={m.account_id}>
              <td style={tdStyle}><code>{m.account_id.slice(0, 8)}...</code></td>
              <td style={{ ...tdRight, fontWeight: 'bold' }}>${m.amount.toFixed(2)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
