import type { CSSProperties } from 'react'

interface ConfirmEditModalProps {
  column: string
  oldValue: any
  newValue: string
  pkValues: Record<string, any>
  loading?: boolean
  onConfirm: () => void
  onCancel: () => void
}

function formatValue(v: any): string {
  if (v === null || v === undefined) return 'NULL'
  if (v === '') return '(empty string)'
  return String(v)
}

export function ConfirmEditModal({
  column, oldValue, newValue, pkValues, loading, onConfirm, onCancel,
}: ConfirmEditModalProps) {
  const oldStr = formatValue(oldValue)
  const newStr = newValue === '' ? '(empty string)' : newValue
  const unchanged = oldStr === newStr

  return (
    <div style={backdrop} onClick={(e) => { if (e.target === e.currentTarget) onCancel() }}>
      <div data-modal style={modal}>
        <h2 style={{ marginTop: 0, marginBottom: 12, fontSize: 16 }}>Confirm Edit</h2>

        <div style={{ fontSize: 13, marginBottom: 12 }}>
          <div style={muted}>Column:</div>
          <div style={{ fontFamily: 'monospace', marginBottom: 8 }}>{column}</div>

          <div style={muted}>Primary key:</div>
          <div style={{ fontFamily: 'monospace', marginBottom: 8 }}>
            {Object.entries(pkValues).map(([k, v]) => `${k}=${v}`).join(', ')}
          </div>
        </div>

        <div style={diffRow}>
          <div style={diffLabel}>Old:</div>
          <pre style={diffOld}>{oldStr}</pre>
        </div>
        <div style={diffRow}>
          <div style={diffLabel}>New:</div>
          <pre style={diffNew}>{newStr}</pre>
        </div>

        {unchanged && (
          <div style={{ color: 'var(--text-muted)', fontSize: 12, marginTop: 8 }}>
            ⚠️ No changes — value is the same as before.
          </div>
        )}

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 16 }}>
          <button onClick={onCancel} disabled={loading}>Cancel</button>
          <button
            onClick={onConfirm}
            disabled={loading || unchanged}
            style={{
              background: 'var(--accent)', color: 'white',
              border: '1px solid var(--accent)',
              opacity: (loading || unchanged) ? 0.6 : 1,
            }}
          >
            {loading ? 'Saving…' : 'Confirm Save'}
          </button>
        </div>
      </div>
    </div>
  )
}

const backdrop: CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1100,
}
const modal: CSSProperties = {
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
  border: '1px solid var(--border-color)',
  borderRadius: 8, padding: 20, minWidth: 480, maxWidth: '90vw', maxHeight: '80vh',
  display: 'flex', flexDirection: 'column', overflow: 'auto',
}
const muted: CSSProperties = { color: 'var(--text-muted)', fontSize: 11 }
const diffRow: CSSProperties = { display: 'flex', flexDirection: 'column', marginBottom: 6 }
const diffLabel: CSSProperties = { fontSize: 11, color: 'var(--text-muted)', marginBottom: 2 }
const diffOld: CSSProperties = {
  margin: 0, padding: 8, fontFamily: 'monospace', fontSize: 12,
  background: 'rgba(220, 53, 69, 0.15)', borderLeft: '3px solid var(--danger)',
  borderRadius: 3, whiteSpace: 'pre-wrap', wordBreak: 'break-all',
  maxHeight: 150, overflow: 'auto',
}
const diffNew: CSSProperties = {
  margin: 0, padding: 8, fontFamily: 'monospace', fontSize: 12,
  background: 'rgba(40, 167, 69, 0.15)', borderLeft: '3px solid #28a745',
  borderRadius: 3, whiteSpace: 'pre-wrap', wordBreak: 'break-all',
  maxHeight: 150, overflow: 'auto',
}
