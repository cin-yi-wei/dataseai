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
      <span style={divider} />
      <span style={label}>{t('bottom_tabs.dbwide_label')}</span>
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
    </div>
  )
}

const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 4, padding: '4px 8px',
  background: 'var(--bg-secondary)', color: 'var(--text-primary)',
  borderTop: '1px solid var(--border-color)', minHeight: 44,
  overflowX: 'auto', overflowY: 'hidden',
  scrollbarWidth: 'thin',
  WebkitOverflowScrolling: 'touch',
  touchAction: 'pan-x',
}
const label: CSSProperties = {
  fontSize: 10, letterSpacing: 1, padding: '0 8px',
  borderRight: '1px solid var(--border-color)', color: 'var(--text-muted)',
  flexShrink: 0,
}
const divider: CSSProperties = {
  width: 1, height: 22, background: 'var(--border-color)', flexShrink: 0, margin: '0 4px',
}
const tab: CSSProperties = {
  background: 'transparent', color: 'var(--text-secondary)', border: 'none', padding: '8px 12px',
  borderRadius: '3px 3px 0 0', fontSize: 12, cursor: 'pointer', flexShrink: 0,
  whiteSpace: 'nowrap',
}
const tabActive: CSSProperties = { background: 'var(--bg-hover)', color: 'var(--text-primary)' }
