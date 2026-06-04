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

interface ApiKeyState {
  set: boolean
  masked: string
}
type ApiKeysResp = {
  anthropic: ApiKeyState
  openai: ApiKeyState
  gemini: ApiKeyState
}

export default function Settings({ onClose }: Props) {
  const [oldPw, setOld] = useState('')
  const [newPw, setNew] = useState('')
  const [pwMsg, setPwMsg] = useState<string | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [keys, setKeys] = useState<ApiKeysResp | null>(null)
  const [keyDraft, setKeyDraft] = useState<{ anthropic: string; openai: string; gemini: string }>({ anthropic: '', openai: '', gemini: '' })
  const [keyMsg, setKeyMsg] = useState<string | null>(null)

  async function loadKeys() {
    try {
      const r = await api.get<ApiKeysResp>('/api/auth/api-keys')
      setKeys(r)
    } catch (err) {
      setKeyMsg(err instanceof ApiError ? err.message : 'failed to load keys')
    }
  }

  async function saveKey(provider: 'anthropic' | 'openai' | 'gemini', key: string) {
    setKeyMsg(null)
    try {
      await api.put('/api/auth/api-keys', { provider, key })
      setKeyMsg(`${provider} key ${key ? 'saved' : 'cleared'}`)
      setKeyDraft((d) => ({ ...d, [provider]: '' }))
      await loadKeys()
    } catch (err) {
      setKeyMsg(err instanceof ApiError ? err.message : 'save failed')
    }
  }

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
    void loadKeys()
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
    <main style={{
      fontFamily: 'system-ui', padding: 24, maxWidth: 720, margin: '0 auto',
      minHeight: '100vh', background: 'var(--bg-primary)', color: 'var(--text-primary)',
    }}>
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

      <section style={{ marginBottom: 32 }}>
        <h2>AI API keys</h2>
        <div style={{ fontSize: 13, color: 'var(--text-muted)', marginBottom: 12 }}>
          Your own LLM API keys take precedence over server defaults. Keys are stored encrypted.
        </div>
        {keyMsg && <div style={{ fontSize: 13, color: 'var(--accent)', marginBottom: 8 }}>{keyMsg}</div>}
        {keys && (['anthropic', 'openai', 'gemini'] as const).map((p) => {
          const k = keys[p]
          const label = p === 'anthropic' ? 'Anthropic Claude' : p === 'openai' ? 'OpenAI' : 'Google Gemini (free)'
          return (
            <div key={p} style={{ marginBottom: 14, padding: 12, border: '1px solid var(--border-color)', borderRadius: 6 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                <strong>{label}</strong>
                {k.set ? (
                  <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>set: <code>{k.masked}</code></span>
                ) : (
                  <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>not set — using server default if available</span>
                )}
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                <input
                  type="password"
                  placeholder={k.set ? 'enter new key to replace' : 'enter your API key'}
                  value={keyDraft[p]}
                  onChange={(e) => setKeyDraft((d) => ({ ...d, [p]: e.target.value }))}
                  style={{ flex: 1 }}
                />
                <button onClick={() => void saveKey(p, keyDraft[p])} disabled={!keyDraft[p]}>save</button>
                {k.set && <button onClick={() => void saveKey(p, '')}>clear</button>}
              </div>
            </div>
          )
        })}
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

const th: CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid var(--border-color)', fontSize: 13 }
const td: CSSProperties = { padding: '6px 8px', borderBottom: '1px solid var(--table-border)', fontSize: 13 }
