import { useState, useEffect, useRef } from 'react'
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export interface Filter {
  enabled: boolean
  column: string
  operator: string
  value: string
}

interface FilterBarProps {
  columns: string[]
  initialFilters?: Filter[]
  history?: Filter[][]
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

export function FilterBar({ columns, initialFilters, history, onApply, onClose }: FilterBarProps) {
  const t = useT()
  const [filters, setFilters] = useState<Filter[]>(
    initialFilters && initialFilters.length > 0
      ? initialFilters
      : [{ enabled: true, column: columns[0] || '', operator: '=', value: '' }],
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
            setFilters((fs) => [...fs, { enabled: true, column: columns[0] || '', operator: '=', value: '' }])
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
    setFilters((fs) => [...fs, { enabled: true, column: columns[0] || '', operator: '=', value: '' }])
  }

  const removeFilter = (idx: number) => {
    setFilters((fs) => fs.length > 1 ? fs.filter((_, i) => i !== idx) : fs)
  }

  const clearAll = () => {
    setFilters([{ enabled: true, column: columns[0] || '', operator: '=', value: '' }])
    onApply([])
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
      {history && history.length > 0 && (
        <RecentFiltersDropdown
          history={history}
          onPick={(entry) => setFilters(entry.map((f) => ({ ...f })))}
        />
      )}

      {filters.map((f, idx) => (
        <div key={idx} style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 6, flexWrap: 'wrap' }}>
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
            placeholder={NO_VALUE_OPS.has(f.operator) ? t('filter.no_value_needed') : t('filter.pattern')}
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
            {t('filter.apply')}
          </button>
          <button
            onClick={() => removeFilter(idx)}
            disabled={filters.length === 1}
            style={{ ...btnStyle, padding: '4px 10px', minWidth: 36, fontSize: 16, lineHeight: 1 }}
            title="Remove (Ctrl+Shift+I)"
          >
            −
          </button>
          <button
            onClick={addFilter}
            style={{ ...btnStyle, padding: '4px 10px', minWidth: 36, fontSize: 16, lineHeight: 1 }}
            title="Add (Ctrl+I)"
          >
            +
          </button>
        </div>
      ))}

      <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginTop: 4, color: 'var(--text-muted)', fontSize: 11 }}>
        <span style={{ flex: 1 }}>
          {t('filter.hint')}
        </span>
        <button onClick={clearAll} style={btnStyle}>{t('filter.clear')}</button>
        <button
          onClick={applyAll}
          style={{ ...btnStyle, background: '#0066cc', color: 'white', border: 'none' }}
        >
          {t('filter.apply_all')}
        </button>
        <button onClick={onClose} style={btnStyle}>{t('common.close')}</button>
      </div>
    </div>
  )
}

function RecentFiltersDropdown({ history, onPick }: { history: Filter[][]; onPick: (entry: Filter[]) => void }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={wrapRef} style={{ position: 'relative', marginBottom: 8, display: 'flex', alignItems: 'center', gap: 6 }}>
      <span style={{ color: 'var(--text-muted)', fontSize: 11 }}>{t('filter.recent')}:</span>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        style={{ ...inputStyle, flex: 1, textAlign: 'left', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}
      >
        <span style={{ color: 'var(--text-muted)' }}>{t('filter.recent_pick')}</span>
        <span style={{ color: 'var(--text-muted)', marginLeft: 6 }}>▾</span>
      </button>
      {open && (
        <ul style={popoverStyle} role="listbox">
          {history.map((entry, i) => (
            <li
              key={i}
              role="option"
              tabIndex={0}
              onClick={() => { onPick(entry); setOpen(false) }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onPick(entry); setOpen(false)
                }
              }}
              style={popoverItemStyle}
            >
              <FilterSummaryRow entry={entry} />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function FilterSummaryRow({ entry }: { entry: Filter[] }) {
  const enabled = entry.filter((f) => f.enabled)
  if (enabled.length === 0) return <span style={{ color: 'var(--text-muted)' }}>(empty)</span>
  return (
    <span style={{ display: 'flex', flexWrap: 'wrap', gap: 4, alignItems: 'center' }}>
      {enabled.map((f, i) => (
        <span key={i} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          <span style={colChipStyle}>{f.column}</span>
          <span style={opTextStyle}>{f.operator}</span>
          {f.operator !== 'IS NULL' && f.operator !== 'IS NOT NULL' && (
            <span style={valTextStyle}>
              {f.value.length > 24 ? f.value.slice(0, 24) + '…' : f.value}
            </span>
          )}
          {i < enabled.length - 1 && <span style={{ color: 'var(--text-muted)', margin: '0 2px' }}>·</span>}
        </span>
      ))}
    </span>
  )
}

const popoverStyle: CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 2px)',
  left: 60,
  right: 0,
  margin: 0,
  padding: 4,
  listStyle: 'none',
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border-strong)',
  borderRadius: 4,
  boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
  zIndex: 50,
  maxHeight: 240,
  overflowY: 'auto',
}

const popoverItemStyle: CSSProperties = {
  padding: '6px 8px',
  borderRadius: 3,
  cursor: 'pointer',
  fontSize: 12,
  lineHeight: '1.5',
}

const colChipStyle: CSSProperties = {
  background: 'var(--accent, #4a8)',
  color: 'white',
  padding: '1px 6px',
  borderRadius: 3,
  fontFamily: 'monospace',
  fontSize: 11,
  fontWeight: 600,
}

const opTextStyle: CSSProperties = {
  color: 'var(--text-muted, #888)',
  fontStyle: 'italic',
  fontSize: 11,
}

const valTextStyle: CSSProperties = {
  color: 'var(--text-primary)',
  fontFamily: 'monospace',
  fontSize: 11,
  background: 'var(--bg-secondary)',
  padding: '0 4px',
  borderRadius: 2,
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
