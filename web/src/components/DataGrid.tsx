import { useEffect, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { ColumnDef, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'
import { useContextMenu } from './useContextMenu'
import { CellContextMenu } from './CellContextMenu'
import { EditCellModal } from './EditCellModal'
import { QuickLookEditorModal } from './QuickLookEditorModal'

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
  onWantImportExport?: () => void
}

export default function DataGrid({ db, table, onWantImportExport }: Props) {
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

  const { position, cellInfo, cellValue, handleContextMenu, closeMenu } = useContextMenu()
  const [showEditModal, setShowEditModal] = useState(false)
  const [showQuickLookModal, setShowQuickLookModal] = useState(false)
  const [editingValue, setEditingValue] = useState<any>(null)

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

  async function handleMenuAction(action: string, subaction?: string) {
    if (!cellInfo || !data) return
    const colName = data.columns[cellInfo.colIdx]
    const rowIdx = cellInfo.rowIdx
    const pk = pkValuesOfRow(rowIdx)
    if (!pk) return

    try {
      switch (action) {
        case 'edit':
          setEditingValue(cellValue)
          setShowQuickLookModal(true)
          closeMenu()
          break

        case 'quick-look':
          setEditingValue(cellValue)
          setShowEditModal(true)
          closeMenu()
          break

        case 'set-value':
          if (subaction === 'EMPTY') {
            await api.patch(dataPath('/rows'), { pk_values: pk, column: colName, new_value: '' })
            reload()
          } else if (subaction === 'NULL') {
            await api.patch(dataPath('/rows'), { pk_values: pk, column: colName, new_value: null })
            reload()
          } else if (subaction === 'DEFAULT') {
            // TODO: Implement DEFAULT from column metadata
            window.alert('DEFAULT not yet implemented')
          }
          break

        case 'copy':
          try {
            // Copy entire row as tab-separated
            const rowData = data.rows[rowIdx]
            const tsvRow = rowData.map((v) => (v === null ? '' : String(v))).join('\t')
            await navigator.clipboard.writeText(tsvRow)
            window.alert('Copied!')
          } catch (err) {
            window.alert('Failed to copy: ' + (err instanceof Error ? err.message : 'Unknown error'))
          }
          break

        case 'copy-cell':
          try {
            const cellValueStr = cellValue === null ? 'NULL' : String(cellValue)
            await navigator.clipboard.writeText(cellValueStr)
            window.alert('Copied!')
          } catch (err) {
            window.alert('Failed to copy: ' + (err instanceof Error ? err.message : 'Unknown error'))
          }
          break

        case 'copy-column':
          try {
            const { copyColumnAsTabSeparated } = await import('../lib/copyFormats')
            const colIdx = cellInfo.colIdx
            const colCopy = copyColumnAsTabSeparated(colName, data.rows, colIdx)
            await navigator.clipboard.writeText(colCopy)
            window.alert('Copied!')
          } catch (err) {
            window.alert('Failed to copy: ' + (err instanceof Error ? err.message : 'Unknown error'))
          }
          break

        case 'copy-as':
          try {
            const copyFormats = await import('../lib/copyFormats')
            const rowData2 = data.rows[rowIdx]
            let copyText = ''
            if (subaction === 'JSON') {
              copyText = copyFormats.copyAsJson(cellValue)
            } else if (subaction === 'TSV for Excel') {
              copyText = copyFormats.copyAsTsv(cellValue)
            } else if (subaction === 'Markdown') {
              copyText = copyFormats.copyAsMarkdown(cellValue)
            } else if (subaction === 'Insert statement') {
              copyText = copyFormats.copyAsInsertStatement(rowData2, data.columns, table)
            }
            if (copyText) {
              await navigator.clipboard.writeText(copyText)
              window.alert('Copied!')
            }
          } catch (err) {
            window.alert('Failed to copy: ' + (err instanceof Error ? err.message : 'Unknown error'))
          }
          break

        case 'delete-row':
          if (window.confirm('Delete this row?')) {
            await api.deleteWithBody(dataPath('/rows'), { pk_values: pk })
            reload()
          }
          break

        case 'quick-filter':
          window.alert('Quick Filter: ' + subaction + ' (coming soon)')
          break

        case 'refresh':
          reload()
          break

        case 'paste':
          try {
            const pastedText = await navigator.clipboard.readText()
            await api.patch(dataPath('/rows'), { pk_values: pk, column: colName, new_value: pastedText })
            reload()
            window.alert('Pasted!')
          } catch (err) {
            throw err // Let outer catch handle it
          }
          break

        case 'add-row':
          setAdding(true)
          break

        case 'duplicate':
          const newRow2 = { ...newRow, ...Object.fromEntries(data.columns.map((c, i) => [c, data.rows[rowIdx][i]])) }
          await insertRowWithValues(newRow2)
          break
      }
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'Operation failed')
    }
  }

  async function insertRowWithValues(values: Record<string, any>) {
    if (connId == null) return
    try {
      await api.post(dataPath('/rows'), { values })
      reload()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'insert failed')
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
        if (v === null || v === undefined) {
          return <span onDoubleClick={startEdit} onContextMenu={(e) => handleContextMenu(e, rowIdx, idx, v)} style={{ color: '#999', cursor: 'context-menu' }}>NULL</span>
        }
        if (v === '') {
          // Empty string: show placeholder dot
          return <span onDoubleClick={startEdit} onContextMenu={(e) => handleContextMenu(e, rowIdx, idx, v)} style={{ color: '#ccc', cursor: 'context-menu' }}>·</span>
        }
        return <span onDoubleClick={startEdit} onContextMenu={(e) => handleContextMenu(e, rowIdx, idx, v)} style={{ cursor: 'context-menu' }}>{String(v)}</span>
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
  }, [data, sortCol, sortDir, editing, editValue, pkCols.length, handleContextMenu])

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
        <button onClick={onWantImportExport}>import/export</button>
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

      {position && cellInfo && data && (
        <CellContextMenu
          position={position}
          cellValue={cellValue}
          columnName={data.columns[cellInfo.colIdx] || ''}
          onAction={handleMenuAction}
          onClose={closeMenu}
        />
      )}

      {showEditModal && cellInfo && data && (
        <EditCellModal
          value={editingValue}
          columnName={data.columns[cellInfo.colIdx] || ''}
          onApply={async (newValue) => {
            if (!cellInfo || !data || connId == null) return
            const colName = data.columns[cellInfo.colIdx]
            const pk = pkValuesOfRow(cellInfo.rowIdx)
            if (!pk) return
            await api.patch(dataPath('/rows'), {
              pk_values: pk,
              column: colName,
              new_value: newValue === '' ? '' : newValue,
            })
            reload()
            setShowEditModal(false)
          }}
          onCancel={() => setShowEditModal(false)}
        />
      )}

      {showQuickLookModal && cellInfo && data && (
        <QuickLookEditorModal
          value={editingValue}
          columnName={data.columns[cellInfo.colIdx] || ''}
          onApply={async (newValue) => {
            if (!cellInfo || !data || connId == null) return
            const colName = data.columns[cellInfo.colIdx]
            const pk = pkValuesOfRow(cellInfo.rowIdx)
            if (!pk) return
            await api.patch(dataPath('/rows'), {
              pk_values: pk,
              column: colName,
              new_value: newValue,
            })
            reload()
            setShowQuickLookModal(false)
          }}
          onCancel={() => setShowQuickLookModal(false)}
        />
      )}
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
