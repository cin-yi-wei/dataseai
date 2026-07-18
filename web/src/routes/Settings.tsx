import { FormEvent, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { openExternal } from '../lib/openExternal'
import { useT } from '../i18n'
import WritesSection from '../components/WritesSection'
import AgentsSection from '../components/AgentsSection'

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
  claudecode: ApiKeyState
  codex: ApiKeyState
}

export default function Settings({ onClose }: Props) {
  const t = useT()
  const [oldPw, setOld] = useState('')
  const [newPw, setNew] = useState('')
  const [pwMsg, setPwMsg] = useState<string | null>(null)
  const [email, setEmail] = useState('')
  const [emailMsg, setEmailMsg] = useState<string | null>(null)
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [keys, setKeys] = useState<ApiKeysResp | null>(null)
  const [keyDraft, setKeyDraft] = useState<{ anthropic: string; openai: string; gemini: string; claudecode: string }>({ anthropic: '', openai: '', gemini: '', claudecode: '' })
  const [keyMsg, setKeyMsg] = useState<string | null>(null)

  async function loadKeys() {
    try {
      const r = await api.get<ApiKeysResp>('/api/auth/api-keys')
      setKeys(r)
    } catch (err) {
      setKeyMsg(err instanceof ApiError ? err.message : 'failed to load keys')
    }
  }

  async function saveKey(provider: 'anthropic' | 'openai' | 'gemini' | 'claudecode', key: string) {
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
    api.get<{ email: string }>('/api/auth/email').then((r) => setEmail(r.email)).catch(() => {})
  }, [])

  async function saveEmail(e: FormEvent) {
    e.preventDefault()
    setEmailMsg(null)
    try {
      await api.put('/api/auth/email', { email })
      setEmailMsg(t('settings.email_saved'))
    } catch (err) {
      setEmailMsg(err instanceof ApiError ? err.message : t('settings.email_save_failed'))
    }
  }

  async function changePassword(e: FormEvent) {
    e.preventDefault()
    setPwMsg(null)
    try {
      await api.put('/api/auth/password', { old: oldPw, new: newPw })
      setPwMsg(t('settings.password_changed'))
      setOld('')
      setNew('')
      await loadSessions()
    } catch (err) {
      setPwMsg(err instanceof ApiError ? err.message : t('settings.password_change_failed'))
    }
  }

  async function revoke(id: string) {
    try {
      await api.del(`/api/auth/sessions/${id}`)
      await loadSessions()
    } catch (err) {
      alert(err instanceof ApiError ? err.message : t('settings.revoke_failed'))
    }
  }

  return (
    <main style={{
      fontFamily: 'system-ui', padding: 24, maxWidth: 720, margin: '0 auto',
      minHeight: '100vh', background: 'var(--bg-primary)', color: 'var(--text-primary)',
    }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>{t('settings.title')}</h1>
        <button onClick={onClose}>{t('common.back')}</button>
      </header>

      {/* ── Group 1: Account ────────────────────────────── */}
      <GroupHeader title={t('settings.group.account')} desc={t('settings.group.account_desc')} />

      <section style={sectionStyle}>
        <h3 style={h3Style}>{t('settings.change_password')}</h3>
        <form onSubmit={changePassword} style={{ display: 'grid', gap: 8, maxWidth: 360 }}>
          <input type="password" placeholder={t('settings.current_password')} value={oldPw} onChange={(e) => setOld(e.target.value)} required />
          <input type="password" placeholder={t('settings.new_password')} value={newPw} onChange={(e) => setNew(e.target.value)} required />
          <button type="submit">{t('settings.change_button')}</button>
          {pwMsg && <div style={{ fontSize: 14 }}>{pwMsg}</div>}
        </form>
      </section>

      <section style={sectionStyle}>
        <h3 style={h3Style}>{t('settings.email_title')}</h3>
        <p style={{ fontSize: 13, color: 'var(--text-muted)', marginTop: 0 }}>{t('settings.email_desc')}</p>
        <form onSubmit={saveEmail} style={{ display: 'grid', gap: 8, maxWidth: 360 }}>
          <input type="email" placeholder={t('settings.email_placeholder')} value={email} onChange={(e) => setEmail(e.target.value)} />
          <button type="submit">{t('settings.email_save')}</button>
          {emailMsg && <div style={{ fontSize: 14 }}>{emailMsg}</div>}
        </form>
      </section>

      <section style={sectionStyle}>
        <h3 style={h3Style}>{t('settings.active_sessions')}</h3>
        {loadErr && <div style={{ color: 'crimson' }}>{loadErr}</div>}
        <SessionsTable sessions={sessions} onRevoke={revoke} />
      </section>

      {/* ── Group 2: Local agents ───────────────────────── */}
      <GroupHeader title={t('settings.group.agents')} desc={t('settings.group.agents_desc')} />
      <AgentsSection />

      {/* ── Group 3: AI providers ───────────────────────── */}
      <GroupHeader title={t('settings.group.ai_provider')} desc={t('settings.group.ai_provider_desc')} />

      <section style={sectionStyle}>
        <h3 style={h3Style}>{t('settings.api_keys_title')}</h3>
        <div style={{ fontSize: 13, color: 'var(--text-muted)', marginBottom: 12 }}>
          {t('settings.api_keys_hint')}
        </div>
        {keyMsg && <div style={{ fontSize: 13, color: 'var(--accent)', marginBottom: 8 }}>{keyMsg}</div>}
        {keys && (['anthropic', 'openai', 'gemini'] as const).map((p) => {
          const k = keys[p]
          const labelKey =
            p === 'anthropic' ? 'settings.provider_anthropic'
            : p === 'openai' ? 'settings.provider_openai'
            : 'settings.provider_gemini'
          return (
            <div key={p} style={{ marginBottom: 14, padding: 12, border: '1px solid var(--border-color)', borderRadius: 6 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                <strong>{t(labelKey)}</strong>
                {k.set ? (
                  <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{t('settings.key_set', { masked: k.masked })}</span>
                ) : (
                  <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{t('settings.key_not_set')}</span>
                )}
              </div>
              <div style={{ display: 'flex', gap: 6 }}>
                <input
                  type="password"
                  placeholder={k.set ? t('settings.key_placeholder_set') : t('settings.key_placeholder_unset')}
                  value={keyDraft[p]}
                  onChange={(e) => setKeyDraft((d) => ({ ...d, [p]: e.target.value }))}
                  style={{ flex: 1 }}
                />
                <button onClick={() => void saveKey(p, keyDraft[p])} disabled={!keyDraft[p]}>{t('settings.key_save')}</button>
                {k.set && <button onClick={() => void saveKey(p, '')}>{t('settings.key_clear')}</button>}
              </div>
            </div>
          )
        })}
        {keys && (
          <ClaudeCodeConnect
            keyState={keys.claudecode ?? { set: false, masked: '' }}
            onChanged={() => void loadKeys()}
          />
        )}
        {keys && (
          <CodexConnect
            keyState={keys.codex ?? { set: false, masked: '' }}
            onChanged={() => void loadKeys()}
          />
        )}
      </section>

      {/* ── Group 4: Write permissions ──────────────────── */}
      <GroupHeader title={t('settings.group.write_perms')} desc={t('settings.group.write_perms_desc')} />

      <WritesSection
        scope="ai"
        title={t('settings.ai_writes.title')}
        masterLabel={t('settings.ai_writes.master_label')}
        masterHintOff={t('settings.ai_writes.master_hint_off')}
        showAudit
      />

      <WritesSection
        scope="dml"
        title={t('settings.dml_writes.title')}
        masterLabel={t('settings.dml_writes.master_label')}
        masterHintOff={t('settings.dml_writes.master_hint_off')}
        showAudit
      />
    </main>
  )
}

// summarizeUA reduces a raw User-Agent string to a short human-readable
// label like "Chrome 148 · Android" so the device column doesn't blow up
// the row height on mobile. Falls back to the raw UA when no pattern matches.
function summarizeUA(ua: string): string {
  if (!ua) return '?'
  // OS
  let os = 'Unknown'
  if (/iPhone|iPad|iOS/.test(ua)) os = 'iOS'
  else if (/Android/.test(ua)) os = 'Android'
  else if (/Mac OS X/.test(ua)) os = 'macOS'
  else if (/Windows/.test(ua)) os = 'Windows'
  else if (/Linux/.test(ua)) os = 'Linux'
  // Browser + version (check Edge/OPR before Chrome — Chrome appears in their UA too)
  let browser = 'Browser'
  const matchers: [RegExp, string][] = [
    [/Edg\/([\d.]+)/, 'Edge'],
    [/OPR\/([\d.]+)/, 'Opera'],
    [/Chrome\/([\d.]+)/, 'Chrome'],
    [/Firefox\/([\d.]+)/, 'Firefox'],
    [/Version\/([\d.]+).*Safari/, 'Safari'],
  ]
  for (const [re, name] of matchers) {
    const m = ua.match(re)
    if (m) {
      const major = m[1].split('.')[0]
      browser = `${name} ${major}`
      break
    }
  }
  return `${browser} · ${os}`
}

// SessionsTable shows the user's active sessions, 3 per page. The current
// session is pinned to the top so the user can always see (and never miss)
// the device they're on right now.
function SessionsTable({ sessions, onRevoke }: { sessions: SessionRow[]; onRevoke: (id: string) => void }) {
  const t = useT()
  const PER_PAGE = 3
  const [page, setPage] = useState(0)
  // Pin current session at index 0; show rest in descending last_used order.
  const sorted = [
    ...sessions.filter((s) => s.current),
    ...sessions.filter((s) => !s.current)
      .sort((a, b) => new Date(b.last_used_at).getTime() - new Date(a.last_used_at).getTime()),
  ]
  const totalPages = Math.max(1, Math.ceil(sorted.length / PER_PAGE))
  const pageRows = sorted.slice(page * PER_PAGE, page * PER_PAGE + PER_PAGE)
  return (
    <>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {pageRows.map((s) => (
          <div key={s.id} style={sessionCard}>
            <div style={sessionRowTop}>
              <span style={{ fontWeight: 600, fontSize: 13 }} title={s.user_agent}>{summarizeUA(s.user_agent)}</span>
              {s.current && <span style={{ fontSize: 11, color: '#3a8' }}>{t('settings.session_current')}</span>}
              {!s.current && (
                <button onClick={() => onRevoke(s.id)} style={{ marginLeft: 'auto', fontSize: 12 }}>
                  {t('settings.revoke')}
                </button>
              )}
            </div>
            <div style={sessionMeta}>
              <span title={t('settings.column_id')}>#{s.id.slice(0, 8)}</span>
              <span title={t('settings.column_last_used')}>
                {t('settings.column_last_used')}: {new Date(s.last_used_at).toLocaleString()}
              </span>
              <span title={t('settings.column_expires')}>
                {t('settings.column_expires')}: {new Date(s.expires_at).toLocaleString()}
              </span>
            </div>
          </div>
        ))}
      </div>
      {totalPages > 1 && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 8, fontSize: 12, color: 'var(--text-muted)' }}>
          <button onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page === 0} style={{ fontSize: 12 }}>‹</button>
          <span>{page + 1} / {totalPages}</span>
          <button onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))} disabled={page >= totalPages - 1} style={{ fontSize: 12 }}>›</button>
          <span style={{ marginLeft: 8 }}>{t('settings.sessions_total', { n: sorted.length })}</span>
        </div>
      )}
    </>
  )
}

