# DataGrid 右键菜单 + 编辑功能实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement comprehensive right-click context menu for DataGrid with 15+ menu items, edit modal supporting JSON tree editing, and fix empty string editing bug.

**Architecture:** 
- Decompose DataGrid into focused components: CellContextMenu handles menu logic, EditCellModal routes to simple/JSON editors, JsonTreeEditor renders tree UI
- Use React hooks and Zustand for state management (consistent with existing patterns)
- Copy operations use browser Clipboard API; API calls leverage existing `api` utility

**Tech Stack:** React, TypeScript, TanStack Table, Zustand, CSS-in-JS (inline styles matching DataGrid pattern)

---

## File Structure

```
web/src/
├── components/
│   ├── DataGrid.tsx (MODIFY - main integration, fix empty string bug)
│   ├── CellContextMenu.tsx (CREATE - right-click menu container)
│   ├── EditCellModal.tsx (CREATE - modal router for simple/JSON)
│   ├── SimpleCellEditor.tsx (CREATE - text input for scalars)
│   ├── JsonTreeEditor.tsx (CREATE - left tree + right editor for JSON)
│   └── useContextMenu.ts (CREATE - hook for menu positioning)
├── lib/
│   └── copyFormats.ts (CREATE - Copy As implementations)
└── (no new test files - integration tests via e2e)
```

---

## Task 1: Create useContextMenu Hook

**Files:**
- Create: `web/src/components/useContextMenu.ts`

- [ ] **Step 1: Create hook file with positioning logic**

```typescript
// web/src/components/useContextMenu.ts
import { useState, useCallback } from 'react'

interface MenuPosition {
  x: number
  y: number
}

export function useContextMenu() {
  const [position, setPosition] = useState<MenuPosition | null>(null)
  const [cellInfo, setCellInfo] = useState<{ rowIdx: number; colIdx: number } | null>(null)
  const [cellValue, setCellValue] = useState<any>(null)

  const handleContextMenu = useCallback(
    (e: React.MouseEvent, rowIdx: number, colIdx: number, value: any) => {
      e.preventDefault()
      setPosition({ x: e.clientX, y: e.clientY })
      setCellInfo({ rowIdx, colIdx })
      setCellValue(value)
    },
    [],
  )

  const closeMenu = useCallback(() => {
    setPosition(null)
    setCellInfo(null)
    setCellValue(null)
  }, [])

  return { position, cellInfo, cellValue, handleContextMenu, closeMenu }
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/useContextMenu.ts
git commit -m "feat: add useContextMenu hook for context menu positioning"
```

---

## Task 2: Create Copy Formats Utility

**Files:**
- Create: `web/src/lib/copyFormats.ts`

- [ ] **Step 1: Create utility with all copy format functions**

```typescript
// web/src/lib/copyFormats.ts
import { ApiError } from './api'

export interface CopyFormatsInput {
  cellValue: any
  columnName: string
  rowData: any[]
  columnNames: string[]
  tableName: string
  dbName: string
}

/**
 * Copy cell value as-is (string representation)
 */
export function copyAsText(value: any): string {
  if (value === null) return 'NULL'
  if (value === undefined) return ''
  return String(value)
}

/**
 * Copy entire row as tab-separated values
 */
export function copyAsTabSeparated(rowData: any[]): string {
  return rowData.map((v) => (v === null ? '' : String(v))).join('\t')
}

/**
 * Copy entire column values as tab-separated
 */
export function copyColumnAsTabSeparated(
  columnName: string,
  allRows: any[][],
  columnIdx: number,
): string {
  return allRows.map((row) => {
    const v = row[columnIdx]
    return v === null ? '' : String(v)
  }).join('\t')
}

/**
 * Copy as JSON (pretty-printed for objects, string for scalars)
 */
export function copyAsJson(value: any): string {
  if (value === null) return 'null'
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return JSON.stringify(value)
    }
  }
  return JSON.stringify(value, null, 2)
}

/**
 * Copy as TSV for Excel (single cell)
 */
export function copyAsTsv(value: any): string {
  // Single value already tab-ready, or as tab-quoted string
  if (value === null) return ''
  const str = String(value)
  // Excel TSV: quote if contains tab/newline/quote
  if (str.includes('\t') || str.includes('\n') || str.includes('"')) {
    return '"' + str.replace(/"/g, '""') + '"'
  }
  return str
}

/**
 * Copy as Markdown code block
 */
export function copyAsMarkdown(value: any): string {
  const content = value === null ? 'null' : String(value)
  return '```\n' + content + '\n```'
}

/**
 * Copy as INSERT statement (single row)
 */
export function copyAsInsertStatement(
  rowData: any[],
  columnNames: string[],
  tableName: string,
): string {
  const cols = columnNames.join(', ')
  const values = rowData
    .map((v) => {
      if (v === null) return 'NULL'
      if (typeof v === 'number') return String(v)
      if (typeof v === 'boolean') return v ? '1' : '0'
      // String: escape single quotes
      return "'" + String(v).replace(/'/g, "''") + "'"
    })
    .join(', ')
  return `INSERT INTO ${tableName} (${cols}) VALUES (${values})`
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/copyFormats.ts
git commit -m "feat: add copyFormats utility with all format converters"
```

