import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useAuth, User } from '../store/auth'
import { useT } from '../i18n'
import { authPage, authCard, authInput, authButton, authError, authLink } from './authStyles'

interface Props {
  onSwitchToLogin: () => void
}

export default function Register({ onSwitchToLogin }: Props) {
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
      const res = await api.post<{ token: string; user: User }>('/api/auth/register', { username, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('auth.register_failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main style={authPage}>
      <div style={authCard}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', marginBottom: 24 }}>
          <img src="/logo.svg" alt="dataseai" width={72} height={72} style={{ borderRadius: 16, marginBottom: 12 }} />
          <h1 style={{ margin: 0, fontSize: 26, fontWeight: 700, letterSpacing: -0.5 }}>dataseai</h1>
          <p style={{ margin: '4px 0 0', color: 'var(--text-muted)', fontSize: 13 }}>
            {t('auth.register_subtitle')}
          </p>
        </div>
        <form onSubmit={submit} style={{ display: 'grid', gap: 12 }}>
          <input
            placeholder={t('auth.register_username_placeholder')}
            value={username}
            onChange={(e) => setU(e.target.value)}
            required
            autoFocus
            style={authInput}
          />
          <input
            placeholder={t('auth.register_password_placeholder')}
            type="password"
            value={password}
            onChange={(e) => setP(e.target.value)}
            required
            style={authInput}
          />
          {error && <div style={authError}>{error}</div>}
          <button disabled={busy} type="submit" style={authButton}>
            {busy ? t('auth.creating') : t('auth.register_button')}
          </button>
        </form>
        <p style={{ marginTop: 20, fontSize: 13, textAlign: 'center', color: 'var(--text-muted)' }}>
          {t('auth.have_account')}{' '}
          <a
            href="#"
            onClick={(e) => {
              e.preventDefault()
              onSwitchToLogin()
            }}
            style={authLink}
          >
            {t('auth.login_link')}
          </a>
        </p>
      </div>
    </main>
  )
}
