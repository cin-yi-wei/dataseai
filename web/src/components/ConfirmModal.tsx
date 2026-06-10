import type { CSSProperties } from 'react'

interface Props {
  title: string
  body: string
  detail?: string
  confirmLabel: string
  cancelLabel: string
  danger?: boolean
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export default function ConfirmModal({
  title, body, detail, confirmLabel, cancelLabel, danger, busy,
  onConfirm, onCancel,
}: Props) {
  return (
    <div style={backdrop} onMouseDown={(e) => { if (e.target === e.currentTarget) onCancel() }}>
      <div data-modal style={modal}>
        <div style={titleStyle}>{title}</div>
        <div style={bodyStyle}>{body}</div>
        {detail && (
          <pre style={detailStyle}>{detail}</pre>
        )}
        <div style={actions}>
          <button onClick={onCancel} disabled={busy}>{cancelLabel}</button>
          <button
            onClick={onConfirm}
            disabled={busy}
            style={danger ? dangerButton : undefined}
          >
            {busy ? '…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

const backdrop: CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1200,
}
const modal: CSSProperties = {
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
  width: 460, maxWidth: 'calc(100vw - 32px)',
  padding: 20, borderRadius: 8, boxShadow: '0 12px 40px rgba(0,0,0,0.4)',
  border: '1px solid var(--border-color)', fontFamily: 'system-ui',
}
const titleStyle: CSSProperties = { fontSize: 16, fontWeight: 600, marginBottom: 10 }
const bodyStyle: CSSProperties = { fontSize: 14, color: 'var(--text-secondary)', marginBottom: 10, lineHeight: 1.5 }
const detailStyle: CSSProperties = {
  fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
  fontSize: 12, background: 'var(--bg-secondary)', padding: '8px 10px',
  borderRadius: 4, overflowX: 'auto', margin: '0 0 16px 0',
  border: '1px solid var(--border-color)',
}
const actions: CSSProperties = { display: 'flex', justifyContent: 'flex-end', gap: 8 }
const dangerButton: CSSProperties = {
  background: 'var(--danger)', color: 'white', borderColor: 'var(--danger)',
}
