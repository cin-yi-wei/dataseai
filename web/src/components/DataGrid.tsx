import { useEffect, useMemo, useRef, useState } from 'react'
import { getCurrentFilters, setCurrentFilters, pushHistory, getHistory } from '../lib/filterMemory'
import type { CSSProperties } from 'react'
import { ColumnDef, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { api, ApiError } from '../lib/api'
import { useActiveConn } from '../store/activeConn'
import { useContextMenu } from './useContextMenu'
import { CellContextMenu } from './CellContextMenu'
import { EditCellModal } from './EditCellModal'
import { QuickLookEditorModal } from './QuickLookEditorModal'
import { CopyTextModal } from './CopyTextModal'
import { FilterBar, type Filter as FilterCondition } from './FilterBar'
import { ConfirmEditModal } from './ConfirmEditModal'
import ConfirmModal from './ConfirmModal'
import { useT, tr } from '../i18n'
import { toast } from '../lib/toast'

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
  nullable: boolean
  default: string
}

interface Structure {
  columns: ColumnInfo[]
}

function isDateTimeType(type: string): boolean {
  const t = type.toLowerCase()
  return t.startsWith('datetime') || t.startsWith('timestamp')
}

// True when MySQL will reject the insert if the column is omitted: NOT NULL
// with no default and no auto-fill behavior. The DEFAULT column is empty for
// such columns; `current_timestamp` etc. show up as a non-empty default.
function isRequiredOnInsert(c: ColumnInfo): boolean {
  if (c.nullable) return false
  if (c.extra.toLowerCase().includes('auto_increment')) return false
  return c.default === ''
}

// Current time formatted as MySQL DATETIME (with microseconds). The backend
// also coerces ISO-8601 strings, but emitting MySQL format directly avoids
// any surprise.
function nowMysqlDateTime(): string {
  const d = new Date()
  const pad = (n: number, w = 2) => String(n).padStart(w, '0')
  return (
    `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ` +
    `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}.` +
    `${pad(d.getUTCMilliseconds(), 3)}000`
  )
}

interface Props {
  db: string
  table: string
  onWantImportExport?: () => void
}

const ROW_LIMIT_OPTIONS = [50, 100, 200, 500, 1000, 10000]

// Helper function to safely copy to clipboard with fallback
async function tryCopyToClipboard(text: string): Promise<boolean> {
  try {
    // 检查是否是 HTTPS 或 localhost
    const isSecure = window.location.protocol === 'https:' || window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1'

    if (!isSecure) {
      // HTTP 环境（局域网），不尝试 clipboard，直接返回 false 让 fallback 显示
      return false
    }

    if (!navigator.clipboard) {
      return false // Clipboard API not available, use fallback
    }
    await navigator.clipboard.writeText(text)
    toast(tr('common.copied'))
    return true
  } catch (err) {
    return false // Clipboard API failed, use fallback
  }
}

