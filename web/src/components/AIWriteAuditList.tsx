import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export interface AuditRow {
  id: number
  database_name: string
  table_name: string
  operation: string
  sql_text: string
  status: 'proposed' | 'executed' | 'denied' | 'cancelled' | 'failed'
  rows_affected: number | null
  error_message: string
  created_at: string
}

const CHIP_COLOR: Record<AuditRow['status'], string> = {
  executed:  '#3a8',
  denied:    '#c44',
  cancelled: '#888',
  failed:    '#e90',
  proposed:  '#48a',
}

export default function AIWriteAuditList({ rows }: { rows: AuditRow[] }) {
  const t = useT()
  if (!rows.length) return <p style={muted}>—</p>
  return (
    <ul style={list}>
      {rows.map((r) => {
        const sqlText = r.sql_text ?? ''
        const created = r.created_at ?? ''
        const status = (r.status ?? 'proposed') as AuditRow['status']
        return (
          <li key={r.id} style={item}>
            <span style={{ ...chip, background: CHIP_COLOR[status] ?? '#666' }}>
              {t(`settings.ai_writes.status.${status}` as const)}
            </span>
            <span style={timeCell}>{created.slice(0, 19).replace('T', ' ')}</span>
            <span>{(r.database_name ?? '?')}.{(r.table_name ?? '?')}</span>
            <span style={op}>{r.operation ?? '—'}</span>
            {r.rows_affected != null && <span>rows={r.rows_affected}</span>}
            {r.error_message && <span style={err}>{r.error_message}</span>}
            <pre style={sql} title={sqlText}>{sqlText.slice(0, 120)}{sqlText.length > 120 ? '…' : ''}</pre>
          </li>
        )
      })}
    </ul>
  )
}

const list: CSSProperties = { listStyle: 'none', padding: 0, margin: 0, fontSize: 12 }
const item: CSSProperties = { display: 'grid', gridTemplateColumns: '80px 130px 1fr 60px auto auto 1fr', gap: 8, padding: '4px 0', borderBottom: '1px solid var(--border-color)' }
const chip: CSSProperties = { color: 'white', padding: '1px 6px', borderRadius: 4, fontSize: 11, textAlign: 'center' }
const timeCell: CSSProperties = { color: 'var(--text-muted, #888)' }
const op: CSSProperties = { fontWeight: 600 }
const err: CSSProperties = { color: '#c44', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }
const sql: CSSProperties = { margin: 0, fontFamily: 'monospace', color: 'var(--text-secondary)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }
const muted: CSSProperties = { color: 'var(--text-muted, #888)' }
