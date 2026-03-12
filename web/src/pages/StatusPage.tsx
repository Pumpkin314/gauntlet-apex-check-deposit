import { useState, useEffect } from 'react'
import { useParams, Link } from 'react-router-dom'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

const stateMessages: Record<string, { text: string; color: string }> = {
  Requested:   { text: 'We received your check', color: '#1976d2' },
  Validating:  { text: 'Verifying your check image...', color: '#1976d2' },
  Analyzing:   { text: 'Reviewing your deposit', color: '#f9a825' },
  Approved:    { text: 'Your deposit has been approved', color: '#388e3c' },
  FundsPosted: { text: 'Funds are available in your account', color: '#2e7d32' },
  Completed:   { text: 'Deposit complete', color: '#1b5e20' },
  Rejected:    { text: 'Your deposit was not accepted', color: '#c62828' },
  Returned:    { text: 'This deposit has been returned', color: '#b71c1c' },
}

interface Transfer {
  id: string
  account_id: string
  from_account_id: string
  correspondent_id: string
  amount: number
  currency: string
  state: string
  type: string
  sub_type: string
  transfer_type: string
  memo: string
  error_code?: string
  review_reason?: string
  contribution_type?: string
  vendor_transaction_id?: string
  confidence_score?: number
  submitted_at: string
  created_at: string
  updated_at: string
}

export default function StatusPage() {
  const { id } = useParams()
  const [transfer, setTransfer] = useState<Transfer | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!id) return

    const fetchStatus = () => {
      fetch(`${API_URL}/deposits/${id}`)
        .then(r => {
          if (!r.ok) throw new Error('Transfer not found')
          return r.json()
        })
        .then(data => {
          setTransfer(data)
          setError('')
        })
        .catch(err => setError(err.message))
    }

    fetchStatus()
    const interval = setInterval(fetchStatus, 2000)
    return () => clearInterval(interval)
  }, [id])

  if (error) {
    return (
      <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
        <h1 style={{ fontSize: '1.5rem' }}>Transfer Status</h1>
        <p style={{ color: '#c62828' }}>{error}</p>
        <Link to="/deposit" style={{ color: '#1976d2' }}>Back to Deposit</Link>
      </div>
    )
  }

  if (!transfer) {
    return (
      <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
        <h1 style={{ fontSize: '1.5rem' }}>Transfer Status</h1>
        <p>Loading...</p>
      </div>
    )
  }

  const stateInfo = stateMessages[transfer.state] || { text: transfer.state, color: '#666' }

  return (
    <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1.5rem' }}>Transfer Status</h1>

      {/* State Banner */}
      <div style={{
        padding: '1.25rem',
        background: stateInfo.color + '15',
        borderLeft: `4px solid ${stateInfo.color}`,
        borderRadius: '6px',
        marginBottom: '1.5rem',
      }}>
        <div style={{ fontSize: '1.1rem', fontWeight: 'bold', color: stateInfo.color }}>
          {stateInfo.text}
        </div>
        <div style={{ fontSize: '0.85rem', color: '#666', marginTop: '0.25rem' }}>
          State: {transfer.state}
        </div>
      </div>

      {/* Error info */}
      {transfer.error_code && (
        <div style={{
          padding: '0.75rem',
          background: '#ffebee',
          borderRadius: '6px',
          marginBottom: '1rem',
          color: '#c62828',
          fontSize: '0.9rem',
        }}>
          Error: {transfer.error_code}
        </div>
      )}

      {/* Details */}
      <div style={{
        background: '#f5f5f5',
        borderRadius: '6px',
        padding: '1rem',
      }}>
        <h2 style={{ fontSize: '1rem', marginBottom: '0.75rem' }}>Details</h2>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.9rem' }}>
          <tbody>
            <DetailRow label="Transfer ID" value={transfer.id} />
            <DetailRow label="Amount" value={`$${transfer.amount.toFixed(2)}`} />
            <DetailRow label="Currency" value={transfer.currency} />
            <DetailRow label="Account" value={transfer.account_id.slice(0, 8) + '...'} />
            <DetailRow label="Type" value={`${transfer.type} / ${transfer.sub_type}`} />
            <DetailRow label="Submitted" value={new Date(transfer.submitted_at).toLocaleString()} />
            <DetailRow label="Last Updated" value={new Date(transfer.updated_at).toLocaleString()} />
            {transfer.contribution_type && (
              <DetailRow label="Contribution" value={transfer.contribution_type} />
            )}
            {transfer.confidence_score !== undefined && (
              <DetailRow label="Confidence" value={`${(transfer.confidence_score * 100).toFixed(0)}%`} />
            )}
          </tbody>
        </table>
      </div>

      <div style={{ marginTop: '1.5rem' }}>
        <Link to="/deposit" style={{
          display: 'inline-block',
          padding: '0.6rem 1.25rem',
          background: '#1976d2',
          color: '#fff',
          textDecoration: 'none',
          borderRadius: '6px',
          fontSize: '0.9rem',
          minHeight: '44px',
          lineHeight: '44px',
        }}>
          New Deposit
        </Link>
      </div>
    </div>
  )
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <tr>
      <td style={{ padding: '0.35rem 0', color: '#666', verticalAlign: 'top' }}>{label}</td>
      <td style={{ padding: '0.35rem 0', textAlign: 'right', wordBreak: 'break-all' }}>{value}</td>
    </tr>
  )
}
