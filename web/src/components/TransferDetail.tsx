import { useState, useEffect } from 'react'
import { apiFetch, apiFetchBlobUrl } from '../api/client'
import OperatorActions from './OperatorActions'
import type { QueueTransfer, TransferEvent } from '../pages/AdminQueue'
import { RiskBadgeChip, getRiskBadges } from '../pages/AdminQueue'

interface Props {
  transfer: QueueTransfer
  onClose: () => void
  onReviewed: (transferId: string) => void
}

interface CheckImageProps {
  transferId: string
  side: 'front' | 'back'
  imageRef: string | null
}

function CheckImage({ transferId, side, imageRef }: CheckImageProps) {
  const [src, setSrc] = useState<string | null>(null)
  const [err, setErr] = useState(false)

  useEffect(() => {
    if (!imageRef) return
    let url: string
    apiFetchBlobUrl(`/deposits/${transferId}/images/${side}`)
      .then(u => { url = u; setSrc(u) })
      .catch(() => setErr(true))
    return () => { if (url) URL.revokeObjectURL(url) }
  }, [transferId, side, imageRef])

  if (!imageRef) return <div style={imgPlaceholder}>No image uploaded</div>
  if (err) return <div style={imgPlaceholder}>Image unavailable</div>
  if (!src) return <div style={imgPlaceholder}>Loading…</div>
  return (
    <img
      src={src}
      alt={`Check ${side}`}
      style={{ maxWidth: '100%', maxHeight: '200px', borderRadius: '4px', border: '1px solid #ddd' }}
    />
  )
}

export default function TransferDetail({ transfer, onClose, onReviewed }: Props) {
  const [events, setEvents] = useState<TransferEvent[]>([])
  const [eventsLoading, setEventsLoading] = useState(true)

  useEffect(() => {
    apiFetch<TransferEvent[]>(`/deposits/${transfer.id}/events`)
      .then(setEvents)
      .catch(() => setEvents([]))
      .finally(() => setEventsLoading(false))
  }, [transfer.id])

  // Extract recognized amount from vss_result event
  const vssEvent = events.find(e => e.event_type === 'vss_result')
  const recognizedAmount = vssEvent?.data?.ocr_amount as number | undefined

  const badges = getRiskBadges(transfer)
  const isAmountMismatch = transfer.review_reason === 'VSS_AMOUNT_MISMATCH'
  const isMicrFail = transfer.review_reason === 'VSS_MICR_READ_FAIL'

  return (
    <div style={panelStyle}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '1rem' }}>
        <div>
          <h2 style={{ fontSize: '1.1rem', margin: 0 }}>Transfer Detail</h2>
          <p style={{ fontSize: '0.75rem', color: '#888', margin: '0.2rem 0 0', fontFamily: 'monospace' }}>{transfer.id}</p>
        </div>
        <button onClick={onClose} style={closeBtnStyle} aria-label="Close">✕</button>
      </div>

      {/* Risk badges */}
      {badges.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.4rem', marginBottom: '1rem' }}>
          {badges.map(b => <RiskBadgeChip key={b.label} badge={b} />)}
        </div>
      )}

      {/* Amounts */}
      <section style={sectionStyle}>
        <h3 style={sectionTitleStyle}>Amounts</h3>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.5rem' }}>
          <Field label="Entered amount" value={`$${transfer.amount.toFixed(2)}`} highlight={isAmountMismatch} />
          {(isAmountMismatch || recognizedAmount !== undefined) && (
            <Field
              label="Recognized amount (OCR)"
              value={recognizedAmount !== undefined ? `$${recognizedAmount.toFixed(2)}` : '—'}
              highlight={isAmountMismatch}
            />
          )}
        </div>
        {isAmountMismatch && (
          <p style={{ color: '#c00', fontSize: '0.8rem', marginTop: '0.4rem' }}>
            Amounts do not match — review required.
          </p>
        )}
      </section>

      {/* MICR data */}
      <section style={sectionStyle}>
        <h3 style={sectionTitleStyle}>MICR Data</h3>
        {isMicrFail ? (
          <p style={{ color: '#c00', fontSize: '0.9rem' }}>MICR unreadable — read failure reported by VSS.</p>
        ) : transfer.micr_data ? (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '0.5rem' }}>
            <Field label="Routing" value={maskNumber(transfer.micr_data.routing)} />
            <Field label="Account" value={maskNumber(transfer.micr_data.account)} />
            <Field label="Check #" value={transfer.micr_data.check_number} />
          </div>
        ) : (
          <p style={{ color: '#888', fontSize: '0.9rem' }}>No MICR data available.</p>
        )}
      </section>

      {/* Confidence score */}
      <section style={sectionStyle}>
        <h3 style={sectionTitleStyle}>VSS Confidence Score</h3>
        {transfer.confidence_score !== null && transfer.confidence_score !== undefined ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <span style={{
              fontSize: '1.5rem',
              fontWeight: 700,
              color: transfer.confidence_score < 0.90 ? '#b45309' : '#1a7a3c',
            }}>
              {(transfer.confidence_score * 100).toFixed(0)}%
            </span>
            {transfer.confidence_score < 0.90 && (
              <span style={{ fontSize: '0.8rem', color: '#b45309' }}>Below 90% threshold</span>
            )}
          </div>
        ) : (
          <p style={{ color: '#888', fontSize: '0.9rem' }}>Not available</p>
        )}
      </section>

      {/* Check images */}
      <section style={sectionStyle}>
        <h3 style={sectionTitleStyle}>Check Images</h3>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
          <div>
            <p style={imgLabelStyle}>Front</p>
            <CheckImage transferId={transfer.id} side="front" imageRef={transfer.front_image_ref} />
          </div>
          <div>
            <p style={imgLabelStyle}>Back</p>
            <CheckImage transferId={transfer.id} side="back" imageRef={transfer.back_image_ref} />
          </div>
        </div>
      </section>

      {/* Transfer info */}
      <section style={sectionStyle}>
        <h3 style={sectionTitleStyle}>Transfer Info</h3>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.5rem' }}>
          <Field label="State" value={transfer.state} />
          <Field label="Review reason" value={transfer.review_reason ?? '—'} />
          <Field label="Submitted" value={new Date(transfer.submitted_at).toLocaleString()} />
          <Field label="Currency" value={transfer.currency} />
          {transfer.contribution_type && (
            <Field label="Contribution type" value={transfer.contribution_type} />
          )}
        </div>
      </section>

      {/* Decision trace */}
      <section style={sectionStyle}>
        <h3 style={sectionTitleStyle}>Decision Trace</h3>
        {eventsLoading ? (
          <p style={{ color: '#888', fontSize: '0.9rem' }}>Loading events…</p>
        ) : events.length === 0 ? (
          <p style={{ color: '#888', fontSize: '0.9rem' }}>No events recorded.</p>
        ) : (
          <ol style={{ margin: 0, padding: '0 0 0 1.25rem', display: 'flex', flexDirection: 'column', gap: '0.4rem' }}>
            {events.map(ev => (
              <li key={ev.id} style={{ fontSize: '0.85rem' }}>
                <span style={{ fontFamily: 'monospace', background: '#f0f0f0', padding: '0.1rem 0.3rem', borderRadius: '3px' }}>
                  {ev.event_type}
                </span>
                {' '}
                <span style={{ color: '#888' }}>{new Date(ev.created_at).toLocaleTimeString()}</span>
                {ev.data && Object.keys(ev.data).length > 0 && (
                  <details style={{ marginTop: '0.2rem' }}>
                    <summary style={{ cursor: 'pointer', color: '#555', fontSize: '0.8rem' }}>data</summary>
                    <pre style={preStyle}>{JSON.stringify(ev.data, null, 2)}</pre>
                  </details>
                )}
              </li>
            ))}
          </ol>
        )}
      </section>

      {/* Operator actions */}
      <section style={{ ...sectionStyle, borderTop: '2px solid #1a1a2e', paddingTop: '1rem' }}>
        <h3 style={sectionTitleStyle}>Operator Decision</h3>
        <OperatorActions transfer={transfer} onReviewed={onReviewed} />
      </section>
    </div>
  )
}

