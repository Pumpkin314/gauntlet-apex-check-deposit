import { useState } from 'react'
import { apiFetch } from '../api/client'
import type { QueueTransfer } from '../pages/AdminQueue'

interface Props {
  transfer: QueueTransfer
  onReviewed: (transferId: string) => void
}

const CONTRIBUTION_TYPES = ['INDIVIDUAL', 'ROLLOVER', 'EMPLOYER']

export default function OperatorActions({ transfer, onReviewed }: Props) {
  const [action, setAction] = useState<'APPROVE' | 'REJECT' | 'REVALIDATE' | null>(null)
  const [reason, setReason] = useState('')
  const [contributionOverride, setContributionOverride] = useState(
    transfer.contribution_type_override ?? transfer.contribution_type ?? ''
  )
  const [submitting, setSubmitting] = useState(false)
  const [alreadyReviewed, setAlreadyReviewed] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const isIra = transfer.contribution_type !== null

  async function handleSubmit() {
    if (!action) return
    if (action === 'REJECT' && !reason.trim()) return

    setSubmitting(true)
    setSubmitError(null)
    try {
      await apiFetch('/operator/actions', {
        method: 'POST',
        body: JSON.stringify({
          transfer_id: transfer.id,
          action,
          reason: reason.trim() || (action === 'REVALIDATE' ? 'Re-scan requested' : `Operator ${action.toLowerCase()}`),
          ...(isIra && contributionOverride ? { contribution_type_override: contributionOverride } : {}),
        }),
      })
      onReviewed(transfer.id)
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Unknown error'
      if (msg.includes('409') || msg.toLowerCase().includes('already')) {
        setAlreadyReviewed(true)
      } else {
        setSubmitError(msg)
      }
      setSubmitting(false)
    }
  }

  if (alreadyReviewed) {
    return (
      <div style={noticeStyle}>
        This transfer has already been reviewed by another operator.
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
      {isIra && (
        <div>
          <label style={labelStyle}>Contribution type override</label>
          <select
            value={contributionOverride}
            onChange={e => setContributionOverride(e.target.value)}
            disabled={submitting}
            style={selectStyle}
          >
            <option value="">— no override —</option>
            {CONTRIBUTION_TYPES.map(ct => (
              <option key={ct} value={ct}>{ct}</option>
            ))}
          </select>
        </div>
      )}

      {action === 'REJECT' && (
        <div>
          <label style={labelStyle}>Rejection reason <span style={{ color: '#c00' }}>*</span></label>
          <textarea
            value={reason}
            onChange={e => setReason(e.target.value)}
            placeholder="Required: enter reason for rejection"
            disabled={submitting}
            rows={3}
            style={textareaStyle}
          />
        </div>
      )}

      {action === 'APPROVE' && (
        <div>
          <label style={labelStyle}>Note (optional)</label>
          <textarea
            value={reason}
            onChange={e => setReason(e.target.value)}
            placeholder="Optional approval note"
            disabled={submitting}
            rows={2}
            style={textareaStyle}
          />
        </div>
      )}

      {action === 'REVALIDATE' && (
        <div>
          <label style={labelStyle}>Re-validation reason</label>
          <textarea
            value={reason}
            onChange={e => setReason(e.target.value)}
            placeholder="e.g., Customer resubmitted clearer images"
            disabled={submitting}
            rows={2}
            style={textareaStyle}
          />
        </div>
      )}

      {submitError && (
        <p style={{ color: '#c00', fontSize: '0.85rem', margin: 0 }}>{submitError}</p>
      )}

      <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
        {action === null && (
          <>
            <button onClick={() => setAction('APPROVE')} disabled={submitting} style={btnStyle('#1a7a3c', submitting)}>
              Approve
            </button>
            <button onClick={() => setAction('REJECT')} disabled={submitting} style={btnStyle('#c00', submitting)}>
              Reject
            </button>
            {transfer.state === 'Analyzing' && (
              <button onClick={() => setAction('REVALIDATE')} disabled={submitting} style={btnStyle('#2563eb', submitting)}>
                Revalidate
              </button>
            )}
          </>
        )}
        {action === 'APPROVE' && (
          <>
            <button onClick={handleSubmit} disabled={submitting} style={btnStyle('#1a7a3c', submitting)}>
              {submitting ? 'Approving…' : 'Confirm Approve'}
            </button>
            <button onClick={() => { setAction(null); setReason('') }} disabled={submitting} style={btnStyle('#666', submitting)}>
              Cancel
            </button>
          </>
        )}
        {action === 'REJECT' && (
          <>
            <button onClick={handleSubmit} disabled={submitting || !reason.trim()} style={btnStyle('#c00', submitting || !reason.trim())}>
              {submitting ? 'Rejecting…' : 'Confirm Reject'}
            </button>
            <button onClick={() => { setAction(null); setReason('') }} disabled={submitting} style={btnStyle('#666', submitting)}>
              Cancel
            </button>
          </>
        )}
        {action === 'REVALIDATE' && (
          <>
            <button onClick={handleSubmit} disabled={submitting} style={btnStyle('#2563eb', submitting)}>
              {submitting ? 'Sending…' : 'Confirm Revalidate'}
            </button>
            <button onClick={() => { setAction(null); setReason('') }} disabled={submitting} style={btnStyle('#666', submitting)}>
              Cancel
            </button>
          </>
        )}
      </div>
    </div>
  )
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: '0.8rem',
  fontWeight: 600,
  color: '#555',
  marginBottom: '0.25rem',
}

const selectStyle: React.CSSProperties = {
  width: '100%',
  padding: '0.4rem 0.6rem',
  border: '1px solid #ccc',
  borderRadius: '4px',
  fontSize: '0.9rem',
}

const textareaStyle: React.CSSProperties = {
  width: '100%',
  padding: '0.4rem 0.6rem',
  border: '1px solid #ccc',
  borderRadius: '4px',
  fontSize: '0.9rem',
  resize: 'vertical',
}

const noticeStyle: React.CSSProperties = {
  padding: '0.75rem',
  background: '#fff3cd',
  border: '1px solid #ffc107',
  borderRadius: '4px',
  fontSize: '0.9rem',
  color: '#856404',
}

function btnStyle(bg: string, disabled: boolean): React.CSSProperties {
  return {
    padding: '0.5rem 1.25rem',
    background: disabled ? '#aaa' : bg,
    color: '#fff',
    border: 'none',
    borderRadius: '4px',
    cursor: disabled ? 'not-allowed' : 'pointer',
    fontWeight: 600,
    fontSize: '0.9rem',
  }
}
