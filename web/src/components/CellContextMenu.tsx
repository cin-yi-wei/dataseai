import { useRef, useEffect, useLayoutEffect, useState } from 'react'
import { useT } from '../i18n'

export interface MenuAction {
  label: string
  value?: string // stable dispatch key (independent of label translation)
  shortcut?: string
  action: 'edit' | 'set-value' | 'copy' | 'copy-cell' | 'copy-column' | 'copy-as' | 'quick-filter' | 'quick-look' |
           'refresh' | 'paste' | 'add-row' | 'duplicate' | 'delete-row' |
           'copy-selected' | 'copy-selected-as' | 'delete-selected'
  submenu?: MenuAction[]
}

interface CellContextMenuProps {
  position: { x: number; y: number }
  cellValue: any
  columnName: string
  selectedCount?: number
  onAction: (action: string, subaction?: string) => void
  onClose: () => void
}

export function CellContextMenu({ position, cellValue: _unused1, columnName: _unused2, selectedCount = 1, onAction, onClose }: CellContextMenuProps) {
  // Note: cellValue and columnName are provided by parent but not used in current implementation
  const t = useT()
  const menuRef = useRef<HTMLDivElement>(null)
  const [submenu, setSubmenu] = useState<string | null>(null)
  // 窄螢幕（手機）：二級選單改成「就地展開」（縮排列在父項下方），整個選單可捲動，
  // 避免浮動側開在小螢幕定位不到、被切掉。
  const [isNarrow, setIsNarrow] = useState(() => typeof window !== 'undefined' && window.innerWidth < 768)
  useEffect(() => {
    const onResize = () => setIsNarrow(window.innerWidth < 768)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  // Clamp the menu into the viewport so it isn't cut off near an edge.
  const [pos, setPos] = useState(position)
  useLayoutEffect(() => {
    const el = menuRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const pad = 8
    let x = position.x
    let y = position.y
    if (x + r.width > window.innerWidth) x = Math.max(pad, window.innerWidth - r.width - pad)
    if (y + r.height > window.innerHeight) y = Math.max(pad, window.innerHeight - r.height - pad)
    setPos({ x, y })
    // 手機就地展開子選單會把選單撐高，需要重新夾制把整個選單往上移到畫面內
    // （高度已被 maxHeight 限制，內部再捲動）。因此 submenu/isNarrow 改變也要重跑。
  }, [position, submenu, isNarrow])

  // 二級選單（submenu）的視窗夾制：預設往右開，右邊放不下就翻到左邊；下方超出就往上移。
  // 第一層已有夾制，第二層先前沒有，靠近邊緣會顯示不到。直接量測後改 style，避免 state 迴圈。
  const submenuRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const el = submenuRef.current
    if (!el || !submenu) return
    const pad = 4
    // 先回到預設（往右、對齊父項頂端、無位移）再量測。
    el.style.left = '100%'; el.style.right = 'auto'
    el.style.marginLeft = '-8px'; el.style.marginRight = '0'
    el.style.top = '0'; el.style.transform = 'none'
    let r = el.getBoundingClientRect()
    // 右邊放不下 → 翻到父項左側。
    if (r.right > window.innerWidth - pad) {
      el.style.left = 'auto'; el.style.right = '100%'
      el.style.marginLeft = '0'; el.style.marginRight = '-8px'
    }
    // 下方超出 → 往上移。
    r = el.getBoundingClientRect()
    if (r.bottom > window.innerHeight - pad) {
      el.style.top = `${Math.min(0, window.innerHeight - pad - r.bottom)}px`
    }
    // 手機窄螢幕：左右都放不下時，最後再用 translateX 把整個子選單推進畫面內。
    r = el.getBoundingClientRect()
    let dx = 0
    if (r.right > window.innerWidth - pad) dx = window.innerWidth - pad - r.right
    if (r.left + dx < pad) dx = pad - r.left
    if (dx !== 0) el.style.transform = `translateX(${dx}px)`
  }, [submenu])

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

  const isMulti = selectedCount > 1

  const menuActions: MenuAction[] = isMulti
    ? [
        { label: t('menu.refresh'), shortcut: 'Ctrl Alt R', action: 'refresh' },
        { label: t('menu.add_row'), shortcut: 'Ctrl I', action: 'add-row' },
        { label: '', action: 'separator' as any },
        { label: t('menu.copy_selected', { count: selectedCount }), shortcut: 'Ctrl C', action: 'copy-selected' },
        {
          label: t('menu.copy_selected_as', { count: selectedCount }),
          action: 'copy-selected-as',
          submenu: [
            { label: t('menu.copy_as_json'), value: 'JSON', action: 'copy-selected-as' },
            { label: t('menu.copy_as_tsv'), value: 'TSV for Excel', action: 'copy-selected-as' },
            { label: t('menu.copy_as_markdown'), value: 'Markdown', action: 'copy-selected-as' },
            { label: t('menu.copy_as_insert'), value: 'Insert statement', action: 'copy-selected-as' },
          ],
        },
        { label: '', action: 'separator' as any },
        { label: t('menu.delete_selected', { count: selectedCount }), shortcut: 'Delete', action: 'delete-selected' },
      ]
    : [
        { label: t('menu.quick_look'), shortcut: 'Ctrl ↵', action: 'quick-look' },
        { label: '', action: 'separator' as any },
        { label: t('menu.edit_in_modal'), action: 'edit' },
        {
          label: t('menu.set_value'),
          action: 'set-value',
          submenu: [
            { label: t('menu.set_empty'), value: 'EMPTY', action: 'set-value' },
            { label: t('menu.set_null'), value: 'NULL', action: 'set-value' },
            { label: t('menu.set_default'), value: 'DEFAULT', action: 'set-value' },
          ],
        },
        { label: '', action: 'separator' as any },
        { label: t('menu.refresh'), shortcut: 'Ctrl Alt R', action: 'refresh' },
        // 最常用的「複製儲存格」提到最上面（貼上之上），不埋在子選單裡。
        { label: '複製儲存格', shortcut: 'Ctrl C', action: 'copy-cell' },
        { label: t('menu.paste'), shortcut: 'Ctrl V', action: 'paste' },
        { label: t('menu.add_row'), shortcut: 'Ctrl I', action: 'add-row' },
        { label: '建立副本', shortcut: 'Ctrl D', action: 'duplicate' },
        { label: '', action: 'separator' as any },
        {
          // 其餘剪貼簿複製整併成子選單，依範圍選（儲存格已提到上面）。
          label: t('menu.copy'),
          action: 'copy',
          submenu: [
            { label: '整列', action: 'copy' },
            { label: '整欄', action: 'copy-column' },
            { label: t('menu.copy_as_json'), value: 'JSON', action: 'copy-as' },
            { label: t('menu.copy_as_tsv'), value: 'TSV for Excel', action: 'copy-as' },
            { label: t('menu.copy_as_markdown'), value: 'Markdown', action: 'copy-as' },
            { label: t('menu.copy_as_insert'), value: 'Insert statement', action: 'copy-as' },
          ],
        },
        { label: '', action: 'separator' as any },
        {
          label: t('menu.quick_filter'),
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
        { label: t('menu.delete_row'), shortcut: 'Delete', action: 'delete-row' },
      ]

  const renderMenuItem = (item: MenuAction, index: number) => {
    if (item.label === '') {
      return (
        <div
          key={index}
          style={{
            borderTop: '1px solid var(--menu-border)',
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
        style={{ position: 'relative' }}
      >
        <div
          onClick={() => {
            if (hasSubmenu) {
              // Toggle submenu (mobile-friendly: click instead of hover)
              setSubmenu((cur) => (cur === item.label ? null : item.label))
            } else {
              onAction(item.action)
              // Note: onClose is called by handleMenuAction itself
            }
          }}
          style={{
            padding: '8px 16px',
            cursor: 'pointer',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            backgroundColor: isOpen ? 'var(--menu-hover)' : 'transparent',
            color: 'var(--menu-text)',
            fontSize: 13,
            userSelect: 'none',
            minWidth: 280,
          }}
          onMouseOver={(e) => {
            (e.currentTarget as HTMLElement).style.backgroundColor = 'var(--menu-hover)'
          }}
          onMouseOut={(e) => {
            (e.currentTarget as HTMLElement).style.backgroundColor = isOpen ? 'var(--menu-hover)' : 'transparent'
          }}
        >
          <span>{item.label}</span>
          {item.shortcut && <span style={{ color: 'var(--text-muted)', fontSize: 12, marginLeft: 16 }}>{item.shortcut}</span>}
          {hasSubmenu && <span style={{ marginLeft: 16, color: 'var(--text-muted)' }}>{isOpen ? '▾' : '▸'}</span>}
        </div>

        {/* 窄螢幕：就地展開（縮排、正常排版，跟著選單一起捲動）。 */}
        {isOpen && hasSubmenu && isNarrow && (
          <div style={{ backgroundColor: 'var(--menu-hover)' }}>
            {item.submenu!.map((subitem, idx) => (
              <div
                key={idx}
                onClick={() => onAction(subitem.action ?? item.action, subitem.value ?? subitem.label)}
                style={{
                  padding: '8px 16px 8px 36px',
                  cursor: 'pointer', color: 'var(--menu-text)', fontSize: 13,
                }}
                onMouseOver={(e) => { (e.currentTarget as HTMLElement).style.backgroundColor = 'var(--menu-hover)' }}
                onMouseOut={(e) => { (e.currentTarget as HTMLElement).style.backgroundColor = 'transparent' }}
              >
                {subitem.label}
              </div>
            ))}
          </div>
        )}

        {/* 桌機：浮動側開（含視窗夾制）。 */}
        {isOpen && hasSubmenu && !isNarrow && (
          <div
            ref={submenuRef}
            data-submenu
            style={{
              position: 'absolute',
              left: '100%',
              top: 0,
              backgroundColor: 'var(--menu-bg)',
              borderRadius: 6,
              padding: '8px 0',
              marginLeft: -8,
              boxShadow: '0 4px 12px var(--menu-shadow)',
              zIndex: 1001,
              maxHeight: '70vh',
              overflowY: 'auto',
            }}
          >
            {item.submenu!.map((subitem, idx) => (
              <div
                key={idx}
                onClick={() => {
                  // 子項可帶自己的 action（讓「複製」子選單能混合 copy-cell/copy/
                  // copy-column/copy-as）；沒帶就沿用父項 action。
                  onAction(subitem.action ?? item.action, subitem.value ?? subitem.label)
                  // Note: onClose is called by handleMenuAction itself
                }}
                style={{
                  padding: '8px 16px',
                  cursor: 'pointer',
                  color: 'var(--menu-text)',
                  fontSize: 13,
                  whiteSpace: 'nowrap',
                }}
                onMouseOver={(e) => {
                  (e.currentTarget as HTMLElement).style.backgroundColor = 'var(--menu-hover)'
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
        left: pos.x,
        top: pos.y,
        backgroundColor: 'var(--menu-bg)',
        borderRadius: 6,
        padding: '8px 0',
        boxShadow: '0 4px 12px var(--menu-shadow)',
        zIndex: 1000,
        // 手機：就地展開會把選單撐高，讓整個選單可捲動、不超出畫面。
        // 桌機不設 overflow，否則會裁掉浮動側開的二級選單。
        ...(isNarrow ? { maxHeight: '85vh', overflowY: 'auto' as const } : {}),
      }}
    >
      {menuActions.map((item, idx) => renderMenuItem(item, idx))}
    </div>
  )
}
