import { useAuth } from '../store/auth'

interface Props {
  onOpenSettings: () => void
}

export default function Workspace({ onOpenSettings }: Props) {
  const user = useAuth((s) => s.user!)
  const logout = useAuth((s) => s.logout)
  return (
    <main style={{ fontFamily: 'system-ui', padding: 24 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>mysqlweb</h1>
        <nav style={{ display: 'flex', gap: 12 }}>
          <span>logged in as <b>{user.username}</b></span>
          <button onClick={onOpenSettings}>settings</button>
          <button onClick={() => logout()}>log out</button>
        </nav>
      </header>
      <section
        style={{
          border: '1px dashed #999',
          padding: 32,
          borderRadius: 8,
          color: '#666',
          textAlign: 'center',
        }}
      >
        connections sidebar + workspace coming in Plan 2
      </section>
    </main>
  )
}
