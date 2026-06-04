import { useState, useEffect, useRef } from 'react'

export interface Filter {
  enabled: boolean
  column: string
  operator: string
  value: string
}

interface FilterBarProps {
  columns: string[]
  initialFilters?: Filter[]
  onApply: (filters: Filter[]) => void
  onClose: () => void
}

const OPERATORS = [
  '=', '<>', '<', '>', '<=', '>=',
  'IN', 'NOT IN',
  'IS NULL', 'IS NOT NULL',
  'BETWEEN', 'NOT BETWEEN',
  'LIKE',
  'Contains', 'Not contains',
  'Has prefix', 'Has suffix',
]

const NO_VALUE_OPS = new Set(['IS NULL', 'IS NOT NULL'])

export function FilterBar({ columns, initialFilters, onApply, onClose }: FilterBarProps) {
  const [filters, setFilters] = useState<Filter[]>(
    initialFilters && initialFilters.length > 0
      ? initialFilters
      : [{ enabled: true, column: columns[0] || '', operator: 'Contains', value: '' }],
  )
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose()
        return
      }
      if (e.ctrlKey || e.metaKey) {
        if (e.key === 'i' || e.key === 'I') {
          e.preventDefault()
          if (e.shiftKey) {
            // Remove last
            setFilters((fs) => fs.length > 1 ? fs.slice(0, -1) : fs)
          } else {
            // Insert new
            setFilters((fs) => [...fs, { enabled: true, column: columns[0] || '', operator: 'Contains', value: '' }])
          }
        }
        if (e.key === 'Enter') {
          e.preventDefault()
          onApply(filters.filter((f) => f.enabled))
        }
      }
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [filters, columns, onApply, onClose])

  const updateFilter = (idx: number, patch: Partial<Filter>) => {
    setFilters((fs) => fs.map((f, i) => (i === idx ? { ...f, ...patch } : f)))
  }

  const addFilter = () => {
    setFilters((fs) => [...fs, { enabled: true, column: columns[0] || '', operator: 'Contains', value: '' }])
  }

  const removeFilter = (idx: number) => {
    setFilters((fs) => fs.length > 1 ? fs.filter((_, i) => i !== idx) : fs)
  }

  const clearAll = () => {
    setFilters([{ enabled: true, column: columns[0] || '', operator: 'Contains', value: '' }])
  }

  const applyAll = () => {
    onApply(filters.filter((f) => f.enabled))
  }

  return (
    <div
      ref={containerRef}
      style={{
        padding: 8,
        background: 'var(--bg-tertiary)',
        color: 'var(--text-primary)',
        borderBottom: '1px solid var(--border-color)',
        fontSize: 12,
      }}
    >
      {filters.map((f, idx) => (
        <div key={idx} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 6 }}>
          <input
            type="checkbox"
            checked={f.enabled}
            onChange={(e) => updateFilter(idx, { enabled: e.target.checked })}
            style={{ cursor: 'pointer' }}
          />
          <select
            value={f.column}
            onChange={(e) => updateFilter(idx, { column: e.target.value })}
            style={inputStyle}
          >
            {columns.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
          <select
            value={f.operator}
            onChange={(e) => updateFilter(idx, { operator: e.target.value })}
            style={{ ...inputStyle, width: 130 }}
          >
            {OPERATORS.map((op) => <option key={op} value={op}>{op}</option>)}
          </select>
          <input
            type="text"
            placeholder={NO_VALUE_OPS.has(f.operator) ? '(no value needed)' : 'Pattern'}
            value={f.value}
            disabled={NO_VALUE_OPS.has(f.operator)}
            onChange={(e) => updateFilter(idx, { value: e.target.value })}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.ctrlKey && !e.metaKey) {
                e.preventDefault()
                onApply(filters.filter((ff) => ff.enabled))
              }
            }}
            style={{ ...inputStyle, flex: 1 }}
          />
          <button
            onClick={() => {
              const single = [{ ...f, enabled: true }]
              onApply(single)
            }}
            style={btnStyle}
          >
            Apply
          </button>
          <button
            onClick={() => removeFilter(idx)}
            disabled={filters.length === 1}
            style={{ ...btnStyle, padding: '4px 8px' }}
            title="Remove (Ctrl+Shift+I)"
          >
            −
          </button>
          <button onClick={addFilter} style={{ ...btnStyle, padding: '4px 8px' }} title="Add (Ctrl+I)">+</button>
        </div>
      ))}

      <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginTop: 4, color: 'var(--text-muted)', fontSize: 11 }}>
        <span style={{ flex: 1 }}>
          Hide: ESC · Insert: Ctrl+I · Remove: Ctrl+Shift+I · Apply: Enter · Apply all: Ctrl+Enter
        </span>
        <button onClick={clearAll} style={btnStyle}>Clear</button>
        <button
          onClick={applyAll}
          style={{ ...btnStyle, background: '#0066cc', color: 'white', border: 'none' }}
        >
          Apply All
        </button>
        <button onClick={onClose} style={btnStyle}>Close</button>
      </div>
    </div>
  )
}

const inputStyle = {
  padding: '4px 6px',
  fontSize: 12,
  border: '1px solid var(--border-strong)',
  borderRadius: 3,
  background: 'var(--bg-input)',
  color: 'var(--text-primary)',
}

const btnStyle = {
  padding: '4px 10px',
  fontSize: 12,
  border: '1px solid var(--border-strong)',
  borderRadius: 3,
  cursor: 'pointer',
  background: 'var(--bg-elevated)',
  color: 'var(--text-primary)',
}