const sessionCard: CSSProperties = {
  border: '1px solid var(--border-color)', borderRadius: 4,
  padding: '8px 10px', display: 'flex', flexDirection: 'column', gap: 4,
}
const sessionRowTop: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap',
}
const sessionMeta: CSSProperties = {
  display: 'flex', gap: 12, flexWrap: 'wrap',
  fontSize: 11, color: 'var(--text-muted)',
}

// GroupHeader is the primary visual landmark — large, accent-tinted,
// thick underline. It clearly outranks the subsection h3 headers below.
function GroupHeader({ title, desc }: { title: string; desc: string }) {
  return (
    <div style={{
      marginTop: 40, marginBottom: 16,
      paddingBottom: 10, borderBottom: '3px solid var(--accent, #4a8fd5)',
    }}>
      <h2 style={{
        margin: 0, fontSize: 22, fontWeight: 700,
        color: 'var(--accent, #4a8fd5)',
        letterSpacing: 0.3,
      }}>{title}</h2>
      <div style={{ marginTop: 4, fontSize: 13, color: 'var(--text-muted)' }}>{desc}</div>
    </div>
  )
}

// Subsection container — indented, light card background, thin border on
// the left so it visually sits "under" the GroupHeader above.
const sectionStyle: CSSProperties = {
  marginBottom: 16, marginLeft: 12,
  padding: '12px 14px',
  background: 'var(--bg-elevated, transparent)',
  border: '1px solid var(--border-color)',
  borderLeft: '3px solid var(--border-strong, var(--border-color))',
  borderRadius: 4,
}
const h3Style: CSSProperties = {
  fontSize: 14, margin: '0 0 10px',
  fontWeight: 600,
  color: 'var(--text-secondary, var(--text-primary))',
  textTransform: 'uppercase',
  letterSpacing: 0.4,
}