---

## Task 3: Create SimpleCellEditor Component

**Files:**
- Create: `web/src/components/SimpleCellEditor.tsx`

- [ ] **Step 1: Create simple text editor component**

```typescript
// web/src/components/SimpleCellEditor.tsx
import { useState, useEffect } from 'react'

interface SimpleCellEditorProps {
  initialValue: any
  onApply: (newValue: string) => void
  onCancel: () => void
}

export function SimpleCellEditor({ initialValue, onApply, onCancel }: SimpleCellEditorProps) {
  const [value, setValue] = useState(initialValue == null ? '' : String(initialValue))

  useEffect(() => {
    // Auto-focus input
    const input = document.querySelector('[data-simple-editor-input]') as HTMLTextAreaElement
    if (input) {
      input.focus()
      input.setSelectionRange(value.length, value.length)
    }
  }, [])

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      window.alert('Failed to copy')
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <textarea
        data-simple-editor-input
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
            onApply(value)
          }
          if (e.key === 'Escape') onCancel()
        }}
        style={{
          flex: 1,
          minHeight: 200,
          padding: 8,
          fontFamily: 'monospace',
          fontSize: 12,
          border: '1px solid #ccc',
          borderRadius: 4,
          resize: 'vertical',
        }}
      />
      <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
        <button onClick={onCancel} style={{ padding: '6px 12px' }}>
          Cancel
        </button>
        <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
          Copy
        </button>
        <button
          onClick={() => onApply(value)}
          style={{ padding: '6px 12px', backgroundColor: '#0066cc', color: 'white', border: 'none', borderRadius: 4 }}
        >
          Apply
        </button>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/SimpleCellEditor.tsx
git commit -m "feat: add SimpleCellEditor component for scalar value editing"
```

---

## Task 4: Create JsonTreeEditor Component

**Files:**
- Create: `web/src/components/JsonTreeEditor.tsx`

- [ ] **Step 1: Create JSON tree editor with left tree + right editor**

