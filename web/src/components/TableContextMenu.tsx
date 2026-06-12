import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export type TableMenuAction =
  | 'open-new-tab'
  | 'open-structure'
  | 'copy-name'
  | 'toggle-pin'
  | 'export'
  | 'truncate'
  | 'drop'

interface Props {
  position: { x: number; y: number }
  tableName: string
  isPinned: boolean
  onAction: (action: TableMenuAction) => void
  onClose: () => void
}

interface Item {
  action: TableMenuAction
  label: string
  shortcut?: string
  danger?: boolean
}

export function TableContextMenu({ position, tableName: _tableName, isPinned, onAction, onClose }: Props) {
  const t = useT()
  const ref = useRef<HTMLDivElement>(null)
  // Clamp the menu into the viewport so it isn't cut off near an edge.
  const [pos, setPos] = useState(position)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const pad = 8
    let x = position.x
    let y = position.y
    if (x + r.width > window.innerWidth) x = Math.max(pad, window.innerWidth - r.width - pad)
    if (y + r.height > window.innerHeight) y = Math.max(pad, window.innerHeight - r.height - pad)
    setPos({ x, y })
  }, [position])

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
    }
  }, [onClose])

  const items: (Item | 'sep')[] = [
    { action: 'open-new-tab', label: t('table_menu.open_new_tab') },
    { action: 'open-structure', label: t('table_menu.open_structure') },
    'sep',
    { action: 'copy-name', label: t('table_menu.copy_name') },
    { action: 'toggle-pin', label: isPinned ? t('table_menu.unpin') : t('table_menu.pin') },
    'sep',
    { action: 'export', label: t('table_menu.export') },
    'sep',
    { action: 'truncate', label: t('table_menu.truncate'), danger: true },
    { action: 'drop', label: t('table_menu.drop'), danger: true },
  ]

  return (
    <div ref={ref} style={{ ...menu, left: pos.x, top: pos.y }}>
      {items.map((it, i) => {
        if (it === 'sep') {
          return <div key={i} style={separator} />
        }
        return (
          <div
            key={i}
            onClick={() => onAction(it.action)}
            style={{ ...itemStyle, color: it.danger ? '#ff6b6b' : '#e0e0e0' }}
            onMouseOver={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = '#404040')}
            onMouseOut={(e) => ((e.currentTarget as HTMLElement).style.backgroundColor = 'transparent')}
          >
            <span>{it.label}</span>
            {it.shortcut && <span style={shortcutStyle}>{it.shortcut}</span>}
          </div>
        )
      })}
    </div>
  )
}

const menu: CSSProperties = {
  position: 'fixed',
  backgroundColor: '#2a2a2a',
  borderRadius: 6,
  padding: '6px 0',
  boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
  zIndex: 1000,
  minWidth: 200,
}
const itemStyle: CSSProperties = {
  padding: '6px 14px',
  cursor: 'pointer',
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
  fontSize: 13,
  userSelect: 'none',
}
const separator: CSSProperties = {
  borderTop: '1px solid #404040',
  margin: '6px 0',
  height: 0,
}
const shortcutStyle: CSSProperties = { color: '#888', fontSize: 12, marginLeft: 16 }
