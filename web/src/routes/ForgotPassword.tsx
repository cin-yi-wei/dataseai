import { FormEvent, useEffect, useState } from 'react'
import { api, ApiError } from '../lib/api'
import { useT } from '../i18n'
import { authPage, authCard, authInput, authButton, authError, authLink } from './authStyles'

interface Props {
  onSwitchToLogin: () => void
}

// Two flows, chosen by the server's /api/auth/config:
//  - email_reset=true  : step 1 request a code by email, step 2 enter code + new password.
//  - email_reset=false : unconditional local reset (single-user desktop GUI).
export default function ForgotPassword({ onSwitchToLogin }: Props) {
  const t = useT()
  const [emailReset, setEmailReset] = useState<boolean | null>(null)
  const [username, setU] = useState('')
  const [code, setCode] = useState('')
  const [password, setP] = useState('')
  const [stage, setStage] = useState<'request' | 'verify'>('request')
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.get<{ email_reset?: boolean }>('/api/auth/config')
      .then((c) => setEmailReset(!!c.email_reset))
      .catch(() => setEmailReset(false))
  }, [])

  // Unconditional (GUI) flow: username + new password → done.
  async function submitUnconditional(e: FormEvent) {
    e.preventDefault()
    setError(null); setBusy(true)
    try {
      await api.post('/api/auth/forgot-password', { username, new: password })
      setDone(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('auth.forgot_failed'))
    } finally { setBusy(false) }
  }

  // Email flow step 1: request a code.
  async function submitRequest(e: FormEvent) {
    e.preventDefault()
    setError(null); setBusy(true)
    try {
      await api.post('/api/auth/forgot-password', { username })
      setNotice(t('auth.forgot_code_sent'))
      setStage('verify')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('auth.forgot_failed'))
    } finally { setBusy(false) }
  }

  // Email flow step 2: verify code + set new password.
  async function submitVerify(e: FormEvent) {
    e.preventDefault()
    setError(null); setBusy(true)
    try {
      await api.post('/api/auth/reset-password', { username, code, new: password })
      setDone(true)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('auth.forgot_failed'))
    } finally { setBusy(false) }
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
        ) : emailReset === null ? (
          <div style={{ color: 'var(--text-muted)', fontSize: 13, textAlign: 'center' }}>{t('common.loading')}</div>
        ) : emailReset ? (
          stage === 'request' ? (
            <form onSubmit={submitRequest} style={{ display: 'grid', gap: 12 }}>
              <input
                placeholder={t('auth.forgot_identifier_placeholder')}
                value={username}
                onChange={(e) => setU(e.target.value)}
                required autoFocus style={authInput}
              />
              {error && <div style={authError}>{error}</div>}
              <button disabled={busy} type="submit" style={authButton}>
                {busy ? t('auth.resetting') : t('auth.forgot_send_code')}
              </button>
            </form>
          ) : (
            <form onSubmit={submitVerify} style={{ display: 'grid', gap: 12 }}>
              {notice && (
                <div style={{ ...authError, background: 'var(--ok-bg, #113a1f)', color: 'var(--ok-text, #7ee2a8)' }}>
                  {notice}
                </div>
              )}
              <input
                placeholder={t('auth.forgot_code_placeholder')}
                value={code}
                onChange={(e) => setCode(e.target.value)}
                required autoFocus inputMode="numeric" style={authInput}
              />
              <input
                placeholder={t('auth.forgot_new_password_placeholder')}
                type="password"
                value={password}
                onChange={(e) => setP(e.target.value)}
                required style={authInput}
              />
              {error && <div style={authError}>{error}</div>}
              <button disabled={busy} type="submit" style={authButton}>
                {busy ? t('auth.resetting') : t('auth.forgot_button')}
              </button>
            </form>
          )
        ) : (
          <form onSubmit={submitUnconditional} style={{ display: 'grid', gap: 12 }}>
            <input
              placeholder={t('auth.username_placeholder')}
              value={username}
              onChange={(e) => setU(e.target.value)}
              required autoFocus style={authInput}
            />
            <input
              placeholder={t('auth.forgot_new_password_placeholder')}
              type="password"
              value={password}
              onChange={(e) => setP(e.target.value)}
              required style={authInput}
            />
            {error && <div style={authError}>{error}</div>}
            <button disabled={busy} type="submit" style={authButton}>
              {busy ? t('auth.resetting') : t('auth.forgot_button')}
            </button>
          </form>
        )}

        <p style={{ marginTop: 20, fontSize: 13, textAlign: 'center', color: 'var(--text-muted)' }}>
          <a href="#" onClick={(e) => { e.preventDefault(); onSwitchToLogin() }} style={authLink}>
            {t('auth.back_to_login')}
          </a>
        </p>
      </div>
    </main>
  )
}