```typescript
// web/src/components/JsonTreeEditor.tsx
import { useState, useEffect } from 'react'

interface JsonNodeInfo {
  path: string[] // e.g., ['payload', 'amount']
  type: 'string' | 'number' | 'boolean' | 'null' | 'object' | 'array'
  value: any
  isExpanded: boolean
}

interface JsonTreeEditorProps {
  initialValue: any
  onApply: (newValue: string) => void
  onCancel: () => void
}

export function JsonTreeEditor({ initialValue, onApply, onCancel }: JsonTreeEditorProps) {
  const [jsonValue, setJsonValue] = useState(() => {
    if (typeof initialValue === 'string') {
      try {
        return JSON.parse(initialValue)
      } catch {
        return initialValue
      }
    }
    return initialValue
  })
  const [selectedPath, setSelectedPath] = useState<string[]>([])
  const [isRawMode, setIsRawMode] = useState(false)
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set())

  const getNodeAtPath = (obj: any, path: string[]): any => {
    return path.reduce((cur, key) => cur?.[key], obj)
  }

  const getNodeType = (value: any): JsonNodeInfo['type'] => {
    if (value === null) return 'null'
    if (Array.isArray(value)) return 'array'
    if (typeof value === 'object') return 'object'
    if (typeof value === 'string') return 'string'
    if (typeof value === 'number') return 'number'
    if (typeof value === 'boolean') return 'boolean'
    return 'null'
  }

  const selectedValue = getNodeAtPath(jsonValue, selectedPath)
  const selectedType = getNodeType(selectedValue)
  const pathKey = selectedPath.join('/')

  const toggleExpand = (path: string[]) => {
    const key = path.join('/')
    setExpandedPaths((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const handleRawChange = (rawText: string) => {
    try {
      const parsed = JSON.parse(rawText)
      setJsonValue(parsed)
      setIsRawMode(false)
    } catch (e) {
      window.alert('Invalid JSON: ' + (e instanceof Error ? e.message : 'parse error'))
    }
  }

  const handleScalarEdit = (newValue: any) => {
    if (selectedPath.length === 0) {
      setJsonValue(newValue)
    } else {
      const newJson = JSON.parse(JSON.stringify(jsonValue)) // Deep clone
      let obj = newJson
      for (let i = 0; i < selectedPath.length - 1; i++) {
        obj = obj[selectedPath[i]]
      }
      obj[selectedPath[selectedPath.length - 1]] = newValue
      setJsonValue(newJson)
    }
  }

  const renderTree = (obj: any, path: string[] = []): JSX.Element => {
    const type = getNodeType(obj)
    const pathKey = path.join('/')
    const isExpanded = expandedPaths.has(pathKey)
    const isSelected = pathKey === selectedPath.join('/')

    if (type === 'object' || type === 'array') {
      const entries = type === 'object' ? Object.entries(obj) : obj.map((v, i) => [i.toString(), v])
      return (
        <div key={pathKey}>
          <div
            onClick={() => setSelectedPath(path)}
            onDoubleClick={() => toggleExpand(path)}
            style={{
              padding: '4px 8px',
              cursor: 'pointer',
              backgroundColor: isSelected ? '#e0e0e0' : 'transparent',
              borderLeft: isSelected ? '3px solid #0066cc' : '3px solid transparent',
            }}
          >
            <span onClick={() => toggleExpand(path)} style={{ marginRight: 4, fontWeight: 'bold' }}>
              {isExpanded ? '▼' : '▶'}
            </span>
            <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
              {path[path.length - 1] || 'ROOT'} ({type})
            </span>
          </div>
          {isExpanded &&
            entries.map(([key, value]) => renderTree(value, [...path, key]))}
        </div>
      )
    } else {
      return (
        <div
          key={pathKey}
          onClick={() => setSelectedPath(path)}
          style={{
            padding: '4px 8px',
            cursor: 'pointer',
            backgroundColor: isSelected ? '#e0e0e0' : 'transparent',
            borderLeft: isSelected ? '3px solid #0066cc' : '3px solid transparent',
          }}
        >
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {path[path.length - 1]} ({type})
          </span>
        </div>
      )
    }
  }

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(jsonValue, null, 2))
    } catch {
      window.alert('Failed to copy')
    }
  }

  if (isRawMode) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <textarea
          defaultValue={JSON.stringify(jsonValue, null, 2)}
          onBlur={(e) => handleRawChange(e.currentTarget.value)}
          style={{
            flex: 1,
            minHeight: 300,
            padding: 8,
            fontFamily: 'monospace',
            fontSize: 12,
            border: '1px solid #ccc',
            borderRadius: 4,
          }}
        />
        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button onClick={onCancel} style={{ padding: '6px 12px' }}>
            Cancel
          </button>
          <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
            Copy
          </button>
          <button onClick={() => setIsRawMode(false)} style={{ padding: '6px 12px' }}>
            Format
          </button>
          <button
            onClick={() => onApply(JSON.stringify(jsonValue))}
            style={{ padding: '6px 12px', backgroundColor: '#0066cc', color: 'white', border: 'none', borderRadius: 4 }}
          >
            Apply
          </button>
        </div>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', gap: 8, height: 300, border: '1px solid #ccc', borderRadius: 4, overflow: 'hidden' }}>
        {/* Left tree */}
        <div style={{ flex: 1, overflowY: 'auto', borderRight: '1px solid #eee', padding: 8 }}>
          {renderTree(jsonValue)}
        </div>

        {/* Right editor */}
        <div style={{ flex: 1, padding: 8, overflowY: 'auto' }}>
          {selectedPath.length > 0 && (
            <>
              <div style={{ fontSize: 12, color: '#666', marginBottom: 8 }}>
                <strong>Type:</strong> {selectedType}
              </div>
              {selectedType === 'string' && (
                <textarea
                  defaultValue={String(selectedValue || '')}
                  onChange={(e) => handleScalarEdit(e.target.value)}
                  style={{
                    width: '100%',
                    minHeight: 100,
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid #ccc',
                    borderRadius: 3,
                  }}
                />
              )}
              {selectedType === 'number' && (
                <input
                  type="number"
                  defaultValue={selectedValue}
                  onChange={(e) => handleScalarEdit(e.target.value === '' ? null : Number(e.target.value))}
                  style={{
                    width: '100%',
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid #ccc',
                    borderRadius: 3,
                  }}
                />
              )}
              {selectedType === 'boolean' && (
                <select
                  defaultValue={String(selectedValue)}
                  onChange={(e) => handleScalarEdit(e.target.value === 'true')}
                  style={{
                    width: '100%',
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid #ccc',
                    borderRadius: 3,
                  }}
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              )}
              {selectedType === 'null' && <div style={{ color: '#999' }}>null (cannot edit)</div>}
              {(selectedType === 'object' || selectedType === 'array') && (
                <div style={{ color: '#999', fontSize: 12 }}>
                  [{selectedType}] - select a leaf node to edit
                </div>
              )}
            </>
          )}
        </div>
      </div>

      <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
        <button onClick={onCancel} style={{ padding: '6px 12px' }}>
          Cancel
        </button>
        <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
          Copy
        </button>
        <button onClick={() => setIsRawMode(true)} style={{ padding: '6px 12px' }}>
          Raw
        </button>
        <button
          onClick={() => onApply(JSON.stringify(jsonValue))}
          style={{ padding: '6px 12px', backgroundColor: '#0066cc', color: 'white', border: 'none', borderRadius: 4 }}
        >
          Apply
        </button>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/JsonTreeEditor.tsx
git commit -m "feat: add JsonTreeEditor with left tree and right value editor"
```

