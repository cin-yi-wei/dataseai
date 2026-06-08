import type { CSSProperties } from 'react'
import { useEditor } from '../store/editor'

export default function ResultPanel() {
  const result = useEditor((s) => s.result)
  const error = useEditor((s) => s.error)
  const resultLimit = useEditor((s) => s.resultLimit)
  const setResultLimit = useEditor((s) => s.setResultLimit)
  const limitControl = (
    <label style={limitLabel}>
      limit
      <select
        aria-label="row limit"
        value={resultLimit}
        onChange={(e) => setResultLimit(Number(e.target.value))}
        style={limitSelect}
      >
        {[50, 100, 200, 500, 1000].map((n) => (
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
          <span style={{ color: '#999' }}>run a query to see results here</span>
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
            <thead style={{ background: '#f4f4f4', position: 'sticky', top: 0 }}>
              <tr>
                {result.columns.map((c) => (
                  <th key={c} style={th}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.rows?.map((row, i) => (
                <tr key={i}>
                  {row.map((v, j) => (
                    <td key={j} style={td}>
                      {v === null || v === undefined ? <span style={{ color: '#999' }}>NULL</span> : String(v)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}

const panel: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%',
  borderTop: '1px solid #ddd', fontFamily: 'system-ui',
}
const status: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8,
  padding: '4px 8px', fontSize: 12, color: '#555',
  background: '#fafafa', borderBottom: '1px solid #ddd',
}
const limitLabel: CSSProperties = { display: 'inline-flex', alignItems: 'center', gap: 4, whiteSpace: 'nowrap' }
const limitSelect: CSSProperties = { fontSize: 12, padding: '1px 4px' }
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd', whiteSpace: 'nowrap' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3', whiteSpace: 'nowrap' }