interface ClaudeCodeConnectProps {
  keyState: ApiKeyState
  onChanged: () => void
}

// useClipboardAutoFill watches for window focus events and, when a pending
// OAuth flow is active, peeks at the clipboard for a localhost callback URL
// matching the given prefix. If found, the URL is pasted into the input
// automatically so the user only has to click Submit.
function useClipboardAutoFill(pending: boolean, prefix: string, setCode: (s: string) => void) {
  useEffect(() => {
    if (!pending) return
    const tryRead = async () => {
      try {
        const txt = await navigator.clipboard.readText()
        const trimmed = txt.trim()
        if (trimmed.startsWith(prefix)) {
          setCode(trimmed)
        }
      } catch {
        // clipboard read denied / unavailable — fall back to manual paste
      }
    }
    window.addEventListener('focus', tryRead)
    // Also try once immediately in case the user comes back instantly.
    void tryRead()
    return () => window.removeEventListener('focus', tryRead)
  }, [pending, prefix, setCode])
}

function ClaudeCodeConnect({ keyState, onChanged }: ClaudeCodeConnectProps) {
  const t = useT()
  const [pendingState, setPending] = useState<{ verifier: string; state: string } | null>(null)
  const [code, setCode] = useState('')
  const [msg, setMsg] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  useClipboardAutoFill(!!pendingState, 'https://platform.claude.com/oauth/code/callback', setCode)

  async function startConnect() {
    setMsg(null)
    setBusy(true)
    try {
      const r = await api.post<{ auth_url: string; verifier: string; state: string }>('/api/auth/claudecode/start', {})
      setPending({ verifier: r.verifier, state: r.state })
      openExternal(r.auth_url)
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : 'start failed')
    } finally {
      setBusy(false)
    }
  }

  async function submitCode() {
    if (!pendingState) return
    setMsg(null)
    setBusy(true)
    try {
      await api.post('/api/auth/claudecode/exchange', {
        code: code.trim(),
        verifier: pendingState.verifier,
        state: pendingState.state,
      })
      setMsg(t('settings.claudecode_connected'))
      setCode('')
      setPending(null)
      onChanged()
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : 'exchange failed')
    } finally {
      setBusy(false)
    }
  }

  async function disconnect() {
    setBusy(true)
    try {
      await api.post('/api/auth/claudecode/disconnect', {})
      setMsg(t('settings.claudecode_disconnected'))
      onChanged()
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : 'disconnect failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div style={{ marginBottom: 14, padding: 12, border: '1px solid var(--border-color)', borderRadius: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <strong>{t('settings.provider_claudecode')}</strong>
        {keyState.set ? (
          <span style={{ fontSize: 12, color: '#3a8' }}>{t('settings.claudecode_status_connected')}</span>
        ) : (
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{t('settings.key_not_set')}</span>
        )}
      </div>
      <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 8, lineHeight: 1.5 }}>
        {t('settings.claudecode_oauth_hint')}
      </div>
      {!pendingState && !keyState.set && (
        <button onClick={() => void startConnect()} disabled={busy}>
          {t('settings.claudecode_connect_button')}
        </button>
      )}
      {pendingState && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>{t('settings.claudecode_paste_code_hint')}</div>
          <div style={{ display: 'flex', gap: 6 }}>
            <input
              autoFocus
              placeholder="paste the code from the callback page"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              style={{ flex: 1 }}
            />
            <button onClick={() => void submitCode()} disabled={!code.trim() || busy}>
              {t('settings.claudecode_submit_code')}
            </button>
            <button onClick={() => { setPending(null); setCode('') }} disabled={busy}>
              {t('common.cancel')}
            </button>
          </div>
        </div>
      )}
      {keyState.set && !pendingState && (
        <div style={{ display: 'flex', gap: 6 }}>
          <button onClick={() => void startConnect()} disabled={busy}>
            {t('settings.claudecode_reconnect_button')}
          </button>
          <button onClick={() => void disconnect()} disabled={busy}>
            {t('settings.claudecode_disconnect_button')}
          </button>
        </div>
      )}
      {msg && <div style={{ marginTop: 8, fontSize: 12, color: 'var(--accent)' }}>{msg}</div>}
    </div>
  )
}

