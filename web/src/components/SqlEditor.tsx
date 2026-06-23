import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import CodeMirror, { ReactCodeMirrorRef } from '@uiw/react-codemirror'
import { sql, MySQL, MariaSQL, PostgreSQL, MSSQL, SQLite, PLSQL, StandardSQL, keywordCompletionSource, schemaCompletionSource } from '@codemirror/lang-sql'
import type { SQLDialect } from '@codemirror/lang-sql'
import { CompletionContext, CompletionResult, autocompletion } from '@codemirror/autocomplete'
import { vscodeDark, vscodeLight } from '@uiw/codemirror-theme-vscode'
import { api, ApiError, getToken } from '../lib/api'
import { streamQuery } from '../lib/wsQuery'
import { useActiveConn } from '../store/activeConn'
import { useConnections } from '../store/connections'
import type { ConnectionEngine } from '../store/connections'

// Map a connection engine to the CodeMirror SQL dialect so autocomplete
// quotes identifiers correctly (MySQL backticks vs MSSQL [] vs PG/standard
// double-quotes) and highlights with the right grammar.
function dialectForEngine(engine: ConnectionEngine | undefined): SQLDialect {
  switch (engine) {
    case 'mysql':
    case 'tidb':
    case 'planetscale':
    case 'singlestore':
      return MySQL
    case 'mariadb':
      return MariaSQL
    case 'postgres':
    case 'cockroachdb':
    case 'redshift':
      return PostgreSQL
    case 'mssql':
      return MSSQL
    case 'sqlite':
    case 'duckdb':
      return SQLite
    case 'oracle':
      return PLSQL
    default:
      // bytehouse / clickhouse / snowflake / unknown — standard SQL uses
      // double-quote identifiers, never MySQL backticks.
      return StandardSQL
  }
}
import { useEditor, QueryResult } from '../store/editor'
import { useTheme } from '../store/theme'
import { useT } from '../i18n'

interface Props {
  onShowHistory: () => void
  database?: string
}

