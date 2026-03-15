import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiFetch } from '../api/client'

interface Transfer {
  id: string
  amount: number
  currency: string
  state: string
  created_at: string
  error_code?: string
}

interface Props {
  account: { id: string; code: string; type: string; name: string }
}

const STATE_LABELS: Record<string, { label: string; color: string }> = {
  Requested:   { label: 'Submitted', color: '#6c757d' },
  Validating:  { label: 'Validating', color: '#0d6efd' },
  Analyzing:   { label: 'Analyzing', color: '#fd7e14' },
  Approved:    { label: 'Approved', color: '#198754' },
  FundsPosted: { label: 'Funds Available', color: '#198754' },
  Completed:   { label: 'Settled', color: '#0d6efd' },
  Rejected:    { label: 'Rejected', color: '#dc3545' },
  Returned:    { label: 'Returned', color: '#6f42c1' },
}

export default function InvestorDashboardPage({ account }: Props) {
  const [balance, setBalance] = useState<string | null>(null)
  const [transfers, setTransfers] = useState<Transfer[]>([])

  useEffect(() => {
    // Fetch account details with balance
    apiFetch<{ balance?: string }>(`/accounts/${account.code}`)
      .then(data => setBalance(data.balance ?? '0.00'))
      .catch(() => {})

    // Fetch deposit history
    apiFetch<Transfer[]>(`/deposits?account_id=${account.id}`)
      .then(setTransfers)
      .catch(() => {})
  }, [account.id, account.code])

  return (
    <div style={{ padding: '2rem', maxWidth: '600px', margin: '0 auto' }}>
      {/* Account summary */}
      <div style={{
        background: '#1a1a2e', color: '#fff', borderRadius: '8px',
        padding: '1.5rem', marginBottom: '1.5rem',
      }}>
        <div style={{ fontSize: '0.85rem', color: '#aaa', marginBottom: '0.25rem' }}>
          {account.code} / {account.type}
        </div>
        <div style={{ fontSize: '1.3rem', fontWeight: 600, marginBottom: '0.75rem' }}>
          {account.name}
        </div>
        <div style={{ fontSize: '0.85rem', color: '#aaa' }}>Available Balance</div>
        <div style={{ fontSize: '2rem', fontWeight: 700 }}>
          ${balance ?? '...'} <span style={{ fontSize: '0.9rem', color: '#aaa' }}>USD</span>
        </div>
      </div>

      {/* Actions */}
      <div style={{ marginBottom: '1.5rem' }}>
        <Link
          to="/deposit"
          style={{
            display: 'inline-block', padding: '0.6rem 1.2rem',
            background: '#1a1a2e', color: '#fff', textDecoration: 'none',
            borderRadius: '6px', fontWeight: 600, fontSize: '0.95rem',
          }}
        >
          Deposit a Check
        </Link>
      </div>

      {/* Deposit history */}
      <h2 style={{ fontSize: '1.1rem', marginBottom: '0.75rem' }}>Deposit History</h2>
      {transfers.length === 0 ? (
        <p style={{ color: '#666' }}>No deposits yet.</p>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.9rem' }}>
          <thead>
            <tr style={{ borderBottom: '2px solid #ddd', textAlign: 'left' }}>
              <th style={{ padding: '0.5rem 0.5rem 0.5rem 0' }}>Date</th>
              <th style={{ padding: '0.5rem' }}>Amount</th>
              <th style={{ padding: '0.5rem' }}>Status</th>
              <th style={{ padding: '0.5rem 0 0.5rem 0.5rem' }}></th>
            </tr>
          </thead>
          <tbody>
            {transfers.map(t => {
              const st = STATE_LABELS[t.state] ?? { label: t.state, color: '#666' }
              return (
                <tr key={t.id} style={{ borderBottom: '1px solid #eee' }}>
                  <td style={{ padding: '0.5rem 0.5rem 0.5rem 0', color: '#666' }}>
                    {new Date(t.created_at).toLocaleDateString()}
                  </td>
                  <td style={{ padding: '0.5rem', fontWeight: 500 }}>
                    ${Number(t.amount).toFixed(2)}
                  </td>
                  <td style={{ padding: '0.5rem' }}>
                    <span style={{
                      display: 'inline-block', padding: '0.15rem 0.5rem',
                      borderRadius: '3px', background: st.color, color: '#fff',
                      fontSize: '0.75rem', fontWeight: 600,
                    }}>
                      {st.label}
                    </span>
                  </td>
                  <td style={{ padding: '0.5rem 0 0.5rem 0.5rem' }}>
                    <Link to={`/status/${t.id}`} style={{ color: '#1a1a2e', fontSize: '0.8rem' }}>
                      Details
                    </Link>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
    </div>
  )
}
