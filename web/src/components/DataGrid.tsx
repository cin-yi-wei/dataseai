import { useEffect, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { ColumnDef, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'

interface RowsPage {
  columns: string[]
  rows: any[][]
  total: number
  page: number
  per_page: number
}

interface ColumnInfo {
  name: string
  type: string
  key: string
  extra: string
}

interface Structure {
  columns: ColumnInfo[]
}

interface Props {
  db: string
  table: string
}

export default function DataGrid({ db, table }: Props) {
  const connId = useActiveConn((s) => s.activeId)
  const [data, setData] = useState<RowsPage | null>(null)
  const [structure, setStructure] = useState<Structure | null>(null)
  const [page, setPage] = useState(1)
  const [perPage] = useState(50)
  const [sortCol, setSortCol] = useState<string | null>(null)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<{ row: number; col: number } | null>(null)
  const [editValue, setEditValue] = useState('')
  const [adding, setAdding] = useState(false)
  const [newRow, setNewRow] = useState<Record<string, string>>({})

  const pkCols = useMemo(
    () => structure?.columns.filter((c) => c.key === 'PRI').map((c) => c.name) ?? [],
    [structure],
  )
  const insertableCols = useMemo(
    () => structure?.columns.filter((c) => !c.extra.toLowerCase().includes('auto_increment')).map((c) => c.name) ?? data?.columns ?? [],
    [structure, data],
  )

  function dataPath(path: string) {
    return `/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}${path}`
  }

  function reload() {
    if (connId == null) return
    setLoading(true)
    setError(null)
    const params = new URLSearchParams({
      page: String(page),
      per_page: String(perPage),
    })
    if (sortCol) {
      params.set('sort_col', sortCol)
      params.set('sort_dir', sortDir)
    }
    api
      .get<RowsPage>(dataPath(`/data?${params}`))
      .then((d) => setData({ ...d, rows: d.rows ?? [] }))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'load failed'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (connId == null) return
    setStructure(null)
    api
      .get<Structure>(dataPath('/structure'))
      .then(setStructure)
      .catch(() => setStructure({ columns: [] }))
  }, [connId, db, table])

  useEffect(reload, [connId, db, table, page, perPage, sortCol, sortDir])

  function pkValuesOfRow(rowIdx: number): Record<string, any> | null {
    if (!data || pkCols.length === 0) return null
    const pk: Record<string, any> = {}
    for (const col of pkCols) {
      const idx = data.columns.indexOf(col)
      if (idx < 0) return null
      pk[col] = data.rows[rowIdx][idx]
    }
    return pk
  }

  async function commitEdit() {
    if (!editing || !data || connId == null) return
    const col = data.columns[editing.col]
    const pk = pkValuesOfRow(editing.row)
    if (!pk) {
      setEditing(null)
      return
    }
    try {
      await api.patch<{ affected: number }>(dataPath('/rows'), {
        pk_values: pk,
        column: col,
        new_value: editValue,
      })
      reload()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'update failed')
    } finally {
      setEditing(null)
    }
  }

  async function insertRow() {
    if (connId == null) return
    const values: Record<string, string> = {}
    for (const col of insertableCols) {
      if (newRow[col] !== undefined && newRow[col] !== '') values[col] = newRow[col]
    }
    if (Object.keys(values).length === 0) return
    try {
      await api.post<{ id: number }>(dataPath('/rows'), { values })
      setAdding(false)
      setNewRow({})
      reload()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'insert failed')
    }
  }

  async function deleteRow(rowIdx: number) {
    if (connId == null) return
    const pk = pkValuesOfRow(rowIdx)
    if (!pk) return
    if (!window.confirm('Delete this row?')) return
    try {
      await api.deleteWithBody<{ affected: number }>(dataPath('/rows'), { pk_values: pk })
      reload()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'delete failed')
    }
  }

  const columns = useMemo<ColumnDef<any[]>[]>(() => {
    if (!data) return []
    const out: ColumnDef<any[]>[] = data.columns.map((name, idx) => ({
      id: name,
      header: () => (
        <span
          onClick={() => {
            if (sortCol === name) {
              setSortDir(sortDir === 'asc' ? 'desc' : 'asc')
            } else {
              setSortCol(name)
              setSortDir('asc')
            }
          }}
          style={{ cursor: 'pointer' }}
        >
          {name}{sortCol === name ? (sortDir === 'asc' ? ' ▲' : ' ▼') : ''}
        </span>
      ),
      accessorFn: (row) => row[idx],
      cell: (info) => {
        const rowIdx = info.row.index
        const v = info.getValue()
        const active = editing?.row === rowIdx && editing?.col === idx
        if (active) {
          return (
            <input
              autoFocus
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
              onBlur={() => void commitEdit()}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void commitEdit()
                if (e.key === 'Escape') setEditing(null)
              }}
              style={editInput}
            />
          )
        }
        const startEdit = pkCols.length === 0 ? undefined : () => {
          setEditing({ row: rowIdx, col: idx })
          setEditValue(v == null ? '' : String(v))
        }
        if (v === null || v === undefined) return <span onDoubleClick={startEdit} style={{ color: '#999' }}>NULL</span>
        return <span onDoubleClick={startEdit}>{String(v)}</span>
      },
    }))
    if (pkCols.length > 0) {
      out.unshift({
        id: '__actions',
        header: '',
        cell: (info) => (
          <button style={smallButton} onClick={() => void deleteRow(info.row.index)}>delete</button>
        ),
      })
    }
    return out
  }, [data, sortCol, sortDir, editing, editValue, pkCols.length])

  const tableInst = useReactTable({
    data: data?.rows ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.per_page)) : 1

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui' }}>
      <div style={toolbar}>
        <button onClick={() => setAdding((v) => !v)}>+ row</button>
        {pkCols.length === 0 && <span style={muted}>read-only edits: no primary key</span>}
        {loading && data && <span style={muted}>refreshing…</span>}
      </div>

      {adding && (
        <div style={addPanel}>
          {insertableCols.map((col) => (
            <label key={col} style={fieldLabel}>
              <span>{col}</span>
              <input
                value={newRow[col] ?? ''}
                onChange={(e) => setNewRow((r) => ({ ...r, [col]: e.target.value }))}
                style={fieldInput}
              />
            </label>
          ))}
          <button onClick={() => void insertRow()}>insert</button>
          <button onClick={() => setAdding(false)}>cancel</button>
        </div>
      )}

      <div style={{ flex: 1, overflow: 'auto' }}>
        {error && <div style={{ color: 'crimson', padding: 8 }}>{error}</div>}
        {loading && !data && <div style={{ color: '#999', padding: 8 }}>loading…</div>}
        {data && (
          <table style={{ borderCollapse: 'collapse', fontSize: 13, width: '100%' }}>
            <thead style={{ background: '#f4f4f4', position: 'sticky', top: 0 }}>
              {tableInst.getHeaderGroups().map((hg) => (
                <tr key={hg.id}>
                  {hg.headers.map((h) => (
                    <th key={h.id} style={th}>{flexRender(h.column.columnDef.header, h.getContext())}</th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {tableInst.getRowModel().rows.map((row) => (
                <tr key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} style={td}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      <div style={pager}>
        <button disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>‹ prev</button>
        <span>page {data?.page ?? 1} / {totalPages} · {data?.total ?? 0} rows total · {perPage}/page</span>
        <button disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>next ›</button>
      </div>
    </div>
  )
}

const toolbar: CSSProperties = {
  display: 'flex', gap: 8, alignItems: 'center', padding: 6,
  borderBottom: '1px solid #e5e5e5', fontSize: 12,
}
const addPanel: CSSProperties = {
  display: 'flex', gap: 8, alignItems: 'end', flexWrap: 'wrap',
  padding: 8, borderBottom: '1px solid #e5e5e5', background: '#fafafa',
}
const fieldLabel: CSSProperties = { display: 'flex', flexDirection: 'column', gap: 2, fontSize: 11, color: '#555' }
const fieldInput: CSSProperties = { width: 140, boxSizing: 'border-box', fontSize: 12, padding: '3px 5px' }
const editInput: CSSProperties = { width: '100%', minWidth: 80, boxSizing: 'border-box', fontSize: 13, padding: '2px 4px' }
const smallButton: CSSProperties = { fontSize: 11, padding: '2px 6px' }
const muted: CSSProperties = { color: '#777' }
const pager: CSSProperties = {
  display: 'flex', gap: 8, alignItems: 'center', padding: 6,
  borderTop: '1px solid #ddd', fontSize: 12,
}
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid #ddd', whiteSpace: 'nowrap' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid #f3f3f3', whiteSpace: 'nowrap' }
