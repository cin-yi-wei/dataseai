import type { CSSProperties } from 'react'

// 批次審核視窗：按 Ctrl+S 送出前，列出所有待寫入的變更（改 / 增 / 刪），
// 確認後才真的寫 DB。取代原本的單格 ConfirmEditModal。
export interface ReviewEdit {
  column: string
  oldValue: any
  newValue: string | null
  pkSummary: string
}
export interface ReviewInsert {
  values: Record<string, string>
}
export interface ReviewDelete {
  pkSummary: string
}

interface ReviewChangesModalProps {
  edits: ReviewEdit[]
  inserts: ReviewInsert[]
  deletes: ReviewDelete[]
  loading?: boolean
  onConfirm: () => void
  onCancel: () => void
}

function fmt(v: any): string {
  if (v === null || v === undefined) return 'NULL'
  if (v === '') return '(空字串)'
  return String(v)
}

export function ReviewChangesModal({
  edits, inserts, deletes, loading, onConfirm, onCancel,
}: ReviewChangesModalProps) {
  const total = edits.length + inserts.length + deletes.length

  return (
    <div style={backdrop} onClick={(e) => { if (e.target === e.currentTarget) onCancel() }}>
      <div data-modal style={modal}>
        <h2 style={{ marginTop: 0, marginBottom: 4, fontSize: 16 }}>確認送出變更</h2>
        <div style={{ ...muted, marginBottom: 12 }}>
          共 {total} 項：修改 {edits.length}、新增 {inserts.length}、刪除 {deletes.length}
        </div>

        {edits.length > 0 && (
          <section style={{ marginBottom: 12 }}>
            <div style={sectionTitle}>修改 {edits.length} 格</div>
            {edits.map((e, i) => (
              <div key={i} style={item}>
                <div style={muted}>
                  <span style={{ fontFamily: 'monospace' }}>{e.column}</span>
                  <span style={{ marginLeft: 8 }}>{e.pkSummary}</span>
                </div>
                <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 2 }}>
                  <span style={oldChip}>{fmt(e.oldValue)}</span>
                  <span style={{ color: 'var(--text-muted)' }}>→</span>
                  <span style={newChip}>{fmt(e.newValue)}</span>
                </div>
              </div>
            ))}
          </section>
        )}

        {inserts.length > 0 && (
          <section style={{ marginBottom: 12 }}>
            <div style={sectionTitle}>新增 {inserts.length} 列</div>
            {inserts.map((r, i) => (
              <div key={i} style={item}>
                <span style={newChip}>
                  {Object.entries(r.values).map(([k, v]) => `${k}=${fmt(v)}`).join(', ') || '(全部使用預設值)'}
                </span>
              </div>
            ))}
          </section>
        )}

        {deletes.length > 0 && (
          <section style={{ marginBottom: 12 }}>
            <div style={sectionTitle}>刪除 {deletes.length} 列</div>
            {deletes.map((d, i) => (
              <div key={i} style={item}>
                <span style={delChip}>{d.pkSummary}</span>
              </div>
            ))}
          </section>
        )}

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 8 }}>
          <button onClick={onCancel} disabled={loading}>取消</button>
          <button
            onClick={onConfirm}
            disabled={loading || total === 0}
            style={{
              background: 'var(--accent)', color: 'white', border: '1px solid var(--accent)',
              opacity: (loading || total === 0) ? 0.6 : 1,
            }}
          >
            {loading ? '送出中…' : '確認送出'}
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
const sectionTitle: CSSProperties = { fontSize: 13, fontWeight: 600, marginBottom: 4 }
const item: CSSProperties = {
  padding: '6px 8px', marginBottom: 4, borderRadius: 4,
  background: 'var(--bg-secondary)', fontSize: 12,
}
const chipBase: CSSProperties = {
  padding: '2px 6px', borderRadius: 3, fontFamily: 'monospace', fontSize: 12,
  whiteSpace: 'pre-wrap', wordBreak: 'break-all',
}
const oldChip: CSSProperties = { ...chipBase, background: 'rgba(220, 53, 69, 0.15)', borderLeft: '3px solid var(--danger)' }
const newChip: CSSProperties = { ...chipBase, background: 'rgba(40, 167, 69, 0.15)', borderLeft: '3px solid #28a745' }
const delChip: CSSProperties = { ...chipBase, background: 'rgba(220, 53, 69, 0.12)', borderLeft: '3px solid var(--danger)' }
