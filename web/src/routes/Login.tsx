import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth, User } from '../store/auth'

interface Props {
  onSwitchToRegister: () => void
}

export default function Login({ onSwitchToRegister }: Props) {
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
      const res = await api.post<{ token: string; user: User }>('/api/auth/login', { username, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'login failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={{ maxWidth: 360, margin: '6rem auto', fontFamily: 'system-ui' }}>
      <h1 style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <img src="/logo.svg" alt="" width={36} height={36} style={{ borderRadius: 8 }} />
        dataseai · login
      </h1>
      <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
        <input
          placeholder="username"
          value={username}
          onChange={(e) => setU(e.target.value)}
          required
          autoFocus
        />
        <input
          placeholder="password"
          type="password"
          value={password}
          onChange={(e) => setP(e.target.value)}
          required
        />
        {error && <div style={{ color: 'crimson', fontSize: 14 }}>{error}</div>}
        <button disabled={busy} type="submit">
          {busy ? 'logging in...' : 'log in'}
        </button>
      </form>
      <p style={{ marginTop: 24, fontSize: 14 }}>
        No account?{' '}
        <a
          href="#"
          onClick={(e) => {
            e.preventDefault()
            onSwitchToRegister()
          }}
        >
          register
        </a>
      </p>
    </main>
  )
}