export default function SqlEditor({ onShowHistory, database }: Props) {
  const t = useT()
  const connId = useActiveConn((s) => s.activeId)
  const theme = useTheme((s) => s.theme)
  const connections = useConnections((s) => s.list)
  const dialect = useMemo(
    () => dialectForEngine(connections.find((c) => c.id === connId)?.engine),
    [connections, connId],
  )
  const [schema, setSchema] = useState<Record<string, string[]>>({})
  const editorRef = useRef<ReactCodeMirrorRef>(null)

  useEffect(() => {
    if (connId == null || !database) {
      setSchema({})
      return
    }
    api.get<{ tables: Record<string, string[]> }>(`/api/db/${connId}/databases/${encodeURIComponent(database)}/schema`)
      .then((r) => {
        const t = r.tables ?? {}
        console.log('[SqlEditor] schema loaded:', Object.keys(t).length, 'tables. Example:', Object.entries(t)[0])
        setSchema(t)
      })
      .catch((err) => {
        console.error('[SqlEditor] schema load failed:', err)
        setSchema({})
      })
  }, [connId, database])

  const sqlExt = useMemo(() => {
    return sql({
      dialect,
      schema: schema as any,
      upperCaseKeywords: true,
    })
  }, [schema, dialect])

  // Context-aware column completion: when the cursor is in WHERE/SELECT/etc.
  // and the SQL has FROM/JOIN tables, suggest those tables' columns even
  // without a `table.` prefix.
  const contextAutocomplete = useMemo(() => {
    const source = (ctx: CompletionContext): CompletionResult | null => {
      const word = ctx.matchBefore(/[\w]+$/)
      if (!word && !ctx.explicit) return null
      const text = ctx.state.doc.sliceString(0, ctx.pos)
      // Don't fire after `table.` — built-in schema completion handles that.
      if (/[\w`]\.[\w]*$/.test(text.slice(-50))) return null
      const tables = extractTablesFromSQL(text, schema)
      if (tables.length === 0) return null
      const seen = new Set<string>()
      const options: { label: string; type: string; detail: string; boost: number }[] = []
      for (const t of tables) {
        const cols = schema[t]
        if (!cols) continue
        for (const c of cols) {
          if (seen.has(c)) continue
          seen.add(c)
          options.push({ label: c, type: 'property', detail: t, boost: 50 })
        }
      }
      if (options.length === 0) return null
      return {
        from: word ? word.from : ctx.pos,
        options,
        validFor: /^[\w]*$/,
      }
    }
    // Include SQL's default sources (keywords + schema) so they keep working.
    const keywordSrc = keywordCompletionSource(dialect, true)
    const schemaSrc = schemaCompletionSource({ schema: schema as any, dialect })
    return autocompletion({ override: [keywordSrc, schemaSrc, source] })
  }, [schema, dialect])

  const draft = useEditor((s) => s.draft)
  const setDraft = useEditor((s) => s.setDraft)
  const setResult = useEditor((s) => s.setResult)
  const setError = useEditor((s) => s.setError)
  const busy = useEditor((s) => s.busy)
  const setBusy = useEditor((s) => s.setBusy)
  const running = useEditor((s) => s.running)
  const setRunning = useEditor((s) => s.setRunning)
  const appendRows = useEditor((s) => s.appendRows)
  const resultLimit = useEditor((s) => s.resultLimit)

  const run = useCallback(async () => {
    if (connId == null || !draft.trim()) return
    // Determine the SQL to run: if the user has a selection, use it.
    // Otherwise, find the statement containing the cursor (split by `;`).
    const view = editorRef.current?.view
    let sqlToRun = draft
    if (view) {
      const { from, to } = view.state.selection.main
      if (from !== to) {
        // User selected text — run just that.
        sqlToRun = view.state.doc.sliceString(from, to).trim()
      } else {
        sqlToRun = getStatementAtCursor(draft, from)
      }
    }
    if (!sqlToRun.trim()) return
    setBusy(true)
    setError(null)
    let streaming = false
    try {
      const res = await api.post<QueryResult>('/api/query', {
        conn_id: connId,
        database_name: database ?? '',
        sql: sqlToRun,
        max_rows: resultLimit,
      })
      setResult(res)
    } catch (err) {
      if (err instanceof ApiError && (err.status === 408 || err.status === 413)) {
        streaming = true
        setResult({ columns: [], rows: [], rows_affected: 0, duration_ms: 0, truncated: false })
        const stream = streamQuery({
          token: getToken() ?? '',
          connId,
          db: database ?? '',
          sql: sqlToRun,
          maxRows: resultLimit,
          onEvent: (ev) => {
            if (ev.type === 'columns') {
              setResult({ columns: ev.cols ?? [], rows: [], rows_affected: 0, duration_ms: 0, truncated: false })
            } else if (ev.type === 'rows') {
              const cols = useEditor.getState().result?.columns ?? []
              appendRows(cols, ev.batch ?? [])
            } else if (ev.type === 'done') {
              const current = useEditor.getState().result
              if (current) {
                setResult({
                  ...current,
                  duration_ms: ev.durationMs ?? current.duration_ms,
                  truncated: ev.truncated ?? current.truncated,
                })
              }
              setBusy(false)
              setRunning(null)
            } else if (ev.type === 'error') {
              setError(ev.message ?? 'stream error')
              setBusy(false)
              setRunning(null)
            }
          },
          onClose: () => {
            setBusy(false)
            setRunning(null)
          },
        })
        setRunning({ queryId: stream.queryId, cancel: stream.cancel })
        return
      }
      setResult(null)
      setError(err instanceof ApiError ? err.message : 'query failed')
    } finally {
      if (!streaming) setBusy(false)
    }
  }, [connId, draft, database, appendRows, resultLimit, setBusy, setError, setResult, setRunning])

  return (
    <div style={wrap}>
      <div style={bar}>
        <button onClick={() => void run()} disabled={busy || connId == null}>
          {busy ? `⏳ ${t('sql.running')}` : `▶ ${t('sql.run')} (Ctrl+↵)`}
        </button>
        {running && <button onClick={() => running.cancel()}>{t('sql.cancel')}</button>}
        <button onClick={onShowHistory}>📜 {t('sql.history')}</button>
        <span style={{ flex: 1 }} />
        {database && <span style={{ fontSize: 12, color: '#666' }}>{t('sql.db_label', { db: database })}</span>}
      </div>
      <div style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
        <CodeMirror
          key={`cm-${Object.keys(schema).length}-${theme}-${database ?? ''}`}
          ref={editorRef}
          value={draft}
          height="100%"
          extensions={[sqlExt, contextAutocomplete]}
          theme={theme === 'dark' ? vscodeDark : vscodeLight}
          onChange={setDraft}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            autocompletion: true,
            bracketMatching: true,
            closeBrackets: true,
            highlightActiveLine: true,
            highlightSelectionMatches: true,
          }}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
              e.preventDefault()
              void run()
            }
          }}
        />
      </div>
    </div>
  )
}

const wrap: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%',
  fontFamily: 'system-ui', background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, padding: 6,
  borderBottom: '1px solid var(--border-color)', background: 'var(--bg-secondary)',
}

// getStatementAtCursor splits the SQL by `;` (ignoring those inside string
// literals or comments) and returns the statement that contains the cursor.
function getStatementAtCursor(text: string, cursor: number): string {
  const boundaries = findStatementBoundaries(text)
  // boundaries are positions of `;` plus implicit ends. The statement that
  // contains `cursor` is the one ending at the first boundary >= cursor.
  let start = 0
  for (const b of boundaries) {
    if (cursor <= b) {
      return text.slice(start, b).trim()
    }
    start = b + 1
  }
  return text.slice(start).trim()
}

// findStatementBoundaries returns positions of `;` characters that act as
// statement separators (i.e. not inside strings or comments).
function findStatementBoundaries(text: string): number[] {
  const out: number[] = []
  let i = 0
  while (i < text.length) {
    const c = text[i]
    // Line comment --
    if (c === '-' && text[i + 1] === '-') {
      while (i < text.length && text[i] !== '\n') i++
      continue
    }
    // Block comment /* */
    if (c === '/' && text[i + 1] === '*') {
      i += 2
      while (i < text.length - 1 && !(text[i] === '*' && text[i + 1] === '/')) i++
      i += 2
      continue
    }
    // Single-quoted string
    if (c === "'") {
      i++
      while (i < text.length && text[i] !== "'") {
        if (text[i] === '\\' && i + 1 < text.length) i += 2
        else i++
      }
      i++
      continue
    }
    // Double-quoted string
    if (c === '"') {
      i++
      while (i < text.length && text[i] !== '"') {
        if (text[i] === '\\' && i + 1 < text.length) i += 2
        else i++
      }
      i++
      continue
    }
    // Backtick identifier
    if (c === '`') {
      i++
      while (i < text.length && text[i] !== '`') i++
      i++
      continue
    }
    if (c === ';') {
      out.push(i)
    }
    i++
  }
  return out
}

// extractTablesFromSQL pulls table names out of FROM and JOIN clauses in the SQL
// text up to the cursor. Returns only names that exist in the schema (preserving original case).
function extractTablesFromSQL(text: string, schema: Record<string, string[]>): string[] {
  const lower = text.toLowerCase()
  const lowerToOriginal = new Map<string, string>()
  for (const k of Object.keys(schema)) lowerToOriginal.set(k.toLowerCase(), k)
  const found: string[] = []
  const seen = new Set<string>()
  const re = /(?:from|join)\s+(?:`?[\w]+`?\.)?`?([\w]+)`?/g
  let m: RegExpExecArray | null
  while ((m = re.exec(lower)) !== null) {
    const name = m[1]
    const original = lowerToOriginal.get(name)
    if (original && !seen.has(original)) {
      seen.add(original)
      found.push(original)
    }
  }
  return found
}
