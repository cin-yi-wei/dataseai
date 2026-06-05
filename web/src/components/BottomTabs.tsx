import type { CSSProperties } from 'react'
import { useT } from '../i18n'
import type { MessageKey } from '../i18n'

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

const LEFT: { key: BottomTab; icon: string; labelKey: MessageKey }[] = [
  { key: 'data', icon: '📊', labelKey: 'bottom_tabs.data' },
  { key: 'structure', icon: '🏗', labelKey: 'bottom_tabs.structure' },
  { key: 'indexes', icon: '🔑', labelKey: 'bottom_tabs.indexes' },
  { key: 'fks', icon: '🔗', labelKey: 'bottom_tabs.fks' },
]

const RIGHT: { key: BottomTab; icon: string; labelKey: MessageKey; enabled: boolean }[] = [
  { key: 'sql', icon: '⌨', labelKey: 'bottom_tabs.sql_editor', enabled: true },
  { key: 'chat', icon: '🤖', labelKey: 'bottom_tabs.ai_chat', enabled: true },
]

export default function BottomTabs({ value, onChange, hasTable = false }: Props) {
  const t = useT()
  return (
    <div data-bottom-tabs style={bar}>
      <span style={label}>{t('bottom_tabs.table_label')}</span>
      {LEFT.map((item) => {
        const enabled = hasTable
        const active = item.key === value
        return (
          <button
            key={item.key}
            onClick={() => enabled && onChange(item.key)}
            disabled={!enabled}
            style={{ ...tab, ...(active ? tabActive : null), opacity: enabled ? 1 : 0.4 }}
          >
            {item.icon} {t(item.labelKey)}
          </button>
        )
      })}
      <span style={{ flex: 1 }} />
      {RIGHT.map((item) => {
        const active = item.key === value
        return (
          <button
            key={item.key}
            onClick={() => item.enabled && onChange(item.key)}
            disabled={!item.enabled}
            style={{ ...tab, ...(active ? tabActive : null), opacity: item.enabled ? 1 : 0.4 }}
          >
            {item.icon} {t(item.labelKey)}
          </button>
        )
      })}
      <span style={label}>{t('bottom_tabs.dbwide_label')}</span>
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
