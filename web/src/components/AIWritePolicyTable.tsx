import { useState } from 'react'
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export interface AIPolicy {
  insert: boolean
  update: boolean
  delete: boolean
  ddl: boolean
}

export interface TablePolicy {
  table: string
  policy: AIPolicy
}

interface Props {
  connId: number
  db: string
  configured: TablePolicy[]
  unconfigured: string[]
  onUpsert: (connId: number, db: string, table: string, policy: AIPolicy) => void
  onBatch: (connId: number, db: string, tables: string[], policy: AIPolicy) => void
  // Flip a single flag across every configured table while leaving the
  // other three flags on each table untouched. Settings provides an
  // implementation that loops upserts and reloads once at the end.
  onColumnSetAll?: (connId: number, db: string, flag: Flag, value: boolean) => Promise<void>
}

// Named presets shown as one-click chips above the configured table.
const PRESETS = [
  { key: 'readonly',  i18n: 'settings.ai_writes.preset.readonly'  as const, emoji: '🔒', policy: { insert: false, update: false, delete: false, ddl: false } },
  { key: 'edit_only', i18n: 'settings.ai_writes.preset.edit_only' as const, emoji: '📝', policy: { insert: false, update: true,  delete: false, ddl: false } },
  { key: 'safe',      i18n: 'settings.ai_writes.preset.safe'      as const, emoji: '➕', policy: { insert: true,  update: true,  delete: false, ddl: false } },
  { key: 'allow_all', i18n: 'settings.ai_writes.preset.allow_all' as const, emoji: '⚠️', policy: { insert: true,  update: true,  delete: true,  ddl: true  } },
]

const FLAGS = ['insert', 'update', 'delete', 'ddl'] as const
type Flag = (typeof FLAGS)[number]

const FLAG_TO_COL: Record<Flag, 'ins' | 'upd' | 'del' | 'ddl'> = {
  insert: 'ins', update: 'upd', delete: 'del', ddl: 'ddl',
}

export default function AIWritePolicyTable({ connId, db, configured, unconfigured, onUpsert, onBatch, onColumnSetAll }: Props) {
  const t = useT()
  return (
    <div>
      <h4>{t('settings.ai_writes.configured', { count: configured.length })}</h4>
      {configured.length > 0 && (
        <PresetChips
          tableCount={configured.length}
          onApply={(p) => onBatch(connId, db, configured.map((c) => c.table), p)}
        />
      )}
      <div style={tableScroll}>
        <table style={tbl}>
          <thead>
            <tr>
              <th style={thTable}>{t('settings.ai_writes.col.table')}</th>
              {FLAGS.map((f) => (
                <ColumnHeader
                  key={f}
                  flag={f}
                  label={t(`settings.ai_writes.col.${FLAG_TO_COL[f]}` as const)}
                  configured={configured}
                  disabled={!onColumnSetAll}
                  onSetAll={(value) => onColumnSetAll && onColumnSetAll(connId, db, f, value)}
                />
              ))}
              <th style={thFlag}>{t('settings.ai_writes.col.select_all')}</th>
            </tr>
          </thead>
          <tbody>
            {configured.map((row) => (
              <ConfiguredRow
                key={row.table}
                row={row}
                onChange={(p) => onUpsert(connId, db, row.table, p)}
              />
            ))}
          </tbody>
        </table>
      </div>

      <h4>{t('settings.ai_writes.unconfigured', { count: unconfigured.length })}</h4>
      <UnconfiguredBatch
        connId={connId}
        db={db}
        tables={unconfigured}
        onBatch={onBatch}
      />
    </div>
  )
}

// PresetChips renders the four canonical policy combinations as one-click
// chips. Clicking applies that combination to every already-configured
// table. Replacement, not patch — flags not in the preset are forced to
// the preset's value too.
function PresetChips({ tableCount, onApply }: { tableCount: number; onApply: (p: AIPolicy) => void }) {
  const t = useT()
  return (
    <div style={batchBar}>
      <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
        {t('settings.ai_writes.preset_label', { n: tableCount })}
      </span>
      {PRESETS.map((preset) => (
        <button
          key={preset.key}
          onClick={() => onApply(preset.policy)}
          style={presetChip}
          data-testid={`preset-${preset.key}`}
        >
          {preset.emoji} {t(preset.i18n)}
        </button>
      ))}
    </div>
  )
}

// ColumnHeader renders a tri-state checkbox header. The state is derived
// from the configured rows: all-true (checked), all-false (unchecked),
// or mixed (indeterminate). Clicking flips to the inverse of the most
// common state — indeterminate behaves like unchecked → checked.
function ColumnHeader({ flag, label, configured, onSetAll, disabled }: {
  flag: Flag; label: string; configured: TablePolicy[];
  onSetAll: (value: boolean) => void | Promise<void>; disabled: boolean;
}) {
  const total = configured.length
  const onCount = configured.filter((c) => c.policy[flag]).length
  const allOn  = total > 0 && onCount === total
  const allOff = onCount === 0
  const mixed  = !allOn && !allOff
  return (
    <th style={thFlag}>
      <label style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2, cursor: disabled ? 'default' : 'pointer' }}>
        <input
          type="checkbox"
          checked={allOn}
          ref={(el) => { if (el) el.indeterminate = mixed }}
          onChange={() => onSetAll(!allOn)}
          disabled={disabled || total === 0}
          aria-label={`${label} column toggle`}
          title={allOn ? 'click to clear column' : 'click to set column'}
        />
        <span style={{ fontSize: 11, whiteSpace: 'nowrap' }}>{label}</span>
      </label>
    </th>
  )
}

