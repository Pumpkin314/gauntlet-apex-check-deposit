import { useState, useEffect, useRef } from 'react'
import { NavLink, Outlet, useOutletContext } from 'react-router-dom'

const API_URL = import.meta.env.VITE_API_URL || '/api'
const SSE_URL = import.meta.env.VITE_SSE_URL || API_URL

export interface TransferEvent {
  transfer_id: string
  from_state: string
  to_state: string
  trigger: string
  timestamp?: string
}

export interface TransferCard {
  id: string
  state: string
  events: TransferEvent[]
  lastUpdate: string
}

export interface AdminSSEContext {
  transfers: Map<string, TransferCard>
  eventLog: TransferEvent[]
  connected: boolean
}

export function useAdminSSE() {
  return useOutletContext<AdminSSEContext>()
}

const navLinkStyle = (isActive: boolean) => ({
  display: 'block',
  padding: '0.6rem 1rem',
  textDecoration: 'none',
  color: isActive ? '#fff' : '#ccc',
  background: isActive ? '#2a2a4a' : 'transparent',
  borderRadius: '4px',
  fontSize: '0.9rem',
})

export default function AdminLayout() {
  const [transfers, setTransfers] = useState<Map<string, TransferCard>>(new Map())
  const [eventLog, setEventLog] = useState<TransferEvent[]>([])
  const [connected, setConnected] = useState(false)
  const eventSourceRef = useRef<EventSource | null>(null)

  // Hydrate transfers from API on mount
  useEffect(() => {
    fetch(`${API_URL}/deposits`)
      .then(r => r.json())
      .then((deposits: Array<{ id: string; state: string; updated_at: string }>) => {
        setTransfers(prev => {
          const next = new Map(prev)
          for (const d of deposits) {
            if (!next.has(d.id)) {
              next.set(d.id, {
                id: d.id,
                state: d.state,
                events: [],
                lastUpdate: d.updated_at,
              })
            }
          }
          return next
        })
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    function connect() {
      const es = new EventSource(`${SSE_URL}/events/stream`)
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

  const ctx: AdminSSEContext = { transfers, eventLog, connected }

  return (
    <div className="admin-layout">
      <aside className="admin-sidebar">
        <nav style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
          <NavLink to="/admin/flow" style={({ isActive }) => navLinkStyle(isActive)}>
            Flow Dashboard
          </NavLink>
          <NavLink to="/admin/queue" style={({ isActive }) => navLinkStyle(isActive)}>
            Review Queue
          </NavLink>
          <NavLink to="/admin/ledger" style={({ isActive }) => navLinkStyle(isActive)}>
            Ledger
          </NavLink>
          <NavLink to="/admin/risk" style={({ isActive }) => navLinkStyle(isActive)}>
            Risk
          </NavLink>
        </nav>
      </aside>
      <div className="admin-content">
        <Outlet context={ctx} />
      </div>
    </div>
  )
}
