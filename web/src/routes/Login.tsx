import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth, User } from '../store/auth'
import { useT } from '../i18n'
import { authPage, authCard, authInput, authButton, authError, authLink } from './authStyles'

interface Props {
  onSwitchToRegister: () => void
}

export default function Login({ onSwitchToRegister }: Props) {
  const login = useAuth((s) => s.login)
  const t = useT()
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
      setError(err instanceof ApiError ? err.message : t('auth.login_failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={authPage}>
      <div style={authCard}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', marginBottom: 24 }}>
          <img src="/logo.svg" alt="DataseAI" width={72} height={72} style={{ borderRadius: 16, marginBottom: 12 }} />
          <h1 style={{ margin: 0, fontSize: 26, fontWeight: 700, letterSpacing: -0.5 }}>DataseAI</h1>
          <p style={{ margin: '4px 0 0', color: 'var(--text-muted)', fontSize: 13 }}>
            {t('auth.login_subtitle')}
          </p>
        </div>
        <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
          <input
            placeholder={t('auth.username_placeholder')}
            value={username}
            onChange={(e) => setU(e.target.value)}
            required
            autoFocus
            style={authInput}
          />
          <input
            placeholder={t('auth.password_placeholder')}
            type="password"
            value={password}
            onChange={(e) => setP(e.target.value)}
            required
            style={authInput}
          />
          {error && <div style={authError}>{error}</div>}
          <button disabled={busy} type="submit" style={authButton}>
            {busy ? t('auth.logging_in') : t('auth.login_button')}
          </button>
        </form>
        <p style={{ marginTop: 20, fontSize: 13, textAlign: 'center', color: 'var(--text-muted)' }}>
          {t('auth.no_account')}{' '}
          <a
            href="#"
            onClick={(e) => {
              e.preventDefault()
              onSwitchToRegister()
            }}
            style={authLink}
          >
            {t('auth.register_link')}
          </a>
        </p>
      </div>
    </main>
  )
}
