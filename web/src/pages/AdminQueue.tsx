import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'
import TransferDetail from '../components/TransferDetail'

// ─── Shared types ─────────────────────────────────────────────────────────────

export interface MicrData {
  routing: string
  account: string
  check_number: string
}

export interface QueueTransfer {
  id: string
  account_id: string
  amount: number
  currency: string
  state: string
  review_reason: string | null
  error_code: string | null
  contribution_type: string | null
  contribution_type_override: string | null
  confidence_score: number | null
  micr_data: MicrData | null
  front_image_ref: string | null
  back_image_ref: string | null
  correspondent_id: string
  submitted_at: string
}

export interface TransferEvent {
  id: string
  transfer_id: string
  event_type: string
  data: Record<string, unknown>
  created_at: string
}

export interface RiskBadge {
  label: string
  color: 'red' | 'yellow'
}

export function getRiskBadges(t: QueueTransfer): RiskBadge[] {
  const badges: RiskBadge[] = []
  if (t.amount > 2000) {
    badges.push({ label: 'Large deposit', color: 'yellow' })
  }
  if (t.confidence_score !== null && t.confidence_score !== undefined && t.confidence_score < 0.9) {
    badges.push({ label: 'Low confidence', color: 'yellow' })
  }
  if (t.review_reason === 'VSS_MICR_READ_FAIL') {
    badges.push({ label: 'MICR unreadable', color: 'red' })
  }
  if (t.review_reason === 'VSS_AMOUNT_MISMATCH') {
    badges.push({ label: 'Amount discrepancy', color: 'yellow' })
  }
  return badges
}

export function RiskBadgeChip({ badge }: { badge: RiskBadge }) {
  const isRed = badge.color === 'red'
  return (
    <span style={{
      display: 'inline-block',
      padding: '0.15rem 0.5rem',
      borderRadius: '12px',
      fontSize: '0.72rem',
      fontWeight: 700,
      background: isRed ? '#fde8e8' : '#fef9c3',
      color: isRed ? '#991b1b' : '#854d0e',
      border: `1px solid ${isRed ? '#fca5a5' : '#fde68a'}`,
      whiteSpace: 'nowrap',
    }}>
      {badge.label}
    </span>
  )
}

// ─── Filter bar ───────────────────────────────────────────────────────────────

interface Filters {
  after: string
  before: string
  status: string
  account_id: string
  min_amount: string
  max_amount: string
}

interface FilterBarProps {
  filters: Filters
  onChange: (f: Filters) => void
  onRefresh: () => void
  loading: boolean
}

function FilterBar({ filters, onChange, onRefresh, loading }: FilterBarProps) {
  const set = (key: keyof Filters) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    onChange({ ...filters, [key]: e.target.value })

  return (
    <div style={filterBarStyle}>
      <div style={filterGroupStyle}>
        <label style={filterLabelStyle}>From date</label>
        <input type="date" value={filters.after} onChange={set('after')} style={filterInputStyle} />
      </div>
      <div style={filterGroupStyle}>
        <label style={filterLabelStyle}>To date</label>
        <input type="date" value={filters.before} onChange={set('before')} style={filterInputStyle} />
      </div>
      <div style={filterGroupStyle}>
        <label style={filterLabelStyle}>Status</label>
        <select value={filters.status} onChange={set('status')} style={filterInputStyle}>
          <option value="">All flagged</option>
          <option value="Analyzing">Analyzing</option>
          <option value="Approved">Approved</option>
          <option value="Rejected">Rejected</option>
        </select>
      </div>
      <div style={filterGroupStyle}>
        <label style={filterLabelStyle}>Account ID</label>
        <input
          type="text"
          value={filters.account_id}
          onChange={set('account_id')}
          placeholder="Search account…"
          style={filterInputStyle}
        />
      </div>
      <div style={filterGroupStyle}>
        <label style={filterLabelStyle}>Min $</label>
        <input
          type="number"
          value={filters.min_amount}
          onChange={set('min_amount')}
          min={0}
          placeholder="0"
          style={{ ...filterInputStyle, width: '80px' }}
        />
      </div>
      <div style={filterGroupStyle}>
        <label style={filterLabelStyle}>Max $</label>
        <input
          type="number"
          value={filters.max_amount}
          onChange={set('max_amount')}
          min={0}
          placeholder="∞"
          style={{ ...filterInputStyle, width: '80px' }}
        />
      </div>
      <button onClick={onRefresh} disabled={loading} style={refreshBtnStyle}>
        {loading ? '…' : '↺ Refresh'}
      </button>
    </div>
  )
}

