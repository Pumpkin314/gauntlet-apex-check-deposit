import { useEffect, useRef, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { apiFetch } from '../api/client'
import DepositError from '../components/DepositError'

interface Transfer {
  id: string
  state: string
  amount: number
  currency: string
  error_code?: string
  user_msg?: string
  duplicate_original_tx_id?: string
  created_at: string
}

const TERMINAL_STATES = new Set(['Rejected', 'Completed', 'Returned'])

const STATE_STYLES: Record<string, { color: string; label: string }> = {
  Requested:   { color: '#6c757d', label: 'Submitted' },
  Validating:  { color: '#0d6efd', label: 'Validating' },
  Analyzing:   { color: '#fd7e14', label: 'Analyzing' },
  Approved:    { color: '#198754', label: 'Approved' },
  FundsPosted: { color: '#198754', label: 'Funds available' },
  Completed:   { color: '#198754', label: 'Completed' },
  Rejected:    { color: '#dc3545', label: 'Rejected' },
  Returned:    { color: '#6f42c1', label: 'Returned' },
}

const POLL_INTERVAL_MS = 2000

export default function StatusPage() {
  const { id } = useParams<{ id: string }>()
  const [transfer, setTransfer] = useState<Transfer | null>(null)
  const [fetchError, setFetchError] = useState<string | null>(null)
  // Keep a ref so the polling callback can read the latest state
  const stateRef = useRef<string | null>(null)

  useEffect(() => {
    if (!id) return

    let cancelled = false

    async function fetchTransfer() {
      try {
        const t = await apiFetch<Transfer>(`/deposits/${id}`)
        if (!cancelled) {
          stateRef.current = t.state
          setTransfer(t)
          setFetchError(null)
        }
      } catch (err) {
        if (!cancelled) {
          setFetchError(err instanceof Error ? err.message : 'Failed to load transfer.')
        }
      }
    }

    fetchTransfer()

    const interval = setInterval(async () => {
      if (stateRef.current && TERMINAL_STATES.has(stateRef.current)) {
        clearInterval(interval)
        return
      }
      await fetchTransfer()
    }, POLL_INTERVAL_MS)

    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [id])

  const stateStyle = transfer
    ? (STATE_STYLES[transfer.state] ?? { color: '#6c757d', label: transfer.state })
    : null

  return (
    <div style={{ padding: '2rem', maxWidth: '480px', margin: '0 auto' }}>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1.5rem' }}>Transfer Status</h1>

      {fetchError && (
        <p style={{ color: '#b02a37' }}>{fetchError}</p>
      )}

      {!transfer && !fetchError && (
        <p style={{ color: '#666' }}>Loading…</p>
      )}

      {transfer && (
        <>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.6rem', marginBottom: '1rem' }}>
            <span
              style={{
                display: 'inline-block',
                width: '12px',
                height: '12px',
                borderRadius: '50%',
                background: stateStyle?.color ?? '#6c757d',
                flexShrink: 0,
              }}
            />
            <span style={{ fontWeight: 600, color: stateStyle?.color, fontSize: '1.1rem' }}>
              {stateStyle?.label ?? transfer.state}
            </span>
            {!TERMINAL_STATES.has(transfer.state) && (
              <span style={{ fontSize: '0.8rem', color: '#888' }}>— updating…</span>
            )}
          </div>

          <dl
            style={{
              margin: '0 0 1rem',
              display: 'grid',
              gridTemplateColumns: 'auto 1fr',
              gap: '0.25rem 1rem',
            }}
          >
            <dt style={{ color: '#666', fontSize: '0.9rem' }}>Transfer ID</dt>
            <dd style={{ margin: 0, fontSize: '0.85rem', fontFamily: 'monospace' }}>{transfer.id}</dd>
            <dt style={{ color: '#666', fontSize: '0.9rem' }}>Amount</dt>
            <dd style={{ margin: 0 }}>
              {transfer.currency} {Number(transfer.amount).toFixed(2)}
            </dd>
          </dl>

          {transfer.state === 'Rejected' && (
            <>
              <DepositError
                errorCode={transfer.error_code ?? 'UNKNOWN'}
                userMsg={transfer.user_msg}
                duplicateRef={transfer.duplicate_original_tx_id}
              />
              <div style={{ marginTop: '1rem' }}>
                <Link to="/deposit" style={{ color: '#1a1a2e', fontSize: '0.9rem' }}>
                  ← Start a new deposit
                </Link>
              </div>
            </>
          )}
        </>
      )}
    </div>
  )
}
