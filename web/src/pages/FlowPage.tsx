import { useState, useEffect, useRef, useCallback } from 'react'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

interface TransferEvent {
  transfer_id: string
  from_state: string
  to_state: string
  trigger: string
  timestamp?: string
}

interface TransferCard {
  id: string
  state: string
  events: TransferEvent[]
  lastUpdate: string
}

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
  Completed:  { bg: '#e8f5e9', border: '#1b5e20', label: 'Completed' },
  Rejected:   { bg: '#ffebee', border: '#c62828', label: 'Rejected' },
  Returned:   { bg: '#ffebee', border: '#b71c1c', label: 'Returned' },
}

export default function FlowPage() {
  const [transfers, setTransfers] = useState<Map<string, TransferCard>>(new Map())
  const [eventLog, setEventLog] = useState<TransferEvent[]>([])
  const [connected, setConnected] = useState(false)
  const [settlement, setSettlement] = useState<SettlementHealth | null>(null)
  const [triggering, setTriggering] = useState(false)
  const eventSourceRef = useRef<EventSource | null>(null)

  const fetchSettlement = useCallback(() => {
    fetch(`${API_URL}/health/settlement`)
      .then(r => r.json())
      .then((data: SettlementHealth) => setSettlement(data))
      .catch(() => {})
  }, [])

  useEffect(() => {
    fetchSettlement()
    const interval = setInterval(fetchSettlement, 5000)
    return () => clearInterval(interval)
  }, [fetchSettlement])

  const triggerSettlement = async () => {
    setTriggering(true)
    try {
      await fetch(`${API_URL}/health/settlement/trigger`, { method: 'POST' })
      fetchSettlement()
    } catch {
      // ignore
    } finally {
      setTriggering(false)
    }
  }

  useEffect(() => {
    function connect() {
      const es = new EventSource(`${API_URL}/events/stream`)
      eventSourceRef.current = es

      es.onopen = () => setConnected(true)
      es.onerror = () => {
        setConnected(false)
        es.close()
        setTimeout(connect, 3000)
      }

      es.addEventListener('transfer_update', (e) => {
        try {
          const data = JSON.parse(e.data)
          const event: TransferEvent = {
            transfer_id: data.transfer_id,
            from_state: data.from_state || '',
            to_state: data.to_state || data.state || '',
            trigger: data.trigger || 'system',
            timestamp: new Date().toISOString(),
          }

          setEventLog(prev => [event, ...prev].slice(0, 100))

          setTransfers(prev => {
            const next = new Map(prev)
            const existing = next.get(event.transfer_id)
            next.set(event.transfer_id, {
              id: event.transfer_id,
              state: event.to_state,
              events: [...(existing?.events || []), event],
              lastUpdate: event.timestamp || new Date().toISOString(),
            })
            return next
          })
        } catch {
          // ignore malformed events
        }
      })
    }

    connect()
    return () => {
      eventSourceRef.current?.close()
    }
  }, [])

  const transferList = Array.from(transfers.values()).sort(
    (a, b) => new Date(b.lastUpdate).getTime() - new Date(a.lastUpdate).getTime()
  )

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
          <h2 style={{ fontSize: '1.1rem', marginBottom: '0.75rem' }}>Active Transfers</h2>
          {transferList.length === 0 && (
            <p style={{ color: '#999' }}>No transfers yet. Submit a deposit to see live updates.</p>
          )}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
            {transferList.map(t => {
              const colors = stateColors[t.state] || { bg: '#f5f5f5', border: '#999', label: t.state }
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
                    {t.events.length} transitions | Last: {new Date(t.lastUpdate).toLocaleTimeString()}
                  </div>
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
                         ev.to_state === 'FundsPosted' || ev.to_state === 'Completed' ? '#66bb6a' : '#fff',
                }}>{ev.to_state}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
