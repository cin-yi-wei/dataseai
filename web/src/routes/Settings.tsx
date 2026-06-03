import { FormEvent, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'

interface SessionRow {
  id: string
  user_agent: string
  created_at: string
  last_used_at: string
  expires_at: string
  current: boolean
}

interface Props {
  onClose: () => void
}

export default function Settings({ onClose }: Props) {
  const [oldPw, setOld] = useState('')
  const [newPw, setNew] = useState('')
  const [pwMsg, setPwMsg] = useState<string | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)

  async function loadSessions() {
    try {
      const r = await api.get<{ sessions: SessionRow[] }>('/api/auth/sessions')
      setSessions(r.sessions)
    } catch (err) {
      setLoadErr(err instanceof ApiError ? err.message : 'failed to load sessions')
    }
  }

  useEffect(() => {
    void loadSessions()
  }, [])

  async function changePassword(e: FormEvent) {
    e.preventDefault()
    setPwMsg(null)
    try {
      await api.put('/api/auth/password', { old: oldPw, new: newPw })
      setPwMsg('password changed (other sessions were revoked)')
      setOld('')
      setNew('')
      await loadSessions()
    } catch (err) {
      setPwMsg(err instanceof ApiError ? err.message : 'change failed')
    }
  }

  async function revoke(id: string) {
    try {
      await api.del(`/api/auth/sessions/${id}`)
      await loadSessions()
    } catch (err) {
      alert(err instanceof ApiError ? err.message : 'revoke failed')
    }
  }

  return (
    <main style={{ fontFamily: 'system-ui', padding: 24, maxWidth: 720, margin: '0 auto' }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>settings</h1>
        <button onClick={onClose}>back</button>
      </header>

      <section style={{ marginBottom: 32 }}>
        <h2>change password</h2>
        <form onSubmit={changePassword} style={{ display: 'grid', gap: 8, maxWidth: 360 }}>
          <input type="password" placeholder="current password" value={oldPw} onChange={(e) => setOld(e.target.value)} required />
          <input type="password" placeholder="new password" value={newPw} onChange={(e) => setNew(e.target.value)} required />
          <button type="submit">change</button>
          {pwMsg && <div style={{ fontSize: 14 }}>{pwMsg}</div>}
        </form>
      </section>

      <section>
        <h2>active sessions</h2>
        {loadErr && <div style={{ color: 'crimson' }}>{loadErr}</div>}
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={th}>id</th>
              <th style={th}>device</th>
              <th style={th}>last used</th>
              <th style={th}>expires</th>
              <th style={th}></th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={s.id}>
                <td style={td}>
                  {s.id}
                  {s.current && <span style={{ marginLeft: 6, fontSize: 11, color: 'green' }}>(this)</span>}
                </td>
                <td style={td}>{s.user_agent}</td>
                <td style={td}>{new Date(s.last_used_at).toLocaleString()}</td>
                <td style={td}>{new Date(s.expires_at).toLocaleString()}</td>
                <td style={td}>
                  {!s.current && <button onClick={() => revoke(s.id)}>revoke</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  )
}

const th: CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid #ddd', fontSize: 13 }
const td: CSSProperties = { padding: '6px 8px', borderBottom: '1px solid #f3f3f3', fontSize: 13 }