---

## Task 5: Create EditCellModal Component

**Files:**
- Create: `web/src/components/EditCellModal.tsx`

- [ ] **Step 1: Create modal that routes to simple or JSON editor**

```typescript
// web/src/components/EditCellModal.tsx
import { useState } from 'react'
import { SimpleCellEditor } from './SimpleCellEditor'
import { JsonTreeEditor } from './JsonTreeEditor'

interface EditCellModalProps {
  value: any
  columnName: string
  onApply: (newValue: string) => Promise<void>
  onCancel: () => void
}

export function EditCellModal({ value, columnName, onApply, onCancel }: EditCellModalProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isJsonType = typeof value === 'string' && value.trim().startsWith('{') && value.trim().endsWith('}')

  const handleApply = async (newValue: string) => {
    setLoading(true)
    setError(null)
    try {
      await onApply(newValue)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div
        style={{
          backgroundColor: 'white',
          borderRadius: 8,
          padding: 24,
          maxWidth: '80vw',
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
        }}
      >
        <h2 style={{ marginTop: 0, marginBottom: 16, fontSize: 16 }}>
          Edit {columnName} {isJsonType ? '(JSON)' : ''}
        </h2>

        {error && <div style={{ color: 'crimson', marginBottom: 12, fontSize: 13 }}>{error}</div>}

        <div style={{ flex: 1, overflow: 'hidden', marginBottom: 16 }}>
          {isJsonType ? (
            <JsonTreeEditor initialValue={value} onApply={handleApply} onCancel={onCancel} />
          ) : (
            <SimpleCellEditor initialValue={value} onApply={handleApply} onCancel={onCancel} />
          )}
        </div>

        {loading && <div style={{ textAlign: 'center', color: '#999' }}>Saving...</div>}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/EditCellModal.tsx
git commit -m "feat: add EditCellModal that routes to SimpleCellEditor or JsonTreeEditor"
```

---

## Task 6: Create CellContextMenu Component

**Files:**
- Create: `web/src/components/CellContextMenu.tsx`

- [ ] **Step 1: Create menu with all 15+ items and submenus**

