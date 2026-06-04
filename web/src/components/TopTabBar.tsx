import type { CSSProperties } from 'react'
import { useActiveConn } from '../store/activeConn'
import { useTabs } from '../store/tabs'

export default function TopTabBar() {
  const connId = useActiveConn((s) => s.activeId)
  const tabs = useTabs((s) => s.tabs)
  const activeId = useTabs((s) => s.activeId)
  const setActive = useTabs((s) => s.setActive)
  const close = useTabs((s) => s.close)
  const open = useTabs((s) => s.open)

  return (
    <div style={bar}>
      {tabs.map((t) => (
        <button
          key={t.id}
          type="button"
          onClick={() => setActive(t.id)}
          style={{
            ...tab,
            ...(t.id === activeId ? activeTab : null),
          }}
          title={t.title}
        >
          <span style={tabTitle}>{t.title}</span>
          <span
            role="button"
            tabIndex={0}
            onClick={(e) => {
              e.stopPropagation()
              close(t.id)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.stopPropagation()
                close(t.id)
              }
            }}
            style={closeBtn}
            title="close"
          >
            x
          </span>
        </button>
      ))}
      <button
        type="button"
        disabled={connId == null}
        onClick={() => {
          if (connId != null) open({ kind: 'sql', connId })
        }}
        style={addBtn}
      >
        + SQL
      </button>
    </div>
  )
}

const bar: CSSProperties = {
  display: 'flex', alignItems: 'flex-end', gap: 2,
  padding: '4px 8px 0', borderBottom: '1px solid var(--border-color)',
  background: 'var(--bg-secondary)', color: 'var(--text-primary)',
  fontFamily: 'system-ui', fontSize: 13,
}
const tab: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, maxWidth: 180,
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
  marginLeft: 6, marginBottom: 4, padding: '3px 8px',
  border: '1px solid var(--border-strong)', borderRadius: 4,
  background: 'var(--bg-elevated)', color: 'var(--text-primary)',
}
