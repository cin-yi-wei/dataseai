import { FormEvent, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useT } from '../i18n'
import { authPage, authCard, authInput, authButton, authError, authLink } from './authStyles'

interface Props {
  onSwitchToLogin: () => void
}

// 忘記密碼（Phase 1）：輸入帳號 + 新密碼即無條件重設，不做身分驗證。
// Phase 2 將改為需 email 驗證碼。
export default function ForgotPassword({ onSwitchToLogin }: Props) {
  const t = useT()
  const [username, setU] = useState('')
  const [password, setP] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await api.post('/api/auth/forgot-password', { username, new: password })
      setDone(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('auth.forgot_failed'))
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
            {t('auth.forgot_subtitle')}
          </p>
        </div>
        {done ? (
          <>
            <div style={{ ...authError, background: 'var(--ok-bg, #113a1f)', color: 'var(--ok-text, #7ee2a8)' }}>
              {t('auth.forgot_success')}
            </div>
            <button type="button" onClick={onSwitchToLogin} style={{ ...authButton, marginTop: 12 }}>
              {t('auth.login_button')}
            </button>
          </>
        ) : (
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
              placeholder={t('auth.forgot_new_password_placeholder')}
              type="password"
              value={password}
              onChange={(e) => setP(e.target.value)}
              required
              style={authInput}
            />
            {error && <div style={authError}>{error}</div>}
            <button disabled={busy} type="submit" style={authButton}>
              {busy ? t('auth.resetting') : t('auth.forgot_button')}
            </button>
          </form>
        )}
        <p style={{ marginTop: 20, fontSize: 13, textAlign: 'center', color: 'var(--text-muted)' }}>
          <a
            href="#"
            onClick={(e) => {
              e.preventDefault()
              onSwitchToLogin()
            }}
            style={authLink}
          >
            {t('auth.back_to_login')}
          </a>
        </p>
      </div>
    </main>
  )
}