function Field({ label, value, highlight }: { label: string; value: string; highlight?: boolean }) {
  return (
    <div>
      <dt style={{ fontSize: '0.75rem', color: '#888', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.03em' }}>{label}</dt>
      <dd style={{ margin: '0.1rem 0 0', fontSize: '0.9rem', fontWeight: highlight ? 700 : 400, color: highlight ? '#c00' : '#213547' }}>{value}</dd>
    </div>
  )
}

/** Show last 4 digits of a number string, masking the rest. */
function maskNumber(s: string): string {
  if (s.length <= 4) return s
  return '*'.repeat(s.length - 4) + s.slice(-4)
}

const panelStyle: React.CSSProperties = {
  background: '#fff',
  border: '1px solid #ddd',
  borderRadius: '8px',
  padding: '1.25rem',
  overflowY: 'auto',
  maxHeight: 'calc(100vh - 120px)',
}

const sectionStyle: React.CSSProperties = {
  marginBottom: '1.25rem',
}

const sectionTitleStyle: React.CSSProperties = {
  fontSize: '0.8rem',
  fontWeight: 700,
  textTransform: 'uppercase',
  letterSpacing: '0.05em',
  color: '#1a1a2e',
  marginBottom: '0.5rem',
}

const closeBtnStyle: React.CSSProperties = {
  background: 'none',
  border: 'none',
  cursor: 'pointer',
  fontSize: '1.1rem',
  color: '#888',
  padding: '0.25rem',
  lineHeight: 1,
}

const imgPlaceholder: React.CSSProperties = {
  height: '120px',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  background: '#f5f5f5',
  border: '1px dashed #ccc',
  borderRadius: '4px',
  fontSize: '0.8rem',
  color: '#888',
}

const imgLabelStyle: React.CSSProperties = {
  fontSize: '0.75rem',
  fontWeight: 600,
  color: '#888',
  marginBottom: '0.4rem',
  textTransform: 'uppercase',
}

const preStyle: React.CSSProperties = {
  background: '#f5f5f5',
  padding: '0.4rem',
  borderRadius: '3px',
  fontSize: '0.75rem',
  overflowX: 'auto',
  margin: '0.2rem 0 0',
}
