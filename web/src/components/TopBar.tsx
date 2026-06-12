import { useAuth } from '../store/auth'
import { useTheme } from '../store/theme'
import { useT } from '../i18n'
import ConnectionPicker from './ConnectionPicker'
import LangSwitcher from './LangSwitcher'

interface Props {
  onOpenConnections: () => void
  onOpenSettings: () => void
  onOpenAdmin?: () => void
}

export default function TopBar({ onOpenConnections, onOpenSettings, onOpenAdmin }: Props) {
  const user = useAuth((s) => s.user!)
  const logout = useAuth((s) => s.logout)
  const theme = useTheme((s) => s.theme)
  const toggleTheme = useTheme((s) => s.toggle)
  const t = useT()
  return (
    <header
      data-topbar
      style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '8px 16px',
        borderBottom: '1px solid var(--border-color)',
        background: 'var(--bg-secondary)',
        color: 'var(--text-primary)',
      }}
    >
      <div
        data-topbar-brand
        style={{ display: 'flex', alignItems: 'center', gap: 8, marginRight: 8, minWidth: 0 }}
      >
        <img src="/logo.svg" alt="dataseai" width={28} height={28} style={{ borderRadius: 6 }} />
        <strong>dataseai</strong>
      </div>
      <div data-topbar-picker style={{ minWidth: 0 }}>
        <ConnectionPicker />
      </div>
      <button data-topbar-manage onClick={onOpenConnections}>{t('topbar.manage')}</button>
      <div data-topbar-actions style={{ display: 'contents' }}>
        <div data-topbar-spacer style={{ flex: 1 }} />
        <span data-hide-mobile style={{ fontSize: 13 }}>{user.username}</span>
        <LangSwitcher />
        <button onClick={toggleTheme} title={theme === 'light' ? t('topbar.dark_mode_tooltip') : t('topbar.light_mode_tooltip')}>
          {theme === 'light' ? '🌙' : '☀️'}
        </button>
        {user.is_admin && onOpenAdmin && (
          <button onClick={onOpenAdmin} title={t('topbar.admin')}>⚙️ {t('topbar.admin')}</button>
        )}
        <button onClick={onOpenSettings}>{t('topbar.settings')}</button>
        <button onClick={() => logout()}>{t('topbar.logout')}</button>
      </div>
    </header>
  )
}
