import type { CSSProperties } from 'react'

export type BottomTab =
  | 'data'
  | 'structure'
  | 'indexes'
  | 'fks'
  | 'sql'
  | 'chat'

interface Props {
  value: BottomTab
  onChange: (t: BottomTab) => void
  hasTable?: boolean
}

const LEFT: { key: BottomTab; label: string }[] = [
  { key: 'data', label: '📊 Data' },
  { key: 'structure', label: '🏗 Structure' },
  { key: 'indexes', label: '🔑 Indexes' },
  { key: 'fks', label: '🔗 FK' },
]

const RIGHT: { key: BottomTab; label: string; enabled: boolean }[] = [
  { key: 'sql', label: '⌨ SQL Editor', enabled: true },
  { key: 'chat', label: '🤖 AI Chat', enabled: true },
]

export default function BottomTabs({ value, onChange, hasTable = false }: Props) {
  return (
    <div data-bottom-tabs style={bar}>
      <span style={label}>TABLE</span>
      {LEFT.map((t) => {
        const enabled = hasTable
        const active = t.key === value
        return (
          <button
            key={t.key}
            onClick={() => enabled && onChange(t.key)}
            disabled={!enabled}
            style={{ ...tab, ...(active ? tabActive : null), opacity: enabled ? 1 : 0.4 }}
          >
            {t.label}
          </button>
        )
      })}
      <span style={{ flex: 1 }} />
      {RIGHT.map((t) => {
        const active = t.key === value
        return (
          <button
            key={t.key}
            onClick={() => t.enabled && onChange(t.key)}
            disabled={!t.enabled}
            style={{ ...tab, ...(active ? tabActive : null), opacity: t.enabled ? 1 : 0.4 }}
          >
            {t.label}
          </button>
        )
      })}
      <span style={label}>DB-WIDE</span>
    </div>
  )
}

const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 4, padding: '0 8px',
  background: 'var(--bg-secondary)', color: 'var(--text-primary)',
  borderTop: '1px solid var(--border-color)', height: 30,
}
const label: CSSProperties = {
  fontSize: 10, letterSpacing: 1, padding: '0 8px',
  borderRight: '1px solid var(--border-color)', color: 'var(--text-muted)',
}
const tab: CSSProperties = {
  background: 'transparent', color: 'var(--text-secondary)', border: 'none', padding: '4px 10px',
  borderRadius: '3px 3px 0 0', fontSize: 12, cursor: 'pointer',
}
const tabActive: CSSProperties = { background: 'var(--bg-hover)', color: 'var(--text-primary)' }