```typescript
// web/src/components/CellContextMenu.tsx
import { useRef, useEffect, useState } from 'react'

export interface MenuAction {
  label: string
  shortcut?: string
  action: 'edit' | 'set-value' | 'copy' | 'copy-cell' | 'copy-column' | 'copy-as' | 'quick-filter' | 'quick-look' |
           'refresh' | 'paste' | 'add-row' | 'duplicate' | 'delete-row'
  submenu?: MenuAction[]
}

interface CellContextMenuProps {
  position: { x: number; y: number }
  cellValue: any
  columnName: string
  onAction: (action: string, subaction?: string) => void
  onClose: () => void
}

export function CellContextMenu({ position, cellValue, columnName, onAction, onClose }: CellContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null)
  const [submenu, setSubmenu] = useState<string | null>(null)

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose()
      }
    }
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [onClose])

  const menuActions: MenuAction[] = [
    { label: 'Quick Look Editor', shortcut: 'Ctrl ↵', action: 'quick-look' },
    { label: '', action: 'separator' as any }, // Separator
    { label: 'Edit in modal', action: 'edit' },
    {
      label: 'Set Value',
      action: 'set-value',
      submenu: [
        { label: 'EMPTY', action: 'set-value' },
        { label: 'NULL', action: 'set-value' },
        { label: 'DEFAULT', action: 'set-value' },
      ],
    },
    { label: '', action: 'separator' as any },
    { label: 'Refresh', shortcut: 'Ctrl Alt R', action: 'refresh' },
    { label: 'Paste', shortcut: 'Ctrl V', action: 'paste' },
    { label: 'Add row', shortcut: 'Ctrl I', action: 'add-row' },
    { label: 'Duplicate', shortcut: 'Ctrl D', action: 'duplicate' },
    { label: '', action: 'separator' as any },
    { label: 'Copy', shortcut: 'Ctrl C', action: 'copy' },
    { label: 'Copy Cell Value', action: 'copy-cell' },
    { label: 'Copy All Column Values', action: 'copy-column' },
    {
      label: 'Copy As',
      action: 'copy-as',
      submenu: [
        { label: 'JSON', action: 'copy-as' },
        { label: 'TSV for Excel', action: 'copy-as' },
        { label: 'Markdown', action: 'copy-as' },
        { label: 'Insert statement', action: 'copy-as' },
      ],
    },
    { label: '', action: 'separator' as any },
    {
      label: 'Quick Filter',
      action: 'quick-filter',
      submenu: [
        { label: '= (equals)', action: 'quick-filter' },
        { label: 'Contains', action: 'quick-filter' },
        { label: 'Not contains', action: 'quick-filter' },
        { label: 'Has prefix', action: 'quick-filter' },
        { label: 'Has suffix', action: 'quick-filter' },
        { label: 'IS NULL', action: 'quick-filter' },
        { label: 'IS NOT NULL', action: 'quick-filter' },
      ],
    },
    { label: 'Delete row', shortcut: 'Delete', action: 'delete-row' },
  ]

  const renderMenuItem = (item: MenuAction, index: number) => {
    if (item.label === '') {
      return (
        <div
          key={index}
          style={{
            borderTop: '1px solid #404040',
            margin: '8px 0',
            height: 0,
          }}
        />
      )
    }

    const hasSubmenu = item.submenu && item.submenu.length > 0
    const isOpen = submenu === item.label

    return (
      <div
        key={index}
        onMouseEnter={() => hasSubmenu && setSubmenu(item.label)}
        style={{ position: 'relative' }}
      >
        <div
          onClick={() => {
            if (!hasSubmenu) {
              onAction(item.action)
              onClose()
            }
          }}
          style={{
            padding: '8px 16px',
            cursor: hasSubmenu ? 'default' : 'pointer',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            backgroundColor: isOpen ? '#404040' : 'transparent',
            color: '#e0e0e0',
            fontSize: 13,
            userSelect: 'none',
            minWidth: 280,
          }}
          onMouseOver={(e) => {
            (e.currentTarget as HTMLElement).style.backgroundColor = '#404040'
          }}
          onMouseOut={(e) => {
            (e.currentTarget as HTMLElement).style.backgroundColor = isOpen ? '#404040' : 'transparent'
          }}
        >
          <span>{item.label}</span>
          {item.shortcut && <span style={{ color: '#888', fontSize: 12, marginLeft: 16 }}>{item.shortcut}</span>}
          {hasSubmenu && <span style={{ marginLeft: 16, color: '#999' }}>→</span>}
        </div>

        {isOpen && hasSubmenu && (
          <div
            style={{
              position: 'absolute',
              left: '100%',
              top: 0,
              backgroundColor: '#2a2a2a',
              borderRadius: 6,
              padding: '8px 0',
              marginLeft: -8,
              boxShadow: '0 4px 12px rgba(0, 0, 0, 0.3)',
              zIndex: 1001,
            }}
          >
            {item.submenu!.map((subitem, idx) => (
              <div
                key={idx}
                onClick={() => {
                  onAction(item.action, subitem.label)
                  onClose()
                }}
                style={{
                  padding: '8px 16px',
                  cursor: 'pointer',
                  color: '#e0e0e0',
                  fontSize: 13,
                  whiteSpace: 'nowrap',
                }}
                onMouseOver={(e) => {
                  (e.currentTarget as HTMLElement).style.backgroundColor = '#404040'
                }}
                onMouseOut={(e) => {
                  (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'
                }}
              >
                {subitem.label}
              </div>
            ))}
          </div>
        )}
      </div>
    )
  }

  return (
    <div
      ref={menuRef}
      style={{
        position: 'fixed',
        left: position.x,
        top: position.y,
        backgroundColor: '#2a2a2a',
        borderRadius: 6,
        padding: '8px 0',
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.3)',
        zIndex: 1000,
      }}
    >
      {menuActions.map((item, idx) => renderMenuItem(item, idx))}
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/CellContextMenu.tsx
git commit -m "feat: add CellContextMenu with 15+ items and submenus"
```

