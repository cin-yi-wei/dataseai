import type { CSSProperties } from 'react'
import { useState } from 'react'
import { useEditor } from '../store/editor'
import { CopyTextModal } from './CopyTextModal'

export default function ResultPanel() {
  const result = useEditor((s) => s.result)
  const error = useEditor((s) => s.error)
  const resultLimit = useEditor((s) => s.resultLimit)
  const setResultLimit = useEditor((s) => s.setResultLimit)
  const [expand, setExpand] = useState<{ title: string; text: string } | null>(null)
  const limitControl = (
    <label style={limitLabel}>
      limit
      <select
        aria-label="row limit"
        value={resultLimit}
        onChange={(e) => setResultLimit(Number(e.target.value))}
        style={limitSelect}
      >
        {[50, 100, 200, 500, 1000, 10000].map((n) => (
          <option key={n} value={n}>{n}</option>
        ))}
      </select>
    </label>
  )

  if (error) {
    return (
      <div style={panel}>
        <div style={status}>
          <span style={{ color: 'crimson', fontFamily: 'monospace', fontSize: 13 }}>{error}</span>
          {limitControl}
        </div>
      </div>
    )
  }
  if (!result) {
    return (
      <div style={panel}>
        <div style={status}>
          <span style={{ color: 'var(--text-muted)' }}>run a query to see results here</span>
          {limitControl}
        </div>
      </div>
    )
  }
  return (
    <div style={panel}>
      <div style={status}>
        <span>
          {result.columns?.length ? (
            <>columns: {result.columns.length} · rows: {result.rows?.length ?? 0}</>
          ) : (
            <>rows_affected: {result.rows_affected}</>
          )}
          {' · '}
          {result.duration_ms} ms
          {result.truncated && <span style={{ color: '#cc7700' }}> · truncated at {result.rows?.length ?? resultLimit} rows</span>}
        </span>
        {limitControl}
      </div>
      <div style={{ flex: 1, overflow: 'auto' }}>
        {result.columns?.length > 0 && (
          <table style={{ borderCollapse: 'collapse', fontSize: 13, width: '100%' }}>
            <thead style={{ background: 'var(--table-header-bg)', position: 'sticky', top: 0 }}>
              <tr>
                {result.columns.map((c) => (
                  <th key={c} style={th} title={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.rows?.map((row, i) => (
                <tr key={i}>
                  {row.map((v, j) => {
                    const isNull = v === null || v === undefined
                    const text = isNull ? '' : String(v)
                    return (
                      <td
                        key={j}
                        style={td}
                        title={isNull ? undefined : text}
                        onClick={isNull ? undefined : () => setExpand({ title: result.columns[j] ?? '', text })}
                      >
                        {isNull ? <span style={{ color: 'var(--text-muted)' }}>NULL</span> : text}
                      </td>
                    )
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {expand && (
        <CopyTextModal title={expand.title} text={expand.text} onCancel={() => setExpand(null)} />
      )}
    </div>
  )
}

const panel: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%',
  borderTop: '1px solid var(--border-color)', fontFamily: 'system-ui',
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
const status: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8,
  padding: '4px 8px', fontSize: 12, color: 'var(--text-secondary)',
  background: 'var(--bg-secondary)', borderBottom: '1px solid var(--border-color)',
}
const limitLabel: CSSProperties = { display: 'inline-flex', alignItems: 'center', gap: 4, whiteSpace: 'nowrap' }
const limitSelect: CSSProperties = { fontSize: 12, padding: '1px 4px' }
const th: CSSProperties = {
  textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)',
  whiteSpace: 'nowrap', maxWidth: 360, overflow: 'hidden', textOverflow: 'ellipsis',
}
const td: CSSProperties = {
  padding: '4px 8px', borderBottom: '1px solid var(--table-border)',
  whiteSpace: 'nowrap', maxWidth: 360, overflow: 'hidden', textOverflow: 'ellipsis', cursor: 'pointer',
}
