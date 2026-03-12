import { useState, useEffect, useRef, useCallback } from 'react'
import { apiFetch } from '../api/client'

interface Notification {
  id: string
  account_id: string
  transfer_id?: string
  type: string
  message: string
  read_at?: string
  created_at: string
}

interface Props {
  /** Re-key when the user changes to reset state */
  userKey: string
}

export default function NotificationBell({ userKey }: Props) {
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [open, setOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  const fetchNotifications = useCallback(async () => {
    try {
      const data = await apiFetch<Notification[]>('/notifications')
      setNotifications(data)
    } catch {
      // auth failure or no notifications — stay silent
    }
  }, [])

  // Reset on user switch
  useEffect(() => {
    setNotifications([])
    setOpen(false)
    fetchNotifications()
  }, [userKey, fetchNotifications])

  // Poll every 10 seconds
  useEffect(() => {
    const interval = setInterval(fetchNotifications, 10_000)
    return () => clearInterval(interval)
  }, [fetchNotifications])

  // Close dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  const unreadCount = notifications.filter(n => !n.read_at).length

  async function markRead(id: string) {
    try {
      await apiFetch(`/notifications/${id}/read`, { method: 'PATCH' })
      setNotifications(prev =>
        prev.map(n => n.id === id ? { ...n, read_at: new Date().toISOString() } : n)
      )
    } catch {
      // ignore
    }
  }

  return (
    <div ref={dropdownRef} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          position: 'relative',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          fontSize: '1.3rem',
          lineHeight: 1,
          padding: '0.25rem',
          color: '#fff',
        }}
        aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ''}`}
      >
        🔔
        {unreadCount > 0 && (
          <span style={{
            position: 'absolute',
            top: 0,
            right: 0,
            background: '#ef5350',
            color: '#fff',
            borderRadius: '50%',
            fontSize: '0.65rem',
            fontWeight: 'bold',
            minWidth: '16px',
            height: '16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            lineHeight: 1,
            padding: '0 3px',
          }}>
            {unreadCount > 9 ? '9+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div style={{
          position: 'absolute',
          right: 0,
          top: 'calc(100% + 6px)',
          width: '320px',
          background: '#fff',
          border: '1px solid #ddd',
          borderRadius: '6px',
          boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
          zIndex: 1000,
          maxHeight: '400px',
          overflowY: 'auto',
          color: '#1a1a2e',
        }}>
          <div style={{
            padding: '0.6rem 0.9rem',
            borderBottom: '1px solid #eee',
            fontWeight: 600,
            fontSize: '0.9rem',
          }}>
            Notifications
          </div>

          {notifications.length === 0 && (
            <div style={{ padding: '1rem', color: '#888', fontSize: '0.85rem', textAlign: 'center' }}>
              No notifications
            </div>
          )}

          {notifications.map(n => (
            <div
              key={n.id}
              onClick={() => !n.read_at && markRead(n.id)}
              style={{
                padding: '0.65rem 0.9rem',
                borderBottom: '1px solid #f0f0f0',
                background: n.read_at ? '#fff' : '#fffde7',
                cursor: n.read_at ? 'default' : 'pointer',
                fontSize: '0.85rem',
              }}
            >
              <div style={{ fontWeight: n.read_at ? 400 : 600, marginBottom: '0.2rem' }}>
                {n.message}
              </div>
              <div style={{ color: '#888', fontSize: '0.75rem' }}>
                {new Date(n.created_at).toLocaleString()}
                {!n.read_at && <span style={{ marginLeft: '0.5rem', color: '#1976d2' }}>● unread</span>}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