function ConfiguredRow({ row, onChange }: { row: TablePolicy; onChange: (p: AIPolicy) => void }) {
  const set = (flag: Flag, value: boolean) => {
    onChange({ ...row.policy, [flag]: value })
  }
  const allOn = FLAGS.every((f) => row.policy[f])
  const toggleAll = () => {
    const v = !allOn
    onChange({ insert: v, update: v, delete: v, ddl: v })
  }
  return (
    <tr>
      <td style={tdTable} title={row.table}>{row.table}</td>
      {FLAGS.map((f) => (
        <td key={f} style={tdFlag}>
          <input
            type="checkbox"
            checked={row.policy[f]}
            onChange={(e) => set(f, e.target.checked)}
            aria-label={`${row.table} ${f}`}
          />
        </td>
      ))}
      <td style={tdFlag}>
        <input
          type="checkbox"
          checked={allOn}
          onChange={toggleAll}
          data-testid="select-all"
          aria-label={`${row.table} select-all`}
        />
      </td>
    </tr>
  )
}

function UnconfiguredBatch({ connId, db, tables, onBatch }: { connId: number; db: string; tables: string[]; onBatch: Props['onBatch'] }) {
  const t = useT()
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const flip = (table: string) => {
    setSelected((s) => {
      const next = new Set(s)
      if (next.has(table)) next.delete(table); else next.add(table)
      return next
    })
  }

  const allSelected = tables.length > 0 && selected.size === tables.length
  const someSelected = selected.size > 0 && selected.size < tables.length
  const toggleAll = () => {
    setSelected(allSelected ? new Set() : new Set(tables))
  }

  const applyPreset = (p: AIPolicy) => {
    if (selected.size === 0) return
    onBatch(connId, db, Array.from(selected), p)
    setSelected(new Set())
  }

  return (
    <div>
      <div style={batchBar}>
        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
          {selected.size === 0
            ? t('settings.ai_writes.unconfigured_hint')
            : t('settings.ai_writes.preset_label', { n: selected.size })}
        </span>
        {PRESETS.map((preset) => (
          <button
            key={preset.key}
            onClick={() => applyPreset(preset.policy)}
            style={{ ...presetChip, opacity: selected.size === 0 ? 0.4 : 1 }}
            disabled={selected.size === 0}
            data-testid={`unconfigured-preset-${preset.key}`}
          >
            {preset.emoji} {t(preset.i18n)}
          </button>
        ))}
      </div>
      <table style={tblNarrow}>
        <thead>
          <tr>
            <th style={tdFlag}>
              <input
                type="checkbox"
                checked={allSelected}
                ref={(el) => { if (el) el.indeterminate = someSelected }}
                onChange={toggleAll}
                aria-label="select all tables"
                title={allSelected ? t('settings.ai_writes.select_none') : t('settings.ai_writes.select_all_tables')}
              />
            </th>
            <th style={th}>
              <span style={{ fontWeight: 'normal', fontSize: 12, color: 'var(--text-muted)' }}>
                {allSelected
                  ? t('settings.ai_writes.select_none')
                  : t('settings.ai_writes.select_all_tables')}
              </span>
            </th>
          </tr>
        </thead>
        <tbody>
          {tables.map((tname) => (
            <tr key={tname}>
              <td style={tdFlag}>
                <input
                  type="checkbox"
                  checked={selected.has(tname)}
                  onChange={() => flip(tname)}
                  aria-label={`select ${tname}`}
                />
              </td>
              <td style={tdTable} title={tname}>{tname}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const tableScroll: CSSProperties = { overflowX: 'auto', WebkitOverflowScrolling: 'touch' }
const tbl: CSSProperties = { width: '100%', borderCollapse: 'collapse', fontSize: 13, minWidth: 360 }
// Narrow variant — used by the unconfigured table which only has 2 columns
// (select checkbox + table name). No minWidth so it fits without scroll.
const tblNarrow: CSSProperties = { width: '100%', borderCollapse: 'collapse', fontSize: 13, tableLayout: 'fixed' }
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)' }
const thTable: CSSProperties = { ...th }
// Flag-column header/cell: narrow + centered so 5 of them fit on mobile.
const thFlag: CSSProperties = {
  textAlign: 'center', padding: '4px 2px', borderBottom: '1px solid var(--border-color)',
  width: 36, minWidth: 32, whiteSpace: 'nowrap',
}
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid var(--table-border, var(--border-color))' }
const tdTable: CSSProperties = {
  ...td, maxWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis',
  whiteSpace: 'nowrap', fontFamily: 'monospace',
}
const tdFlag: CSSProperties = {
  padding: '4px 2px', borderBottom: '1px solid var(--table-border, var(--border-color))',
  textAlign: 'center', width: 36, minWidth: 32,
}
const batchBar: CSSProperties = { padding: '4px 0', display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }
const presetChip: CSSProperties = {
  padding: '4px 12px', fontSize: 12, borderRadius: 16,
  border: '1px solid var(--border-color)', background: 'transparent',
  cursor: 'pointer',
}
