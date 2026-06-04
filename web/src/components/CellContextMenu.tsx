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

export function CellContextMenu({ position, cellValue: _unused1, columnName: _unused2, onAction, onClose }: CellContextMenuProps) {
  // Note: cellValue and columnName are provided by parent but not used in current implementation
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
              // Note: onClose is called by handleMenuAction itself
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
                  // Note: onClose is called by handleMenuAction itself
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
