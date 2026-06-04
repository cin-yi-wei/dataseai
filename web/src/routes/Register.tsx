import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth, User } from '../store/auth'

interface Props {
  onSwitchToLogin: () => void
}

export default function Register({ onSwitchToLogin }: Props) {
  const login = useAuth((s) => s.login)
  const [username, setU] = useState('')
  const [password, setP] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const res = await api.post<{ token: string; user: User }>('/api/auth/register', { username, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'register failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={{ maxWidth: 360, margin: '6rem auto', fontFamily: 'system-ui' }}>
      <h1>dataseai · register</h1>
      <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
        <input placeholder="username (3-32 chars)" value={username} onChange={(e) => setU(e.target.value)} required autoFocus />
        <input
          placeholder="password (≥8 chars, letters+digits)"
          type="password"
          value={password}
          onChange={(e) => setP(e.target.value)}
          required
        />
        {error && <div style={{ color: 'crimson', fontSize: 14 }}>{error}</div>}
        <button disabled={busy} type="submit">
          {busy ? 'creating...' : 'create account'}
        </button>
      </form>
      <p style={{ marginTop: 24, fontSize: 14 }}>
        Already have an account?{' '}
        <a
          href="#"
          onClick={(e) => {
            e.preventDefault()
            onSwitchToLogin()
          }}
        >
          log in
        </a>
      </p>
    </main>
  )
}
