import { useState, useEffect, useCallback } from 'react'
import { useAdminSSE } from '../components/AdminLayout'
import { POLL_SETTLEMENT_MS } from '../config'

const API_URL = import.meta.env.VITE_API_URL || '/api'

interface SettlementBatch {
  id: string
  generated_at: string
  count: number
  status: string
}

interface SettlementHealth {
  healthy: boolean
  unbatched_count: number
  ready_count: number
  last_batch: SettlementBatch | null
}

const stateColors: Record<string, { bg: string; border: string; label: string }> = {
  Requested:  { bg: '#e3f2fd', border: '#1976d2', label: 'Requested' },
  Validating: { bg: '#e3f2fd', border: '#1976d2', label: 'Validating' },
  Analyzing:  { bg: '#fff8e1', border: '#f9a825', label: 'Analyzing' },
  Approved:   { bg: '#e8f5e9', border: '#388e3c', label: 'Approved' },
  FundsPosted:{ bg: '#e8f5e9', border: '#2e7d32', label: 'Funds Posted' },
  Completed:  { bg: '#e0f2f1', border: '#00695c', label: 'Completed' },
  Rejected:   { bg: '#ffebee', border: '#c62828', label: 'Rejected' },
  Returned:   { bg: '#ffebee', border: '#b71c1c', label: 'Returned' },
}

const TERMINAL_STATES = new Set(['Rejected', 'Returned'])

