import { useAuth } from '../store/auth'
import ConnectionPicker from './ConnectionPicker'

interface Props {
  onOpenConnections: () => void
  onOpenSettings: () => void
}

export default function TopBar({ onOpenConnections, onOpenSettings }: Props) {
  const user = useAuth((s) => s.user!)
  const logout = useAuth((s) => s.logout)
  return (
    <header
      style={{
        display: 'flex', alignItems: 'center', gap: 12,
        padding: '8px 16px', borderBottom: '1px solid #ddd', background: '#fafafa',
      }}
    >
      <strong style={{ marginRight: 8 }}>mysqlweb</strong>
      <ConnectionPicker />
      <button onClick={onOpenConnections}>manage</button>
      <div style={{ flex: 1 }} />
      <span style={{ fontSize: 13 }}>{user.username}</span>
      <button onClick={onOpenSettings}>settings</button>
      <button onClick={() => logout()}>log out</button>
    </header>
  )
}