// ─── Transfer list row ────────────────────────────────────────────────────────

function TransferRow({
  transfer,
  selected,
  onClick,
}: {
  transfer: QueueTransfer
  selected: boolean
  onClick: () => void
}) {
  const badges = getRiskBadges(transfer)

  return (
    <tr
      onClick={onClick}
      style={{
        cursor: 'pointer',
        background: selected ? '#eef2ff' : 'transparent',
        borderBottom: '1px solid #eee',
      }}
    >
      <td style={tdStyle}>{new Date(transfer.submitted_at).toLocaleDateString()}</td>
      <td style={tdStyle}>
        <span style={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
          {transfer.id.slice(0, 8)}…
        </span>
      </td>
      <td style={{ ...tdStyle, fontWeight: 600 }}>${transfer.amount.toFixed(2)}</td>
      <td style={tdStyle}>
        <span style={stateChip(transfer.state)}>{transfer.state}</span>
      </td>
      <td style={tdStyle}>
        <span style={{ fontSize: '0.8rem', color: '#666' }}>
          {transfer.review_reason ?? '—'}
        </span>
      </td>
      <td style={tdStyle}>
        {transfer.confidence_score !== null && transfer.confidence_score !== undefined
          ? `${(transfer.confidence_score * 100).toFixed(0)}%`
          : '—'}
      </td>
      <td style={{ ...tdStyle, minWidth: '180px' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.25rem' }}>
          {badges.map(b => <RiskBadgeChip key={b.label} badge={b} />)}
        </div>
      </td>
    </tr>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function AdminQueue() {
  const [transfers, setTransfers] = useState<QueueTransfer[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [filters, setFilters] = useState<Filters>({
    after: '',
    before: '',
    status: '',
    account_id: '',
    min_amount: '',
    max_amount: '',
  })

  const fetchQueue = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const params = new URLSearchParams()
      if (filters.after) params.set('after', new Date(filters.after).toISOString())
      if (filters.before) params.set('before', new Date(filters.before + 'T23:59:59').toISOString())
      if (filters.status) params.set('status', filters.status)
      if (filters.account_id.trim()) params.set('account_id', filters.account_id.trim())
      if (filters.min_amount) params.set('min_amount', filters.min_amount)
      if (filters.max_amount) params.set('max_amount', filters.max_amount)
      const qs = params.toString()
      const data = await apiFetch<QueueTransfer[]>(`/operator/queue${qs ? '?' + qs : ''}`)
      setTransfers(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load queue')
    } finally {
      setLoading(false)
    }
  }, [filters])

  useEffect(() => { fetchQueue() }, [fetchQueue])

  const selectedTransfer = transfers.find(t => t.id === selectedId) ?? null

  function handleReviewed(transferId: string) {
    setTransfers(prev => prev.filter(t => t.id !== transferId))
    setSelectedId(null)
  }

  return (
    <div>
      <h1 style={{ fontSize: '1.5rem', marginBottom: '1rem' }}>Review Queue</h1>

      <FilterBar filters={filters} onChange={setFilters} onRefresh={fetchQueue} loading={loading} />

      <div style={{ display: 'flex', gap: '1.5rem', alignItems: 'flex-start', marginTop: '1rem' }}>
        {/* List panel */}
        <div style={{ flex: selectedId ? '0 0 auto' : 1, maxWidth: selectedId ? '55%' : '100%', overflowX: 'auto' }}>
          {error && (
            <div style={errorBannerStyle}>{error}</div>
          )}
          {!error && !loading && transfers.length === 0 && (
            <p style={{ color: '#888', fontSize: '0.95rem', padding: '2rem 0' }}>
              No flagged transfers match the current filters.
            </p>
          )}
          {!error && transfers.length > 0 && (
            <table style={tableStyle}>
              <thead>
                <tr style={{ borderBottom: '2px solid #1a1a2e' }}>
                  {['Date', 'ID', 'Amount', 'State', 'Reason', 'Confidence', 'Risk'].map(h => (
                    <th key={h} style={thStyle}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {transfers.map(t => (
                  <TransferRow
                    key={t.id}
                    transfer={t}
                    selected={t.id === selectedId}
                    onClick={() => setSelectedId(prev => prev === t.id ? null : t.id)}
                  />
                ))}
              </tbody>
            </table>
          )}
          {loading && (
            <p style={{ color: '#888', padding: '1rem 0' }}>Loading…</p>
          )}
        </div>

        {/* Detail panel */}
        {selectedId && selectedTransfer && (
          <div style={{ flex: 1, minWidth: 0 }}>
            <TransferDetail
              transfer={selectedTransfer}
              onClose={() => setSelectedId(null)}
              onReviewed={handleReviewed}
            />
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const filterBarStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: '0.75rem',
  alignItems: 'flex-end',
  padding: '0.75rem',
  background: '#fff',
  border: '1px solid #ddd',
  borderRadius: '6px',
}

const filterGroupStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '0.25rem',
}

const filterLabelStyle: React.CSSProperties = {
  fontSize: '0.7rem',
  fontWeight: 700,
  textTransform: 'uppercase',
  letterSpacing: '0.04em',
  color: '#666',
}

const filterInputStyle: React.CSSProperties = {
  padding: '0.35rem 0.5rem',
  border: '1px solid #ccc',
  borderRadius: '4px',
  fontSize: '0.85rem',
  minWidth: '120px',
}

const refreshBtnStyle: React.CSSProperties = {
  padding: '0.35rem 0.85rem',
  background: '#1a1a2e',
  color: '#fff',
  border: 'none',
  borderRadius: '4px',
  cursor: 'pointer',
  fontSize: '0.85rem',
  fontWeight: 600,
  alignSelf: 'flex-end',
}

const tableStyle: React.CSSProperties = {
  width: '100%',
  borderCollapse: 'collapse',
  background: '#fff',
  borderRadius: '6px',
  overflow: 'hidden',
  boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
}

const thStyle: React.CSSProperties = {
  padding: '0.6rem 0.75rem',
  textAlign: 'left',
  fontSize: '0.75rem',
  fontWeight: 700,
  textTransform: 'uppercase',
  letterSpacing: '0.04em',
  color: '#1a1a2e',
  background: '#f5f5f5',
}

const tdStyle: React.CSSProperties = {
  padding: '0.6rem 0.75rem',
  fontSize: '0.875rem',
  verticalAlign: 'middle',
}

const errorBannerStyle: React.CSSProperties = {
  padding: '0.75rem 1rem',
  background: '#fde8e8',
  border: '1px solid #fca5a5',
  borderRadius: '4px',
  color: '#991b1b',
  fontSize: '0.9rem',
}

function stateChip(state: string): React.CSSProperties {
  const colors: Record<string, [string, string]> = {
    Analyzing: ['#dbeafe', '#1e40af'],
    Approved: ['#d1fae5', '#065f46'],
    Rejected: ['#fde8e8', '#991b1b'],
    FundsPosted: ['#d1fae5', '#065f46'],
    Completed: ['#f0fdf4', '#166534'],
    Returned: ['#fef9c3', '#854d0e'],
  }
  const [bg, color] = colors[state] ?? ['#f3f4f6', '#374151']
  return {
    display: 'inline-block',
    padding: '0.1rem 0.5rem',
    borderRadius: '10px',
    fontSize: '0.75rem',
    fontWeight: 700,
    background: bg,
    color,
  }
}
