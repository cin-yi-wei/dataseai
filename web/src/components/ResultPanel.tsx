import type { CSSProperties } from 'react'
import { useEditor } from '../store/editor'

export default function ResultPanel() {
  const result = useEditor((s) => s.result)
  const error = useEditor((s) => s.error)

  if (error) {
    return (
      <div style={panel}>
        <div style={{ color: 'crimson', padding: 8, fontFamily: 'monospace', fontSize: 13 }}>{error}</div>
      </div>
    )
  }
  if (!result) {
    return (
      <div style={{ ...panel, color: '#999' }}>
        run a query to see results here
      </div>
    )
  }
  return (
    <div style={panel}>
      <div style={status}>
        {result.columns?.length ? (
          <>columns: {result.columns.length} · rows: {result.rows?.length ?? 0}</>
        ) : (
          <>rows_affected: {result.rows_affected}</>
        )}
        {' · '}
        {result.duration_ms} ms
        {result.truncated && <span style={{ color: '#cc7700' }}> · ⚠ truncated at 10 000 rows</span>}
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
  padding: '4px 8px', fontSize: 12, color: '#555',
  background: '#fafafa', borderBottom: '1px solid #ddd',
}
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd', whiteSpace: 'nowrap' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3', whiteSpace: 'nowrap' }
