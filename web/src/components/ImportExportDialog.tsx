import { ChangeEvent, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError, getToken } from '../lib/api'
import { saveAsFile } from '../lib/saveAsFile'
import { splitSQL } from '../lib/sqlSplit'
import { useActiveConn } from '../store/activeConn'

interface Props {
  db: string
  table: string
  onClose: () => void
  onImported: () => void
}

export default function ImportExportDialog({ db, table, onClose, onImported }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [format, setFormat] = useState<'csv' | 'sql'>('csv')
  const [importFormat, setImportFormat] = useState<'csv' | 'sql'>('csv')
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  function endpoint(path: string) {
    return `/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}${path}`
  }

  async function download() {
    setError(null)
    const res = await fetch(endpoint(`/export?format=${format}`), {
      headers: { Authorization: `Bearer ${getToken() ?? ''}` },
    })
    if (!res.ok) {
      setError(`download failed: HTTP ${res.status}`)
      return
    }
    const blob = await res.blob()
    await saveAsFile(blob, `${table}.${format}`)
  }

  async function uploadCSV(file: File) {
    const fd = new FormData()
    fd.append('file', file)
    const res = await fetch(endpoint('/import'), {
      method: 'POST',
      headers: { Authorization: `Bearer ${getToken() ?? ''}` },
      body: fd,
    })
    const json = await res.json()
    if (!res.ok) throw new Error(json.error ?? `HTTP ${res.status}`)
    setMessage(`inserted ${json.rows_inserted ?? 0} rows; ${json.errors?.length ?? 0} errors`)
  }

  async function uploadSQL(file: File) {
    const text = await file.text()
    const stmts = splitSQL(text)
    if (stmts.length === 0) {
      setMessage('no statements found')
      return
    }
    let ok = 0
    const errs: string[] = []
    for (let i = 0; i < stmts.length; i++) {
      setProgress(`${i + 1} / ${stmts.length}`)
      try {
        await api.post('/api/query', { conn_id: connId, database_name: db, sql: stmts[i] })
        ok++
      } catch (err) {
        errs.push(`#${i + 1}: ${err instanceof ApiError ? err.message : String(err)}`)
      }
    }
    setProgress(null)
    const msg = `executed ${ok}/${stmts.length} statements`
    if (errs.length > 0) {
      setMessage(msg)
      setError(errs.slice(0, 5).join('\n') + (errs.length > 5 ? `\n... +${errs.length - 5} more` : ''))
    } else {
      setMessage(msg)
    }
  }

  async function upload(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setBusy(true)
    setError(null)
    setMessage(null)
    setProgress(null)
    try {
      if (importFormat === 'sql') await uploadSQL(file)
      else await uploadCSV(file)
      onImported()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'import failed')
    } finally {
      setBusy(false)
      setProgress(null)
      e.target.value = ''
    }
  }

  return (
    <div style={backdrop}>
      <div data-modal style={modal}>
        <div style={title}>import/export · {table}</div>
        <div style={section}>
          <label style={label}>
            export format
            <select value={format} onChange={(e) => setFormat(e.target.value as 'csv' | 'sql')} style={select}>
              <option value="csv">CSV</option>
              <option value="sql">SQL dump</option>
            </select>
          </label>
          <button onClick={() => void download()}>download</button>
        </div>
        <div style={section}>
          <label style={label}>
            import format
            <select value={importFormat} onChange={(e) => setImportFormat(e.target.value as 'csv' | 'sql')} style={select}>
              <option value="csv">CSV</option>
              <option value="sql">SQL</option>
            </select>
          </label>
          <input
            type="file"
            accept={importFormat === 'sql' ? '.sql,application/sql,text/plain' : '.csv,text/csv'}
            disabled={busy}
            onChange={(e) => void upload(e)}
          />
        </div>
        {progress && <div style={{ fontSize: 13, color: 'var(--text-muted)', marginTop: 4 }}>running… {progress}</div>}
        {message && <div style={ok}>{message}</div>}
        {error && <div style={{ ...err, whiteSpace: 'pre-wrap' }}>{error}</div>}
        <div style={actions}>
          <button onClick={onClose}>close</button>
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
  width: 460, maxWidth: 'calc(100vw - 32px)',
  padding: 16, borderRadius: 8, boxShadow: '0 12px 40px rgba(0,0,0,0.4)',
  border: '1px solid var(--border-color)', fontFamily: 'system-ui',
}
const title: CSSProperties = { fontSize: 16, fontWeight: 600, marginBottom: 14 }
const section: CSSProperties = { display: 'flex', gap: 10, alignItems: 'end', marginBottom: 14 }
const label: CSSProperties = { display: 'flex', flexDirection: 'column', gap: 4, fontSize: 12, color: 'var(--text-secondary)', flex: 1 }
const select: CSSProperties = { padding: '4px 6px', fontSize: 13 }
const actions: CSSProperties = { display: 'flex', justifyContent: 'flex-end', marginTop: 16 }
const ok: CSSProperties = { color: '#0a7a3d', fontSize: 13, marginTop: 8 }
const err: CSSProperties = { color: 'crimson', fontSize: 13, marginTop: 8 }