export default function FlowPage() {
  const { transfers, eventLog, connected } = useAdminSSE()
  const [settlement, setSettlement] = useState<SettlementHealth | null>(null)
  const [triggering, setTriggering] = useState(false)
  const [simulatingReturn, setSimulatingReturn] = useState<string | null>(null)
  const [returnMsg, setReturnMsg] = useState<string | null>(null)
  const [searchTerm, setSearchTerm] = useState('')
  const [stateFilter, setStateFilter] = useState<string>('active')

  const fetchSettlement = useCallback(() => {
    fetch(`${API_URL}/health/settlement`)
      .then(r => r.json())
      .then((data: SettlementHealth) => setSettlement(data))
      .catch(() => {})
  }, [])

  useEffect(() => {
    fetchSettlement()
    const interval = setInterval(fetchSettlement, POLL_SETTLEMENT_MS)
    return () => clearInterval(interval)
  }, [fetchSettlement])

  const simulateReturn = async (transferId: string) => {
    setSimulatingReturn(transferId)
    setReturnMsg(null)
    try {
      const res = await fetch(`${API_URL}/admin/simulate-return`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ transfer_id: transferId, reason_code: 'R01' }),
      })
      if (res.ok) {
        setReturnMsg(`Return initiated for ${transferId.slice(0, 8)}… (R01 - Insufficient Funds)`)
      } else {
        setReturnMsg('Return request failed.')
      }
    } catch {
      setReturnMsg('Could not reach API.')
    } finally {
      setSimulatingReturn(null)
    }
  }

  const triggerSettlement = async () => {
    setTriggering(true)
    try {
      await fetch(`${API_URL}/settlement/trigger`, { method: 'POST' })
      fetchSettlement()
    } catch {
      // ignore
    } finally {
      setTriggering(false)
    }
  }

  const transferList = Array.from(transfers.values())
    .filter(t => {
      // State filter
      if (stateFilter === 'active' && TERMINAL_STATES.has(t.state)) return false
      if (stateFilter !== 'active' && stateFilter !== 'all' && t.state !== stateFilter) return false
      // Search by ID
      if (searchTerm && !t.id.toLowerCase().includes(searchTerm.toLowerCase())) return false
      return true
    })
    .sort((a, b) => new Date(b.lastUpdate).getTime() - new Date(a.lastUpdate).getTime())

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '1.5rem' }}>
        <h1 style={{ fontSize: '1.5rem', margin: 0 }}>Flow Dashboard</h1>
        <span style={{
          display: 'inline-block',
          width: 10,
          height: 10,
          borderRadius: '50%',
          background: connected ? '#4caf50' : '#f44336',
        }} />
        <span style={{ fontSize: '0.85rem', color: '#666' }}>
          {connected ? 'Connected' : 'Reconnecting...'}
        </span>
      </div>

      {/* Settlement Status */}
      <div style={{
        marginBottom: '1.5rem',
        padding: '0.75rem 1rem',
        borderRadius: '6px',
        background: settlement?.healthy === false ? '#fff3e0' : '#f5f5f5',
        border: `1px solid ${settlement?.healthy === false ? '#ff9800' : '#ddd'}`,
        display: 'flex',
        alignItems: 'center',
        gap: '1rem',
        flexWrap: 'wrap',
      }}>
        <div style={{ flex: 1 }}>
          <span style={{ fontWeight: 600, fontSize: '0.9rem' }}>Settlement: </span>
          {settlement === null && (
            <span style={{ color: '#999', fontSize: '0.85rem' }}>Loading...</span>
          )}
          {settlement !== null && settlement.last_batch && (
            <span style={{ fontSize: '0.85rem', color: '#388e3c' }}>
              Batch generated at {new Date(settlement.last_batch.generated_at).toLocaleTimeString()},
              {' '}{settlement.last_batch.count} deposit{settlement.last_batch.count !== 1 ? 's' : ''},{' '}
              {settlement.last_batch.status}
            </span>
          )}
          {settlement !== null && !settlement.last_batch && (
            <span style={{ fontSize: '0.85rem' }}>
              {settlement.ready_count} deposit{settlement.ready_count !== 1 ? 's' : ''} ready
            </span>
          )}
          {settlement?.healthy === false && (
            <span style={{
              marginLeft: '0.75rem',
              fontSize: '0.8rem',
              color: '#e65100',
              fontWeight: 600,
            }}>
              ⚠ {settlement.unbatched_count} unbatched transfer{settlement.unbatched_count !== 1 ? 's' : ''} from yesterday
            </span>
          )}
        </div>
        <button
          onClick={triggerSettlement}
          disabled={triggering}
          style={{
            padding: '0.4rem 0.9rem',
            background: triggering ? '#bbb' : '#1976d2',
            color: '#fff',
            border: 'none',
            borderRadius: '4px',
            cursor: triggering ? 'not-allowed' : 'pointer',
            fontSize: '0.85rem',
            fontWeight: 600,
          }}
        >
          {triggering ? 'Triggering...' : 'Trigger Settlement'}
        </button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
        {/* Transfer Cards */}
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem', flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: '1.1rem', margin: 0 }}>Transfers</h2>
            <input
              type="text"
              placeholder="Search by ID..."
              value={searchTerm}
              onChange={e => setSearchTerm(e.target.value)}
              style={{
                padding: '0.3rem 0.5rem', borderRadius: '4px',
                border: '1px solid #ccc', fontSize: '0.8rem', width: '140px',
              }}
            />
            <select
              value={stateFilter}
              onChange={e => setStateFilter(e.target.value)}
              style={{
                padding: '0.3rem 0.5rem', borderRadius: '4px',
                border: '1px solid #ccc', fontSize: '0.8rem',
              }}
            >
              <option value="active">Active Only</option>
              <option value="all">All States</option>
              <option value="FundsPosted">Funds Posted</option>
              <option value="Completed">Completed</option>
              <option value="Analyzing">Analyzing</option>
              <option value="Rejected">Rejected</option>
              <option value="Returned">Returned</option>
            </select>
          </div>
          {returnMsg && (
            <div style={{
              marginBottom: '0.5rem',
              padding: '0.5rem 0.75rem',
              background: '#e8f5e9',
              border: '1px solid #4caf50',
              borderRadius: '4px',
              fontSize: '0.8rem',
              color: '#1b5e20',
            }}>
              {returnMsg}
            </div>
          )}
          {transferList.length === 0 && (
            <p style={{ color: '#999' }}>No transfers yet. Submit a deposit to see live updates.</p>
          )}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
            {transferList.map(t => {
              const colors = stateColors[t.state] || { bg: '#f5f5f5', border: '#999', label: t.state }
              const canSimulateReturn = t.state === 'FundsPosted' || t.state === 'Completed'
              return (
                <div key={t.id} style={{
                  padding: '0.75rem',
                  borderLeft: `4px solid ${colors.border}`,
                  background: colors.bg,
                  borderRadius: '4px',
                }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <code style={{ fontSize: '0.8rem' }}>{t.id.slice(0, 8)}...</code>
                    <span style={{
                      padding: '0.2rem 0.5rem',
                      borderRadius: '3px',
                      background: colors.border,
                      color: '#fff',
                      fontSize: '0.75rem',
                      fontWeight: 'bold',
                    }}>{colors.label}</span>
                  </div>
                  <div style={{ fontSize: '0.75rem', color: '#666', marginTop: '0.25rem' }}>
                    {new Date(t.lastUpdate).toLocaleString()}
                  </div>
                  {canSimulateReturn && (
                    <button
                      onClick={() => simulateReturn(t.id)}
                      disabled={simulatingReturn === t.id}
                      style={{
                        marginTop: '0.5rem',
                        padding: '0.25rem 0.6rem',
                        background: simulatingReturn === t.id ? '#bbb' : '#b71c1c',
                        color: '#fff',
                        border: 'none',
                        borderRadius: '3px',
                        cursor: simulatingReturn === t.id ? 'not-allowed' : 'pointer',
                        fontSize: '0.75rem',
                        fontWeight: 600,
                      }}
                    >
                      {simulatingReturn === t.id ? 'Simulating…' : 'Simulate Return'}
                    </button>
                  )}
                </div>
              )
            })}
          </div>
        </div>

        {/* Event Stream Log */}
        <div>
          <h2 style={{ fontSize: '1.1rem', marginBottom: '0.75rem' }}>Event Stream</h2>
          <div style={{
            maxHeight: '500px',
            overflowY: 'auto',
            fontFamily: 'monospace',
            fontSize: '0.8rem',
            background: '#1a1a2e',
            color: '#e0e0e0',
            borderRadius: '4px',
            padding: '0.75rem',
          }}>
            {eventLog.length === 0 && (
              <div style={{ color: '#666' }}>Waiting for events...</div>
            )}
            {eventLog.map((ev, i) => (
              <div key={i} style={{ marginBottom: '0.3rem', borderBottom: '1px solid #333', paddingBottom: '0.3rem' }}>
                <span style={{ color: '#888' }}>
                  {ev.timestamp ? new Date(ev.timestamp).toLocaleTimeString() : ''}
                </span>
                {' '}
                <span style={{ color: '#64b5f6' }}>{ev.transfer_id.slice(0, 8)}</span>
                {' '}
                <span style={{ color: '#aaa' }}>{ev.from_state}</span>
                <span style={{ color: '#fff' }}> → </span>
                <span style={{
                  color: ev.to_state === 'Rejected' || ev.to_state === 'Returned' ? '#ef5350' :
                         ev.to_state === 'Completed' ? '#4dd0e1' :
                         ev.to_state === 'FundsPosted' ? '#66bb6a' : '#fff',
                }}>{ev.to_state}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
