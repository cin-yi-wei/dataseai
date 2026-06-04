import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth } from '../store/auth'

interface AdminStats {
  total_users: number
  total_admins: number
  total_connections: number
  total_sessions: number
  total_queries: number
}

interface UserInfo {
  id: number
  username: string
  is_admin: boolean
  created_at: string
  conn_count: number
  session_count: number
  last_seen_at: string | null
}

interface ConnectionInfo {
  id: number
  user_id: number
  username: string
  name: string
  host: string
  port: number
  db_username: string
  default_db: string
  tls: string
  created_at: string
}

type Tab = 'stats' | 'users' | 'connections'

interface Props {
  onClose: () => void
}

export default function AdminPage({ onClose }: Props) {
  const me = useAuth((s) => s.user)
  const [tab, setTab] = useState<Tab>('stats')
  const [stats, setStats] = useState<AdminStats | null>(null)
  const [users, setUsers] = useState<UserInfo[]>([])
  const [connections, setConnections] = useState<ConnectionInfo[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const loadStats = async () => {
    try {
      const s = await api.get<AdminStats>('/api/admin/stats')
      setStats(s)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'load failed')
    }
  }

  const loadUsers = async () => {
    try {
      const r = await api.get<{ users: UserInfo[] }>('/api/admin/users')
      setUsers(r.users ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'load failed')
    }
  }

  const loadConnections = async () => {
    try {
      const r = await api.get<{ connections: ConnectionInfo[] }>('/api/admin/connections')
      setConnections(r.connections ?? [])
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'load failed')
    }
  }

  useEffect(() => {
    setLoading(true)
    setError(null)
    const promises: Promise<any>[] = [loadStats()]
    if (tab === 'users') promises.push(loadUsers())
    if (tab === 'connections') promises.push(loadConnections())
    Promise.all(promises).finally(() => setLoading(false))
  }, [tab])

  const handleDeleteUser = async (id: number, username: string) => {
    if (!window.confirm(`Delete user "${username}"? This deletes all their connections, sessions, and history.`)) return
    try {
      await api.del(`/api/admin/users/${id}`)
      loadUsers()
      loadStats()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'delete failed')
    }
  }

  const handleToggleAdmin = async (id: number, current: boolean) => {
    try {
      await api.patch(`/api/admin/users/${id}/admin`, { is_admin: !current })
      loadUsers()
      loadStats()
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : 'update failed')
    }
  }

  return (
    <div style={container}>
      <header style={header}>
        <h1 style={{ margin: 0, fontSize: 20 }}>⚙️ Admin Panel</h1>
        <div style={{ flex: 1 }} />
        <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>{me?.username} (admin)</span>
        <button onClick={onClose}>← Back to workspace</button>
      </header>

      <div style={tabBar}>
        <button
          onClick={() => setTab('stats')}
          style={{ ...tabBtn, ...(tab === 'stats' ? tabBtnActive : {}) }}
        >
          📊 Stats
        </button>
        <button
          onClick={() => setTab('users')}
          style={{ ...tabBtn, ...(tab === 'users' ? tabBtnActive : {}) }}
        >
          👥 Users {stats && `(${stats.total_users})`}
        </button>
        <button
          onClick={() => setTab('connections')}
          style={{ ...tabBtn, ...(tab === 'connections' ? tabBtnActive : {}) }}
        >
          🔌 Connections {stats && `(${stats.total_connections})`}
        </button>
      </div>

      <main style={content}>
        {error && <div style={{ color: 'var(--danger)', marginBottom: 12 }}>{error}</div>}
        {loading && <div style={{ color: 'var(--text-muted)' }}>Loading…</div>}

        {tab === 'stats' && stats && (
          <div style={statsGrid}>
            <StatCard label="Total Users" value={stats.total_users} icon="👥" />
            <StatCard label="Admins" value={stats.total_admins} icon="⚙️" />
            <StatCard label="DB Connections" value={stats.total_connections} icon="🔌" />
            <StatCard label="Active Sessions" value={stats.total_sessions} icon="🔑" />
            <StatCard label="Queries Run" value={stats.total_queries} icon="📜" />
          </div>
        )}

        {tab === 'users' && (
          <table style={table}>
            <thead>
              <tr>
                <th style={th}>ID</th>
                <th style={th}>Username</th>
                <th style={th}>Admin</th>
                <th style={th}>Connections</th>
                <th style={th}>Sessions</th>
                <th style={th}>Last Seen</th>
                <th style={th}>Created At</th>
                <th style={th}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td style={td}>{u.id}</td>
                  <td style={td}>{u.username}</td>
                  <td style={td}>
                    <input
                      type="checkbox"
                      checked={u.is_admin}
                      onChange={() => handleToggleAdmin(u.id, u.is_admin)}
                    />
                  </td>
                  <td style={td}>{u.conn_count}</td>
                  <td style={td}>{u.session_count}</td>
                  <td style={td}>{u.last_seen_at ? formatRelativeTime(u.last_seen_at) : <span style={{ color: 'var(--text-muted)' }}>never</span>}</td>
                  <td style={td}>{u.created_at}</td>
                  <td style={td}>
                    <button
                      onClick={() => handleDeleteUser(u.id, u.username)}
                      disabled={u.id === me?.id}
                      style={{ color: 'var(--danger)' }}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {tab === 'connections' && (
          <table style={table}>
            <thead>
              <tr>
                <th style={th}>ID</th>
                <th style={th}>Owner</th>
                <th style={th}>Name</th>
                <th style={th}>Host:Port</th>
                <th style={th}>DB User</th>
                <th style={th}>Default DB</th>
                <th style={th}>TLS</th>
                <th style={th}>Created</th>
              </tr>
            </thead>
            <tbody>
              {connections.map((c) => (
                <tr key={c.id}>
                  <td style={td}>{c.id}</td>
                  <td style={td}>{c.username}</td>
                  <td style={td}>{c.name}</td>
                  <td style={td}>{c.host}:{c.port}</td>
                  <td style={td}>{c.db_username}</td>
                  <td style={td}>{c.default_db || '—'}</td>
                  <td style={td}>{c.tls}</td>
                  <td style={td}>{c.created_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </main>
    </div>
  )
}

function formatRelativeTime(iso: string): string {
  const date = new Date(iso)
  if (isNaN(date.getTime())) return iso
  const diff = Date.now() - date.getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return date.toLocaleDateString()
}

function StatCard({ label, value, icon }: { label: string; value: number; icon: string }) {
  return (
    <div style={statCard}>
      <div style={{ fontSize: 36 }}>{icon}</div>
      <div style={{ fontSize: 32, fontWeight: 700, color: 'var(--accent)' }}>{value}</div>
      <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>{label}</div>
    </div>
  )
}

const container: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100vh',
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
const header: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 12,
  padding: '12px 20px', borderBottom: '1px solid var(--border-color)',
  background: 'var(--bg-secondary)',
}
const tabBar: CSSProperties = {
  display: 'flex', gap: 4, padding: '8px 20px',
  borderBottom: '1px solid var(--border-color)', background: 'var(--bg-tertiary)',
}
const tabBtn: CSSProperties = {
  padding: '6px 14px', fontSize: 13,
  background: 'transparent', border: '1px solid transparent', borderRadius: 4,
  cursor: 'pointer', color: 'var(--text-secondary)',
}
const tabBtnActive: CSSProperties = {
  background: 'var(--bg-elevated)', borderColor: 'var(--border-strong)',
  color: 'var(--text-primary)',
}
const content: CSSProperties = {
  flex: 1, padding: 20, overflowY: 'auto',
}
const statsGrid: CSSProperties = {
  display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
  gap: 16,
}
const statCard: CSSProperties = {
  display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8,
  padding: 24, background: 'var(--bg-elevated)', borderRadius: 8,
  border: '1px solid var(--border-color)',
}
const table: CSSProperties = {
  width: '100%', borderCollapse: 'collapse', fontSize: 13,
}
const th: CSSProperties = {
  textAlign: 'left', padding: '8px 12px',
  borderBottom: '1px solid var(--border-color)',
  background: 'var(--table-header-bg)',
}
const td: CSSProperties = {
  padding: '8px 12px', borderBottom: '1px solid var(--table-border)',
}