---

## Task 7: Fix Empty String Bug in DataGrid

**Files:**
- Modify: `web/src/components/DataGrid.tsx:196-197` (cell rendering)

- [ ] **Step 1: Read current cell rendering logic**

```bash
grep -n "onDoubleClick" /home/conray/project/mysqlweb/web/src/components/DataGrid.tsx | head -5
```

Expected output shows lines around 196.

- [ ] **Step 2: Update cell renderer to always show clickable content**

Replace this:
```typescript
if (v === null || v === undefined) return <span onDoubleClick={startEdit} style={{ color: '#999' }}>NULL</span>
return <span onDoubleClick={startEdit}>{String(v)}</span>
```

With this:
```typescript
if (v === null || v === undefined) {
  return <span onDoubleClick={startEdit} onContextMenu={(e) => handleCellContextMenu(e, rowIdx, idx, v)} style={{ color: '#999' }}>NULL</span>
}
if (v === '') {
  // Empty string: show placeholder dot
  return <span onDoubleClick={startEdit} onContextMenu={(e) => handleCellContextMenu(e, rowIdx, idx, v)} style={{ color: '#ccc' }}>·</span>
}
return <span onDoubleClick={startEdit} onContextMenu={(e) => handleCellContextMenu(e, rowIdx, idx, v)}>{String(v)}</span>
```

- [ ] **Step 3: Commit**

```bash
git add web/src/components/DataGrid.tsx
git commit -m "fix: make empty strings and all values clickable, add context menu support"
```

---

## Task 8: Integrate Components into DataGrid

**Files:**
- Modify: `web/src/components/DataGrid.tsx` (comprehensive integration)

- [ ] **Step 1: Add state and hooks for context menu and modal**

Add at the top of the component function:
```typescript
const { position, cellInfo, cellValue, handleContextMenu, closeMenu } = useContextMenu()
const [showEditModal, setShowEditModal] = useState(false)
const [editingValue, setEditingValue] = useState<any>(null)
```

- [ ] **Step 2: Add handler for context menu actions**

Add this function inside DataGrid:
```typescript
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
        setShowEditModal(true)
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
        // Copy entire row as tab-separated
        const rowData = data.rows[rowIdx]
        const tsvRow = rowData.map((v) => (v === null ? '' : String(v))).join('\t')
        await navigator.clipboard.writeText(tsvRow)
        break

      case 'copy-cell':
        const cellValueStr = cellValue === null ? 'NULL' : String(cellValue)
        await navigator.clipboard.writeText(cellValueStr)
        break

      case 'copy-column':
        const { copyColumnAsTabSeparated } = await import('../lib/copyFormats')
        const colIdx = cellInfo.colIdx
        const colCopy = copyColumnAsTabSeparated(colName, data.rows, colIdx)
        await navigator.clipboard.writeText(colCopy)
        break

      case 'copy-as':
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
        if (copyText) await navigator.clipboard.writeText(copyText)
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

      case 'quick-look':
        setEditingValue(cellValue)
        setShowEditModal(true)
        break

      case 'refresh':
        reload()
        break

      case 'paste':
        const pastedText = await navigator.clipboard.readText()
        await api.patch(dataPath('/rows'), { pk_values: pk, column: colName, new_value: pastedText })
        reload()
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
```