export default function DataGrid({ db, table, onWantImportExport }: Props) {
  const t = useT()
  const connId = useActiveConn((s) => s.activeId)
  const [data, setData] = useState<RowsPage | null>(null)
  const [structure, setStructure] = useState<Structure | null>(null)
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(50)
  const [sortCol, setSortCol] = useState<string | null>(null)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<{ row: number; col: number } | null>(null)
  const [editValue, setEditValue] = useState('')
  const [noPkHelp, setNoPkHelp] = useState(false)
  const [selectedCell, setSelectedCell] = useState<{ row: number; col: number } | null>(null)
  const [selectedRows, setSelectedRows] = useState<Set<number>>(new Set())
  const [lastClickedRow, setLastClickedRow] = useState<number | null>(null)
  const dragAnchor = useRef<number | null>(null)
  const isDragging = useRef(false)
  // 草稿列：直接顯示在資料表格內、綠底、尚未寫入 DB。新增與複製整列共用同一套：
  // 新增 = 空白草稿；複製整列 = 帶入來源列的值的草稿。按 Ctrl+S 才 INSERT。
  const [draftRows, setDraftRows] = useState<Record<string, string>[]>([])

  const { position, cellInfo, cellValue, handleContextMenu, closeMenu } = useContextMenu()
  const [showEditModal, setShowEditModal] = useState(false)
  const [showQuickLookModal, setShowQuickLookModal] = useState(false)
  const [editingValue, setEditingValue] = useState<any>(null)
  const [editingCellInfo, setEditingCellInfo] = useState<{ rowIdx: number; colIdx: number } | null>(null)
  const [showCopyModal, setShowCopyModal] = useState(false)
  const [copyModalText, setCopyModalText] = useState('')
  const [copyModalTitle, setCopyModalTitle] = useState('')
  const [showFilters, setShowFilters] = useState(false)
  const [activeFilters, setActiveFilters] = useState<FilterCondition[]>([])
  const [filterHistory, setFilterHistory] = useState<FilterCondition[][]>([])
  const [pendingEdit, setPendingEdit] = useState<{
    column: string
    oldValue: any
    newValue: string
    pkValues: Record<string, any>
    source: 'inline' | 'modal' | 'quicklook'
  } | null>(null)
  const [confirmLoading, setConfirmLoading] = useState(false)
  const loadSeq = useRef(0)
  const loadAbort = useRef<AbortController | null>(null)

  const pkCols = useMemo(
    () => structure?.columns?.filter((c) => c.key === 'PRI').map((c) => c.name) ?? [],
    [structure],
  )
  const insertableCols = useMemo<ColumnInfo[]>(
    () =>
      structure?.columns?.filter((c) => !c.extra.toLowerCase().includes('auto_increment')) ??
      (data?.columns.map((name) => ({ name, type: '', key: '', extra: '', nullable: true, default: '' })) ?? []),
    [structure, data],
  )

  // 一列草稿的預設值（必填 datetime 給 now）。
  function draftSeed(): Record<string, string> {
    const seed: Record<string, string> = {}
    for (const c of insertableCols) {
      if (isDateTimeType(c.type) && isRequiredOnInsert(c)) {
        seed[c.name] = nowMysqlDateTime()
      }
    }
    return seed
  }

  // 新增一列草稿；可帶初值（複製整列時把來源列的值帶進來）。
  function addDraftRow(values?: Record<string, string>) {
    setDraftRows((rows) => [...rows, { ...draftSeed(), ...(values ?? {}) }])
  }

  function updateDraft(i: number, col: string, value: string) {
    setDraftRows((rows) => rows.map((r, idx) => (idx === i ? { ...r, [col]: value } : r)))
  }

  function discardDraft(i: number) {
    setDraftRows((rows) => rows.filter((_, idx) => idx !== i))
  }

  // 取某資料列的欄位值（複製整列用），全部轉字串。
  function rowValuesOf(rowIdx: number): Record<string, string> {
    const out: Record<string, string> = {}
    if (!data) return out
    data.columns.forEach((c, i) => {
      const v = data.rows[rowIdx][i]
      out[c] = v == null ? '' : String(v)
    })
    return out
  }

  function dataPath(path: string) {
    return `/api/db/${connId}/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}${path}`
  }

  function handleCellClick(rowIdx: number, colIdx: number, e: React.MouseEvent) {
    setSelectedCell({ row: rowIdx, col: colIdx })
    const isCmdOrCtrl = e.metaKey || e.ctrlKey
    const isShift = e.shiftKey
    if (isShift && lastClickedRow != null) {
      const start = Math.min(lastClickedRow, rowIdx)
      const end = Math.max(lastClickedRow, rowIdx)
      const next = new Set<number>()
      for (let i = start; i <= end; i++) next.add(i)
      setSelectedRows(next)
    } else if (isCmdOrCtrl) {
      const next = new Set(selectedRows)
      if (next.has(rowIdx)) next.delete(rowIdx)
      else next.add(rowIdx)
      setSelectedRows(next)
      setLastClickedRow(rowIdx)
    } else {
      setSelectedRows(new Set([rowIdx]))
      setLastClickedRow(rowIdx)
    }
  }

  function clearRowSelection() {
    setSelectedRows(new Set())
    setLastClickedRow(null)
  }

  function handleRowMouseDown(rowIdx: number, e: React.MouseEvent) {
    if (e.button !== 0) return
    if (e.metaKey || e.ctrlKey || e.shiftKey) return
    const target = e.target as HTMLElement
    if (target.closest('input, textarea, button, select, a')) return
    dragAnchor.current = rowIdx
    isDragging.current = true
  }

  function handleRowMouseEnter(rowIdx: number) {
    if (!isDragging.current || dragAnchor.current == null) return
    const anchor = dragAnchor.current
    const start = Math.min(anchor, rowIdx)
    const end = Math.max(anchor, rowIdx)
    const next = new Set<number>()
    for (let i = start; i <= end; i++) next.add(i)
    setSelectedRows(next)
    setLastClickedRow(rowIdx)
  }

  useEffect(() => {
    function handleMouseUp() {
      isDragging.current = false
      dragAnchor.current = null
    }
    function handleSelectStart(e: Event) {
      if (isDragging.current) e.preventDefault()
    }
    window.addEventListener('mouseup', handleMouseUp)
    window.addEventListener('selectstart', handleSelectStart)
    return () => {
      window.removeEventListener('mouseup', handleMouseUp)
      window.removeEventListener('selectstart', handleSelectStart)
    }
  }, [])

  // 鍵盤快捷鍵：跟右鍵選單標示的一致，全部真的接上動作。
  //   Ctrl/Cmd+A 全選、Ctrl/Cmd+C 複製、Ctrl/Cmd+I 新增列、
  //   Ctrl/Cmd+D 複製整列、Ctrl/Cmd+V 貼上、Ctrl/Cmd+Enter 快速檢視、
  //   Ctrl/Cmd+Alt+R 重新整理、Ctrl/Cmd+S 儲存草稿列、Delete 刪除選取/目前列。
  // 目標格用 selectedCell，選取列用 selectedRows。跨平台（mac 用 metaKey，
  // win/linux 用 ctrlKey）。在輸入框或 SQL 編輯器內時跳過，保留原生行為。
  // Ctrl+D 有 preventDefault，蓋掉 Chrome 的「加入書籤」避免衝突。
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      // Ctrl/Cmd+S：儲存所有草稿列。要在輸入框守衛之前處理，這樣使用者正在
      // 編輯草稿格時也能直接存。同時 preventDefault 蓋掉瀏覽器的「儲存網頁」。
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === 's') {
        if (draftRows.length > 0) {
          e.preventDefault()
          void saveDrafts()
        }
        return
      }

      const el = document.activeElement as HTMLElement | null
      const tag = el?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el?.isContentEditable) return
      if (el?.closest?.('.cm-editor')) return
      if (!data) return

      const mod = e.metaKey || e.ctrlKey

      // Delete：刪除選取的列，否則刪除目前格所在列（兩條路徑都有確認視窗）。
      if ((e.key === 'Delete' || e.key === 'Backspace') && !mod && !e.altKey) {
        if (selectedRows.size > 0) {
          e.preventDefault()
          void deleteSelectedRows()
        } else if (selectedCell) {
          e.preventDefault()
          void deleteRow(selectedCell.row)
        }
        return
      }

      if (!mod) return
      const key = e.key.toLowerCase()

      // Ctrl+Alt+R：重新整理。
      if (e.altKey) {
        if (key === 'r') {
          e.preventDefault()
          reload()
        }
        return
      }

      switch (key) {
        case 'a':
          e.preventDefault()
          setSelectedRows(new Set(data.rows.map((_, i) => i)))
          setSelectedCell(null)
          break
        case 'c':
          if (selectedRows.size > 0) {
            e.preventDefault()
            void copySelectedRows()
          } else if (selectedCell) {
            e.preventDefault()
            const v = data.rows[selectedCell.row]?.[selectedCell.col]
            void tryCopyToClipboard(v == null ? '' : String(v))
          }
          break
        case 'i':
          e.preventDefault()
          addDraftRow()
          break
        case 'd':
          if (selectedCell) {
            e.preventDefault()
            void duplicateRowAt(selectedCell.row)
          }
          break
        case 'v':
          if (selectedCell) {
            e.preventDefault()
            void pasteIntoCell(selectedCell.row, selectedCell.col)
          }
          break
        case 'enter':
          if (selectedCell) {
            e.preventDefault()
            openQuickLook(selectedCell.row, selectedCell.col)
          }
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, selectedRows, selectedCell, draftRows])

  function cancelLoad() {
    loadAbort.current?.abort()
  }

  function reload() {
    if (connId == null) return
    const seq = ++loadSeq.current
    // Abort any in-flight browse request before starting a new one.
    loadAbort.current?.abort()
    const ac = new AbortController()
    loadAbort.current = ac
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
    if (activeFilters.length > 0) {
      params.set('filters', JSON.stringify(activeFilters.map(f => ({
        column: f.column,
        operator: f.operator,
        value: f.value,
      }))))
    }
    api
      .get<RowsPage>(dataPath(`/data?${params}`), { signal: ac.signal })
      .then((d) => {
        if (seq !== loadSeq.current) return
        setData({ ...d, rows: d.rows ?? [] })
      })
      .catch((err) => {
        if (seq !== loadSeq.current) return
        // Aborted by the user (cancel) or a superseding reload: not an error.
        if (ac.signal.aborted || (err && err.name === 'AbortError')) {
          setError(null)
          return
        }
        setError(err instanceof ApiError ? err.message : 'load failed')
      })
      .finally(() => {
        if (seq !== loadSeq.current) return
        setLoading(false)
      })
  }

  useEffect(() => {
    if (connId == null) return
    setStructure(null)
    api
      .get<Structure>(dataPath('/structure'))
      .then(setStructure)
      .catch(() => setStructure({ columns: [] }))
  }, [connId, db, table])

  // Restore last-used filter for this (conn, db, table) and load history.
  useEffect(() => {
    if (connId == null) return
    const saved = getCurrentFilters(connId, db, table)
    setActiveFilters(saved ?? [])
    setFilterHistory(getHistory(connId, db, table))
  }, [connId, db, table])

  useEffect(reload, [connId, db, table, page, perPage, sortCol, sortDir, activeFilters])

  useEffect(() => { clearRowSelection() }, [connId, db, table, page, perPage, sortCol, sortDir, activeFilters])

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

  function commitEdit() {
    if (!editing || !data || connId == null) return
    const col = data.columns[editing.col]
    const pk = pkValuesOfRow(editing.row)
    if (!pk) {
      setEditing(null)
      return
    }
    const oldValue = data.rows[editing.row]?.[editing.col]
    setPendingEdit({
      column: col,
      oldValue,
      newValue: editValue,
      pkValues: pk,
      source: 'inline',
    })
    setEditing(null)
  }

  async function confirmEdit() {
    if (!pendingEdit) return
    setConfirmLoading(true)
    try {
      await api.patch<{ affected: number }>(dataPath('/rows'), {
        pk_values: pendingEdit.pkValues,
        column: pendingEdit.column,
        new_value: pendingEdit.newValue,
      })
      reload()
      setPendingEdit(null)
      // Close edit modals if open
      setShowEditModal(false)
      setShowQuickLookModal(false)
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : t('edit.update_failed'))
    } finally {
      setConfirmLoading(false)
    }
  }

  // 把所有草稿列 INSERT 進 DB（Ctrl+S）。只送可插入且非空的欄位；空白草稿直接
  // 丟掉不算錯；失敗的草稿留著讓使用者修再存。
  async function saveDrafts() {
    if (connId == null || draftRows.length === 0) return
    const insertableSet = new Set(insertableCols.map((c) => c.name))
    const errors: string[] = []
    const remaining: Record<string, string>[] = []
    let saved = 0
    for (const draft of draftRows) {
      const values: Record<string, string> = {}
      for (const [k, v] of Object.entries(draft)) {
        if (insertableSet.has(k) && v !== undefined && v !== '') values[k] = v
      }
      if (Object.keys(values).length === 0) continue
      try {
        await api.post<{ id: number }>(dataPath('/rows'), { values })
        saved++
      } catch (err) {
        errors.push(err instanceof ApiError ? err.message : String(err))
        remaining.push(draft)
      }
    }
    setDraftRows(remaining)
    if (errors.length > 0) {
      window.alert(t('edit.insert_failed') + '\n' + errors.join('\n'))
    } else if (saved > 0) {
      toast(`已新增 ${saved} 列`)
    }
    reload()
  }

  async function deleteRow(rowIdx: number) {
    if (connId == null) return
    const pk = pkValuesOfRow(rowIdx)
    if (!pk) return
    if (!window.confirm(t('edit.delete_confirm'))) return
    try {
      await api.deleteWithBody<{ affected: number }>(dataPath('/rows'), { pk_values: pk })
      reload()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : t('edit.delete_failed'))
    }
  }

  async function deleteSelectedRows() {
    if (connId == null || selectedRows.size === 0) return
    if (!window.confirm(t('edit.delete_selected_confirm', { count: selectedRows.size }))) return
    const indices = Array.from(selectedRows).sort((a, b) => b - a)
    const errors: string[] = []
    for (const rowIdx of indices) {
      const pk = pkValuesOfRow(rowIdx)
      if (!pk) continue
      try {
        await api.deleteWithBody<{ affected: number }>(dataPath('/rows'), { pk_values: pk })
      } catch (err) {
        errors.push(err instanceof ApiError ? err.message : String(err))
      }
    }
    clearRowSelection()
    if (errors.length > 0) {
      window.alert(t('edit.delete_selected_partial', { count: errors.length }) + '\n' + errors.join('\n'))
    }
    reload()
  }

  async function copySelectedRows() {
    if (!data || selectedRows.size === 0) return
    const indices = Array.from(selectedRows).sort((a, b) => a - b)
    const lines = indices.map((rowIdx) => {
      const rowData = data.rows[rowIdx]
      return rowData.map((v) => (v === null ? '' : String(v))).join('\t')
    })
    const tsv = lines.join('\n')
    const copied = await tryCopyToClipboard(tsv)
    if (!copied) {
      setCopyModalText(tsv)
      setCopyModalTitle(`Copy ${selectedRows.size} Rows`)
      setShowCopyModal(true)
    }
  }

  async function copySelectedRowsAs(format: string) {
    if (!data || selectedRows.size === 0) return
    try {
      const copyFormats = await import('../lib/copyFormats')
      const indices = Array.from(selectedRows).sort((a, b) => a - b)
      const rowsData = indices.map((i) => data.rows[i])
      let copyText = ''
      if (format === 'JSON') {
        const arr = rowsData.map((r) =>
          Object.fromEntries(data.columns.map((c, i) => [c, r[i]])),
        )
        copyText = JSON.stringify(arr, null, 2)
      } else if (format === 'TSV for Excel') {
        copyText = rowsData
          .map((r) => r.map((v) => (v === null ? '' : copyFormats.copyAsTsv(v))).join('\t'))
          .join('\n')
      } else if (format === 'Markdown') {
        const header = '| ' + data.columns.join(' | ') + ' |'
        const sep = '| ' + data.columns.map(() => '---').join(' | ') + ' |'
        const body = rowsData
          .map((r) => '| ' + r.map((v) => (v === null ? 'NULL' : String(v).replace(/\|/g, '\\|'))).join(' | ') + ' |')
          .join('\n')
        copyText = [header, sep, body].join('\n')
      } else if (format === 'Insert statement') {
        copyText = rowsData
          .map((r) => copyFormats.copyAsInsertStatement(r, data.columns, table))
          .join('\n')
      }
      if (copyText) {
        const copied = await tryCopyToClipboard(copyText)
        if (!copied) {
          setCopyModalText(copyText)
          setCopyModalTitle(`Copy ${selectedRows.size} Rows as ${format}`)
          setShowCopyModal(true)
        }
      }
    } catch {
      window.alert(t('edit.failed_to_copy'))
    }
  }


  async function handleMenuAction(action: string, subaction?: string) {
    if (!data) return
    try {
      if (action === 'copy-selected') {
        await copySelectedRows()
        return
      }
      if (action === 'copy-selected-as') {
        await copySelectedRowsAs(subaction ?? '')
        return
      }
      if (action === 'delete-selected') {
        await deleteSelectedRows()
        return
      }
      if (action === 'refresh') {
        reload()
        return
      }
      if (!cellInfo) return
      const colName = data.columns[cellInfo.colIdx]
      const rowIdx = cellInfo.rowIdx
      const pk = pkValuesOfRow(rowIdx)
      if (!pk) return

      switch (action) {
        case 'edit':
          setEditingValue(cellValue)
          setEditingCellInfo(cellInfo)
          setShowEditModal(true)
          break

        case 'quick-look':
          setEditingValue(cellValue)
          setEditingCellInfo(cellInfo)
          setShowQuickLookModal(true)
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
          {
            // Copy entire row as tab-separated
            const rowData = data.rows[rowIdx]
            const tsvRow = rowData.map((v) => (v === null ? '' : String(v))).join('\t')
            const copied = await tryCopyToClipboard(tsvRow)
            if (!copied) {
              // Fallback: show copy modal
              setCopyModalText(tsvRow)
              setCopyModalTitle('Copy Row')
              setShowCopyModal(true)
            }
          }
          break

        case 'copy-cell':
          {
            const cellValueStr = cellValue === null ? 'NULL' : String(cellValue)
            const copied = await tryCopyToClipboard(cellValueStr)
            if (!copied) {
              // Fallback: show copy modal
              setCopyModalText(cellValueStr)
              setCopyModalTitle('Copy Cell')
              setShowCopyModal(true)
            }
          }
          break

        case 'copy-column':
          {
            try {
              const { copyColumnAsTabSeparated } = await import('../lib/copyFormats')
              const colIdx = cellInfo.colIdx
              const colCopy = copyColumnAsTabSeparated(colName, data.rows, colIdx)
              const copied = await tryCopyToClipboard(colCopy)
              if (!copied) {
                // Fallback: show copy modal
                setCopyModalText(colCopy)
                setCopyModalTitle(`Copy Column: ${colName}`)
                setShowCopyModal(true)
              }
            } catch {
              window.alert(t('edit.failed_to_copy'))
            }
          }
          break

        case 'copy-as':
          {
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
                const copied = await tryCopyToClipboard(copyText)
                if (!copied) {
                  // Fallback: show copy modal
                  setCopyModalText(copyText)
                  setCopyModalTitle(`Copy as ${subaction}`)
                  setShowCopyModal(true)
                }
              }
            } catch {
              window.alert(t('edit.failed_to_copy'))
            }
          }
          break

        case 'delete-row':
          if (window.confirm(t('edit.delete_confirm'))) {
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
          {
            try {
              if (!navigator.clipboard) {
                throw new Error('Clipboard API not available')
              }
              const pastedText = await navigator.clipboard.readText()
              await api.patch(dataPath('/rows'), { pk_values: pk, column: colName, new_value: pastedText })
              reload()
              toast(tr('common.pasted'))
            } catch (err) {
              throw err // Let outer catch handle it
            }
          }
          break

        case 'add-row':
          addDraftRow()
          break

        case 'duplicate':
          addDraftRow(rowValuesOf(rowIdx))
          break
      }
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'Operation failed')
    } finally {
      closeMenu()
    }
  }

  // 以下三個 helper 讓鍵盤快捷鍵與右鍵選單共用同一套動作，目標為指定的
  // (rowIdx, colIdx)，鍵盤路徑用 selectedCell/selectedRows 當目標。

  // 複製整列（Ctrl+D）：把來源列的值帶進一列新草稿（綠底、未存），讓使用者
  // 改完（例如 PK）再按 Ctrl+S 存。與新增列共用同一套草稿流程。
  function duplicateRowAt(rowIdx: number) {
    addDraftRow(rowValuesOf(rowIdx))
  }

  // 把剪貼簿內容貼進指定格（Ctrl+V）
  async function pasteIntoCell(rowIdx: number, colIdx: number) {
    if (!data) return
    const pk = pkValuesOfRow(rowIdx)
    if (!pk) return
    try {
      if (!navigator.clipboard) throw new Error('Clipboard API not available')
      const pastedText = await navigator.clipboard.readText()
      await api.patch(dataPath('/rows'), { pk_values: pk, column: data.columns[colIdx], new_value: pastedText })
      reload()
      toast(tr('common.pasted'))
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'Operation failed')
    }
  }

  // 開啟快速檢視 modal（Ctrl+Enter）
  function openQuickLook(rowIdx: number, colIdx: number) {
    if (!data) return
    const v = data.rows[rowIdx]?.[colIdx]
    setEditingValue(v)
    setEditingCellInfo({ rowIdx, colIdx })
    setShowQuickLookModal(true)
  }

  const columns = useMemo<ColumnDef<any[]>[]>(() => {
    if (!data) return []
    const out: ColumnDef<any[]>[] = data.columns.map((name, idx) => ({
      id: name,
      header: () => (
        <span
          onClick={() => {
            if (sortCol === name) {
              if (sortDir === 'asc') {
                setSortDir('desc')
              } else {
                setSortCol(null)
                setSortDir('asc')
              }
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
        const startEdit = pkCols.length === 0
          ? () => setNoPkHelp(true)
          : () => {
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
      out.push({
        id: '__actions',
        header: '',
        size: 72,
        minSize: 48,
        enableResizing: false,
        cell: (info) => (
          <button style={smallButton} onClick={() => void deleteRow(info.row.index)}>{t('common.delete')}</button>
        ),
      })
    }
    return out
  }, [data, sortCol, sortDir, editing, editValue, pkCols.length, handleContextMenu])

  const tableInst = useReactTable({
    data: data?.rows ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
    columnResizeMode: 'onChange',
    defaultColumn: { size: 180, minSize: 56, maxSize: 1200 },
  })

  // data.total < 0 means the row count is unknown (COUNT(*) was too slow on a
  // large table). Fall back to "there's a next page if this one is full".
  const totalPages = data
    ? data.total < 0
      ? (data.rows.length >= data.per_page ? page + 1 : page)
      : Math.max(1, Math.ceil(data.total / data.per_page))
    : 1

  // 草稿列渲染用：欄位順序/寬度取自 table 的 leaf columns，才能跟資料列對齊。
  const leafCols = tableInst.getAllLeafColumns()
  const insertableSet = new Set(insertableCols.map((c) => c.name))
  const hasActionsCol = leafCols.some((c) => c.id === '__actions')
  const firstDataColId = data?.columns[0]
  const draftCellBg = 'rgba(46,160,67,0.16)' // 綠底：表示未存的草稿

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui', background: 'var(--bg-primary)', color: 'var(--text-primary)' }}>
      <div style={toolbar}>
        <button onClick={() => addDraftRow()}>{t('datagrid.add_row')}</button>
        {draftRows.length > 0 && (
          <>
            <button
              onClick={() => void saveDrafts()}
              style={{ background: 'var(--accent)', color: 'white', borderColor: 'var(--accent)' }}
              title="Ctrl+S"
            >
              儲存 {draftRows.length} 列 (Ctrl+S)
            </button>
            <button onClick={() => setDraftRows([])}>捨棄</button>
          </>
        )}
        <button onClick={onWantImportExport}>{t('datagrid.import_export')}</button>
        <button
          onClick={() => setShowFilters((v) => !v)}
          style={activeFilters.length > 0 ? { background: 'var(--accent)', color: 'white', borderColor: 'var(--accent)' } : undefined}
        >
          🔍 {t('datagrid.filter_button')}{activeFilters.length > 0 ? ` (${activeFilters.length})` : ''}
        </button>
        {data && pkCols.length === 0 && (
          <button
            type="button"
            onClick={() => setNoPkHelp(true)}
            title={t('datagrid.no_pk_help_hint')}
            style={{
              fontSize: 12, padding: '2px 8px', cursor: 'pointer',
              color: 'var(--danger)', borderColor: 'var(--danger)', background: 'transparent',
            }}
          >
            ⚠ {t('datagrid.read_only_no_pk')}
          </button>
        )}
        {loading && data && <span style={muted}>{t('datagrid.refreshing')}</span>}
        {loading && (
          <button
            type="button"
            onClick={cancelLoad}
            style={{ fontSize: 12, padding: '2px 8px', cursor: 'pointer' }}
          >
            ✕ {t('datagrid.cancel_load')}
          </button>
        )}
      </div>

      {showFilters && data && (
        <FilterBar
          columns={data.columns}
          initialFilters={activeFilters.length > 0 ? activeFilters : undefined}
          history={filterHistory}
          onApply={(fs) => {
            setActiveFilters(fs)
            setPage(1)
            if (connId != null) {
              setCurrentFilters(connId, db, table, fs)
              pushHistory(connId, db, table, fs)
              setFilterHistory(getHistory(connId, db, table))
            }
          }}
          onClose={() => setShowFilters(false)}
        />
      )}

      <div style={{ flex: 1, overflow: 'auto' }}>
        {error && <div style={{ color: 'crimson', padding: 8 }}>{error}</div>}
        {loading && !data && <div style={{ color: '#999', padding: 8 }}>loading…</div>}
        {data && (
          <table style={{ borderCollapse: 'collapse', fontSize: 13, tableLayout: 'fixed', width: tableInst.getCenterTotalSize() }}>
            <thead style={{ background: 'var(--table-header-bg)', position: 'sticky', top: 0 }}>
              {tableInst.getHeaderGroups().map((hg) => (
                <tr key={hg.id}>
                  {hg.headers.map((h) => (
                    <th key={h.id} style={{ ...th, width: h.getSize(), position: 'relative' }}>
                      {flexRender(h.column.columnDef.header, h.getContext())}
                      {h.column.getCanResize() && (
                        <div
                          data-col-resizer
                          onMouseDown={h.getResizeHandler()}
                          onTouchStart={h.getResizeHandler()}
                          onClick={(e) => e.stopPropagation()}
                          style={{
                            position: 'absolute', top: 0, right: 0, width: 7, height: '100%',
                            cursor: 'col-resize', userSelect: 'none', touchAction: 'none',
                            background: h.column.getIsResizing() ? 'var(--accent)' : 'transparent',
                          }}
                          title="drag to resize column"
                        />
                      )}
                    </th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {draftRows.map((draft, di) => (
                <tr key={`draft-${di}`} style={{ background: draftCellBg }}>
                  {leafCols.map((col) => {
                    const w = col.getSize()
                    if (col.id === '__actions') {
                      return (
                        <td key={col.id} style={{ ...td, width: w, background: draftCellBg }}>
                          <button style={smallButton} onClick={() => discardDraft(di)} title="捨棄這列">✕</button>
                        </td>
                      )
                    }
                    const editable = insertableSet.has(col.id)
                    return (
                      <td key={col.id} style={{ ...td, width: w, background: draftCellBg }}>
                        <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                          <input
                            value={draft[col.id] ?? ''}
                            readOnly={!editable}
                            onChange={(e) => updateDraft(di, col.id, e.target.value)}
                            placeholder={editable ? '' : '(自動)'}
                            style={{
                              ...editInput,
                              background: editable ? 'var(--bg-primary)' : 'transparent',
                              color: editable ? undefined : 'var(--text-secondary)',
                            }}
                          />
                          {!hasActionsCol && col.id === firstDataColId && (
                            <button style={smallButton} onClick={() => discardDraft(di)} title="捨棄這列">✕</button>
                          )}
                        </span>
                      </td>
                    )
                  })}
                </tr>
              ))}
              {tableInst.getRowModel().rows.map((row) => {
                const rowSelected = selectedRows.has(row.index) ||
                  (selectedRows.size === 0 && selectedCell?.row === row.index)
                return (
                  <tr
                    key={row.id}
                    style={rowSelected ? trSelected : undefined}
                    onMouseDown={(e) => handleRowMouseDown(row.index, e)}
                    onMouseEnter={() => handleRowMouseEnter(row.index)}
                  >
                    {row.getVisibleCells().map((cell) => {
                      const colIdx = data.columns.indexOf(cell.column.id)
                      const rowIdx = row.index
                      const v = data.rows[rowIdx]?.[colIdx]
                      const isActionCell = cell.column.id === '__actions'
                      const cellSelected = selectedCell?.row === rowIdx && selectedCell?.col === colIdx
                      let longPressTimer: ReturnType<typeof setTimeout> | null = null
                      const cancelLongPress = () => {
                        if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null }
                      }
                      return (
                        <td
                          key={cell.id}
                          style={cellSelected ? { ...td, ...tdSelected } : td}
                          title={isActionCell || v === null || v === undefined ? undefined : String(v)}
                          onClick={isActionCell ? undefined : (e) => handleCellClick(rowIdx, colIdx, e)}
                          onContextMenu={isActionCell ? undefined : (e) => {
                            setSelectedCell({ row: rowIdx, col: colIdx })
                            if (!selectedRows.has(rowIdx)) {
                              setSelectedRows(new Set([rowIdx]))
                              setLastClickedRow(rowIdx)
                            }
                            handleContextMenu(e, rowIdx, colIdx, v)
                          }}
                          onTouchStart={isActionCell ? undefined : (e) => {
                            const touch = e.touches[0]
                            const startX = touch.clientX, startY = touch.clientY
                            longPressTimer = setTimeout(() => {
                              setSelectedCell({ row: rowIdx, col: colIdx })
                              handleContextMenu(
                                { preventDefault: () => {}, clientX: startX, clientY: startY } as any,
                                rowIdx, colIdx, v,
                              )
                              longPressTimer = null
                            }, 500)
                          }}
                          onTouchMove={cancelLongPress}
                          onTouchEnd={cancelLongPress}
                          onTouchCancel={cancelLongPress}
                        >
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </td>
                      )
                    })}
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
      <div style={pager}>
        <button disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>{t('pager.prev')}</button>
        {buildPageNumbers(page, totalPages).map((n, i) =>
          n === '...' ? (
            <span key={`dot-${i}`} style={{ padding: '0 4px', color: '#999' }}>...</span>
          ) : (
            <button
              key={n}
              disabled={n === page}
              onClick={() => setPage(n as number)}
              style={{
                padding: '4px 8px',
                fontWeight: n === page ? 700 : 400,
                background: n === page ? '#cfe2ff' : 'transparent',
                border: '1px solid #ccc',
                borderRadius: 3,
                cursor: n === page ? 'default' : 'pointer',
                minWidth: 28,
              }}
            >
              {n}
            </button>
          ),
        )}
        <button disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>{t('pager.next')}</button>
        <label style={limitLabel}>
          {t('pager.limit')}
          <select
            aria-label="table row limit"
            value={perPage}
            onChange={(e) => {
              setPage(1)
              setPerPage(Number(e.target.value))
            }}
            style={limitSelect}
          >
            {ROW_LIMIT_OPTIONS.map((n) => (
              <option key={n} value={n}>{n}</option>
            ))}
          </select>
        </label>
        <span style={{ marginLeft: 4, color: '#666', fontSize: 12 }}>
          {t('pager.per_page', { n: perPage })} · {t('pager.rows_total', { count: data && data.total >= 0 ? data.total : '?' })}
        </span>
      </div>

      {position && cellInfo && data && (
        <CellContextMenu
          position={position}
          cellValue={cellValue}
          columnName={data.columns[cellInfo.colIdx] || ''}
          selectedCount={selectedRows.size}
          onAction={handleMenuAction}
          onClose={closeMenu}
        />
      )}

      {showEditModal && editingCellInfo && data && (
        <EditCellModal
          value={editingValue}
          columnName={data.columns[editingCellInfo.colIdx] || ''}
          columnType={structure?.columns?.find(c => c.name === data.columns[editingCellInfo.colIdx])?.type}
          onApply={async (newValue) => {
            if (!editingCellInfo || !data || connId == null) return
            const colName = data.columns[editingCellInfo.colIdx]
            const pk = pkValuesOfRow(editingCellInfo.rowIdx)
            if (!pk) return
            setPendingEdit({
              column: colName,
              oldValue: editingValue,
              newValue,
              pkValues: pk,
              source: 'modal',
            })
          }}
          onCancel={() => setShowEditModal(false)}
        />
      )}

      {showQuickLookModal && editingCellInfo && data && (
        <QuickLookEditorModal
          value={editingValue}
          columnName={data.columns[editingCellInfo.colIdx] || ''}
          onApply={async (newValue) => {
            if (!editingCellInfo || !data || connId == null) return
            const colName = data.columns[editingCellInfo.colIdx]
            const pk = pkValuesOfRow(editingCellInfo.rowIdx)
            if (!pk) return
            setPendingEdit({
              column: colName,
              oldValue: editingValue,
              newValue,
              pkValues: pk,
              source: 'quicklook',
            })
          }}
          onCancel={() => setShowQuickLookModal(false)}
        />
      )}

      {pendingEdit && (
        <ConfirmEditModal
          column={pendingEdit.column}
          oldValue={pendingEdit.oldValue}
          newValue={pendingEdit.newValue}
          pkValues={pendingEdit.pkValues}
          loading={confirmLoading}
          onConfirm={() => void confirmEdit()}
          onCancel={() => setPendingEdit(null)}
        />
      )}

      {showCopyModal && (
        <CopyTextModal
          text={copyModalText}
          title={copyModalTitle}
          onCancel={() => setShowCopyModal(false)}
        />
      )}

      {noPkHelp && (() => {
        const alterSnippet =
          `-- 加主鍵後即可就地編輯/刪除\n` +
          `-- 1) 既有唯一欄位設為主鍵：\n` +
          `ALTER TABLE \`${table}\` ADD PRIMARY KEY (\`your_unique_column\`);\n` +
          `-- 2) 或新增自增主鍵 (MySQL)：\n` +
          `ALTER TABLE \`${table}\` ADD COLUMN id INT AUTO_INCREMENT PRIMARY KEY;`
        return (
          <ConfirmModal
            title={t('datagrid.no_pk_title')}
            body={t('datagrid.no_pk_body')}
            detail={alterSnippet}
            confirmLabel={t('datagrid.no_pk_copy_sql')}
            cancelLabel={t('common.close')}
            onConfirm={() => {
              void navigator.clipboard?.writeText(alterSnippet).catch(() => {})
              setNoPkHelp(false)
            }}
            onCancel={() => setNoPkHelp(false)}
          />
        )
      })()}
    </div>
  )
}

const toolbar: CSSProperties = {
  display: 'flex', gap: 8, alignItems: 'center', padding: 6,
  borderBottom: '1px solid var(--border-color)', fontSize: 12,
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
const editInput: CSSProperties = { width: '100%', minWidth: 80, boxSizing: 'border-box', fontSize: 13, padding: '2px 4px' }
const smallButton: CSSProperties = { fontSize: 11, padding: '2px 6px' }
const muted: CSSProperties = { color: 'var(--text-muted)' }
const pager: CSSProperties = {
  display: 'flex', gap: 8, alignItems: 'center', padding: 6,
  borderTop: '1px solid var(--border-color)', fontSize: 12,
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
const limitLabel: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  marginLeft: 8,
  color: '#666',
  fontSize: 12,
  whiteSpace: 'nowrap',
}
const limitSelect: CSSProperties = {
  fontSize: 12,
  padding: '2px 4px',
}
const th: CSSProperties = {
  textAlign: 'left', padding: '4px 8px',
  borderBottom: '1px solid var(--border-color)', borderRight: '1px solid var(--border-color)',
  whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
}
const td: CSSProperties = {
  padding: '4px 8px',
  borderBottom: '1px solid var(--table-border)', borderRight: '1px solid var(--table-border)',
  whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
}
const trSelected: CSSProperties = { background: 'var(--bg-active)' }
const tdSelected: CSSProperties = { outline: '2px solid var(--accent)', outlineOffset: '-2px' }

function buildPageNumbers(current: number, total: number): (number | '...')[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1)
  }
  const pages: (number | '...')[] = []
  pages.push(1)
  if (current <= 4) {
    // 1 2 3 4 5 ... last
    for (let i = 2; i <= 5; i++) pages.push(i)
    pages.push('...')
    pages.push(total)
  } else if (current >= total - 3) {
    // 1 ... last-4 last-3 last-2 last-1 last
    pages.push('...')
    for (let i = total - 4; i <= total; i++) pages.push(i)
  } else {
    // 1 ... current-1 current current+1 ... last
    pages.push('...')
    pages.push(current - 1)
    pages.push(current)
    pages.push(current + 1)
    pages.push('...')
    pages.push(total)
  }
  return pages
}
