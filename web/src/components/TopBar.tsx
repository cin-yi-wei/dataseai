import { useAuth } from '../store/auth'
import { useTheme } from '../store/theme'
import ConnectionPicker from './ConnectionPicker'

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
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginRight: 8 }}>
        <img src="/logo.svg" alt="dataseai" width={28} height={28} style={{ borderRadius: 6 }} />
        <strong>dataseai</strong>
      </div>
      <ConnectionPicker />
      <button onClick={onOpenConnections}>manage</button>
      <div style={{ flex: 1 }} />
      <span data-hide-mobile style={{ fontSize: 13 }}>{user.username}</span>
      <button onClick={toggleTheme} title={`Switch to ${theme === 'light' ? 'dark' : 'light'} mode`}>
        {theme === 'light' ? '🌙' : '☀️'}
      </button>
      {user.is_admin && onOpenAdmin && (
        <button onClick={onOpenAdmin} title="Admin Panel">⚙️ admin</button>
      )}
      <button onClick={onOpenSettings}>settings</button>
      <button onClick={() => logout()}>log out</button>
    </header>
  )
}