- [ ] **Step 3: Update cell rendering to include context menu handler**

In the `columns` useMemo, update the cell function to:
```typescript
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
    return (
      <span
        onDoubleClick={startEdit}
        onContextMenu={(e) => handleContextMenu(e, rowIdx, idx, v)}
        style={{ color: '#999', cursor: 'context-menu' }}
      >
        NULL
      </span>
    )
  }
  if (v === '') {
    return (
      <span
        onDoubleClick={startEdit}
        onContextMenu={(e) => handleContextMenu(e, rowIdx, idx, v)}
        style={{ color: '#ccc', cursor: 'context-menu' }}
      >
        ·
      </span>
    )
  }
  return (
    <span
      onDoubleClick={startEdit}
      onContextMenu={(e) => handleContextMenu(e, rowIdx, idx, v)}
      style={{ cursor: 'context-menu' }}
    >
      {String(v)}
    </span>
  )
}
```

- [ ] **Step 4: Render CellContextMenu and EditCellModal**

Before the closing `</div>` of the component, add:
```typescript
{position && (
  <CellContextMenu
    position={position}
    cellValue={cellValue}
    columnName={cellInfo ? data?.columns[cellInfo.colIdx] : ''}
    onAction={handleMenuAction}
    onClose={closeMenu}
  />
)}

{showEditModal && (
  <EditCellModal
    value={editingValue}
    columnName={cellInfo ? data?.columns[cellInfo.colIdx] : ''}
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
```

- [ ] **Step 5: Add imports**

At the top of DataGrid.tsx, add:
```typescript
import { useContextMenu } from './useContextMenu'
import { CellContextMenu } from './CellContextMenu'
import { EditCellModal } from './EditCellModal'
```

- [ ] **Step 6: Commit**

```bash
git add web/src/components/DataGrid.tsx
git commit -m "feat: integrate context menu, edit modal, and menu actions into DataGrid"
```

---

## Task 9: Build and Test

**Files:**
- Build artifact: `web/dist/` (verify bundle works)

- [ ] **Step 1: Build frontend**

```bash
cd /home/conray/project/mysqlweb/web && npm run build
```

Expected: No errors, bundle size warnings are OK.

- [ ] **Step 2: Rebuild Go binary**

```bash
cd /home/conray/project/mysqlweb && go build -o bin/dataseai ./cmd/dataseai && go test ./...
```

Expected: All tests pass.

- [ ] **Step 3: Start server and manual test**

```bash
pkill -9 -f dataseai || true
cd /home/conray/project/mysqlweb && \
MYSQLWEB_DB_PATH=./data/dataseai.db MYSQLWEB_PORT=53306 setsid ./bin/dataseai > logs/dataseai.log 2>&1 &
sleep 2
curl http://127.0.0.1:53306/api/health | jq .
```

Expected: Server starts, health check returns `ok: true`.

- [ ] **Step 4: Manual feature test checklist**

In browser at http://127.0.0.1:53306/:
1. ✅ Login/register
2. ✅ Connect to a database
3. ✅ Browse a table with data
4. ✅ Right-click on a cell → menu appears with all items
5. ✅ Click "Copy Cell Value" → value copied to clipboard
6. ✅ Click "Edit in modal" → modal opens with editor
7. ✅ Try editing a string value → apply → see update
8. ✅ Find a cell with empty string → right-click → "Set Value EMPTY" → verify
9. ✅ Find a JSON cell (if exists) → right-click → "Edit in modal" → tree appears
10. ✅ Try expanding/collapsing JSON nodes → works
11. ✅ Try double-clicking empty cell → inline editor appears

- [ ] **Step 5: Commit full integration**

```bash
git add -A
git commit -m "feat: complete context menu implementation with all features"
```

---

## Summary

This plan implements:
- ✅ 15+ context menu items with submenus
- ✅ Edit modal for simple and JSON values
- ✅ JSON tree editor with left tree + right editor
- ✅ Copy As multiple formats (JSON, TSV, Markdown, Insert)
- ✅ Bug fix for empty string editing
- ✅ Set Value quick actions
- ✅ Refresh, Paste, Duplicate, Delete actions

Total tasks: 9 (hooks, utils, 3 editors, menu, bug fix, integration, test)