function CodexConnect({ keyState, onChanged }: ClaudeCodeConnectProps) {
  const t = useT()
  const [pending, setPending] = useState<{ verifier: string; state: string } | null>(null)
  const [code, setCode] = useState('')
  const [msg, setMsg] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  useClipboardAutoFill(!!pending, 'http://localhost:1455/auth/callback', setCode)

  async function startConnect() {
    setMsg(null); setBusy(true)
    try {
      const r = await api.post<{ auth_url: string; verifier: string; state: string }>('/api/auth/codex/start', {})
      setPending({ verifier: r.verifier, state: r.state })
      openExternal(r.auth_url)
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : 'start failed')
    } finally { setBusy(false) }
  }

  async function submitCode() {
    if (!pending) return
    setMsg(null); setBusy(true)
    try {
      await api.post('/api/auth/codex/exchange', { code: code.trim(), verifier: pending.verifier, state: pending.state })
      setMsg(t('settings.codex_connected'))
      setCode(''); setPending(null)
      onChanged()
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : 'exchange failed')
    } finally { setBusy(false) }
  }

  async function disconnect() {
    setBusy(true)
    try {
      await api.post('/api/auth/codex/disconnect', {})
      setMsg(t('settings.codex_disconnected'))
      onChanged()
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : 'disconnect failed')
    } finally { setBusy(false) }
  }

  return (
    <div style={{ marginBottom: 14, padding: 12, border: '1px solid var(--border-color)', borderRadius: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <strong>{t('settings.provider_codex')}</strong>
        {keyState.set ? (
          <span style={{ fontSize: 12, color: '#3a8' }}>{t('settings.codex_status_connected')}</span>
        ) : (
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{t('settings.key_not_set')}</span>
        )}
      </div>
      <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 8, lineHeight: 1.5 }}>
        {t('settings.codex_oauth_hint')}
      </div>
      {!pending && !keyState.set && (
        <button onClick={() => void startConnect()} disabled={busy}>{t('settings.codex_connect_button')}</button>
      )}
      {pending && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>{t('settings.codex_paste_code_hint')}</div>
          <div style={{ display: 'flex', gap: 6 }}>
            <input autoFocus placeholder="paste the callback URL" value={code} onChange={(e) => setCode(e.target.value)} style={{ flex: 1 }} />
            <button onClick={() => void submitCode()} disabled={!code.trim() || busy}>{t('settings.codex_submit_code')}</button>
            <button onClick={() => { setPending(null); setCode('') }} disabled={busy}>{t('common.cancel')}</button>
          </div>
        </div>
      )}
      {keyState.set && !pending && (
        <div style={{ display: 'flex', gap: 6 }}>
          <button onClick={() => void startConnect()} disabled={busy}>{t('settings.codex_reconnect_button')}</button>
          <button onClick={() => void disconnect()} disabled={busy}>{t('settings.codex_disconnect_button')}</button>
        </div>
      )}
      {msg && <div style={{ marginTop: 8, fontSize: 12, color: 'var(--accent)' }}>{msg}</div>}
    </div>
  )
}
