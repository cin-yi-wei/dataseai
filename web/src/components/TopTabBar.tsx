import type { CSSProperties } from 'react'
import { useActiveConn } from '../store/activeConn'
import { useTabs } from '../store/tabs'
import { useT } from '../i18n'

export default function TopTabBar() {
  const t = useT()
  const connId = useActiveConn((s) => s.activeId)
  const tabs = useTabs((s) => s.tabs)
  const activeId = useTabs((s) => s.activeId)
  const setActive = useTabs((s) => s.setActive)
  const close = useTabs((s) => s.close)
  const closeAll = useTabs((s) => s.closeAll)
  const open = useTabs((s) => s.open)

  return (
    <div style={bar}>
      <div style={scrollArea}>
        {tabs.map((tab_) => (
          <button
            key={tab_.id}
            type="button"
            onClick={() => setActive(tab_.id)}
            style={{
              ...tab,
              ...(tab_.id === activeId ? activeTab : null),
            }}
            title={tab_.title}
          >
            <span style={tabTitle}>{tab_.title}</span>
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation()
                close(tab_.id)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.stopPropagation()
                  close(tab_.id)
                }
              }}
              style={closeBtn}
              title={t('top_tab_bar.close_tab')}
            >
              x
            </span>
          </button>
        ))}
      </div>
      <button
        type="button"
        disabled={tabs.length === 0}
        onClick={() => {
          if (tabs.length === 0) return
          if (window.confirm(t('top_tab_bar.close_all_confirm'))) closeAll()
        }}
        style={addBtn}
        title={t('top_tab_bar.close_all_title')}
      >
        {t('top_tab_bar.close_all')}
      </button>
      <button
        type="button"
        disabled={connId == null}
        onClick={() => {
          if (connId != null) open({ kind: 'sql', connId })
        }}
        style={addBtn}
      >
        {t('top_tab_bar.add_sql')}
      </button>
    </div>
  )
}

const bar: CSSProperties = {
  display: 'flex', alignItems: 'flex-end', gap: 2,
  padding: '4px 8px 0', borderBottom: '1px solid var(--border-color)',
  background: 'var(--bg-secondary)', color: 'var(--text-primary)',
  fontFamily: 'system-ui', fontSize: 13,
  minWidth: 0,
}
const scrollArea: CSSProperties = {
  display: 'flex', alignItems: 'flex-end', gap: 2,
  flex: '1 1 auto', minWidth: 0,
  overflowX: 'auto', overflowY: 'hidden',
  scrollbarWidth: 'thin',
  WebkitOverflowScrolling: 'touch',
}
const tab: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, maxWidth: 180, flexShrink: 0,
  padding: '5px 10px', border: '1px solid transparent',
  borderBottom: '1px solid var(--border-color)', borderRadius: '4px 4px 0 0',
  background: 'transparent', color: 'var(--text-primary)', cursor: 'pointer',
}
const activeTab: CSSProperties = {
  background: 'var(--bg-primary)', borderColor: 'var(--border-strong)', borderBottomColor: 'var(--bg-primary)',
}
const tabTitle: CSSProperties = { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }
const closeBtn: CSSProperties = { color: 'var(--text-muted)', fontSize: 12, lineHeight: 1, padding: '1px 2px' }
const addBtn: CSSProperties = {
  marginLeft: 6, marginBottom: 4, padding: '3px 8px', flexShrink: 0,
  border: '1px solid var(--border-strong)', borderRadius: 4,
  background: 'var(--bg-elevated)', color: 'var(--text-primary)',
}
