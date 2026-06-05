import { FormEvent, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api, ApiError } from '../lib/api'
import { useT } from '../i18n'
import AIWritePolicyTable, { type AIPolicy, type TablePolicy } from '../components/AIWritePolicyTable'
import AIWriteAuditList, { type AuditRow } from '../components/AIWriteAuditList'

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
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [keys, setKeys] = useState<ApiKeysResp | null>(null)
  const [keyDraft, setKeyDraft] = useState<{ anthropic: string; openai: string; gemini: string; claudecode: string }>({ anthropic: '', openai: '', gemini: '', claudecode: '' })
  const [keyMsg, setKeyMsg] = useState<string | null>(null)

  // AI Writes state
  const [aiEnabled, setAiEnabled] = useState(false)
  const [audit, setAudit] = useState<AuditRow[]>([])
  const [connections, setConnections] = useState<{ id: number; name: string }[]>([])
  const [selectedConn, setSelectedConn] = useState<number | null>(null)
  const [databases, setDatabases] = useState<string[]>([])
  const [selectedDb, setSelectedDb] = useState<string | null>(null)
  const [policy, setPolicy] = useState<{ configured: TablePolicy[]; unconfigured: string[] }>(
    { configured: [], unconfigured: [] }
  )

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
  }, [])

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

  // AI Writes functions
  async function loadMaster() {
    try {
      const r = await api.get<{ enabled: boolean }>('/api/auth/ai-writes')
      setAiEnabled(r.enabled)
    } catch {/* leave default */}
  }
  async function toggleMaster(v: boolean) {
    await api.put('/api/auth/ai-writes', { enabled: v })
    setAiEnabled(v)
    if (v) {
      await loadConnections()
      await loadAudit()
    } else {
      setSelectedConn(null)
      setSelectedDb(null)
      setPolicy({ configured: [], unconfigured: [] })
      setAudit([])
    }
  }
  async function loadConnections() {
    const r = await api.get<{ connections: { id: number; name: string }[] }>('/api/connections')
    setConnections(r.connections ?? [])
  }
  async function loadDatabases(connId: number) {
    const r = await api.get<{ databases: string[] }>(`/api/db/${connId}/databases`)
    setDatabases(r.databases ?? [])
  }
  async function loadPolicy(connId: number, db: string) {
    const r = await api.get<typeof policy>(`/api/auth/ai-policy?conn=${connId}&db=${encodeURIComponent(db)}`)
    setPolicy({ configured: r.configured ?? [], unconfigured: r.unconfigured ?? [] })
  }
  async function loadAudit() {
    const rows = await api.get<AuditRow[]>('/api/auth/ai-audit?limit=50')
    setAudit(rows ?? [])
  }
  async function upsertPolicy(connId: number, db: string, table: string, p: AIPolicy) {
    await api.put('/api/auth/ai-policy', { conn: connId, db, table, policy: p })
    await loadPolicy(connId, db)
  }
  async function batchPolicy(connId: number, db: string, tables: string[], p: AIPolicy) {
    await api.put('/api/auth/ai-policy/batch', { conn: connId, db, tables, policy: p })
    await loadPolicy(connId, db)
  }

  useEffect(() => { void loadMaster() }, [])
  useEffect(() => { if (selectedConn != null) void loadDatabases(selectedConn) }, [selectedConn])
  useEffect(() => {
    if (selectedConn != null && selectedDb != null) void loadPolicy(selectedConn, selectedDb)
  }, [selectedConn, selectedDb])
  useEffect(() => { if (aiEnabled) void loadConnections() }, [aiEnabled])
  useEffect(() => { if (aiEnabled) void loadAudit() }, [aiEnabled])

  return (
    <main style={{
      fontFamily: 'system-ui', padding: 24, maxWidth: 720, margin: '0 auto',
      minHeight: '100vh', background: 'var(--bg-primary)', color: 'var(--text-primary)',
    }}>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <h1 style={{ margin: 0 }}>{t('settings.title')}</h1>
        <button onClick={onClose}>{t('common.back')}</button>
      </header>

      <section style={{ marginBottom: 32 }}>
        <h2>{t('settings.change_password')}</h2>
        <form onSubmit={changePassword} style={{ display: 'grid', gap: 8, maxWidth: 360 }}>
          <input type="password" placeholder={t('settings.current_password')} value={oldPw} onChange={(e) => setOld(e.target.value)} required />
          <input type="password" placeholder={t('settings.new_password')} value={newPw} onChange={(e) => setNew(e.target.value)} required />
          <button type="submit">{t('settings.change_button')}</button>
          {pwMsg && <div style={{ fontSize: 14 }}>{pwMsg}</div>}
        </form>
      </section>

      <section style={section}>
        <h2>{t('settings.ai_writes.title')}</h2>
        <label>
          <input type="checkbox" checked={aiEnabled} onChange={(e) => void toggleMaster(e.target.checked)} />
          {' '}{t('settings.ai_writes.master_label')}
        </label>
        {!aiEnabled && <p style={hint}>{t('settings.ai_writes.master_hint_off')}</p>}
        {aiEnabled && (
          <div>
            <div style={pickerRow}>
              <label>
                {t('settings.ai_writes.connection')}:{' '}
                <select value={selectedConn ?? ''} onChange={(e) => setSelectedConn(e.target.value ? Number(e.target.value) : null)}>
                  <option value="">—</option>
                  {connections.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                </select>
              </label>
              <label>
                {t('settings.ai_writes.database')}:{' '}
                <select value={selectedDb ?? ''} onChange={(e) => setSelectedDb(e.target.value || null)} disabled={selectedConn == null}>
                  <option value="">—</option>
                  {databases.map((d) => <option key={d} value={d}>{d}</option>)}
                </select>
              </label>
            </div>
            {selectedConn != null && selectedDb != null && (
              <AIWritePolicyTable
                connId={selectedConn}
                db={selectedDb}
                configured={policy.configured}
                unconfigured={policy.unconfigured}
                onUpsert={upsertPolicy}
                onBatch={batchPolicy}
              />
            )}
            <h4>{t('settings.ai_writes.audit_title')}</h4>
            <AIWriteAuditList rows={audit} />
          </div>
        )}
      </section>

      <section style={{ marginBottom: 32 }}>
        <h2>{t('settings.api_keys_title')}</h2>
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
            keyState={keys.claudecode}
            onChanged={() => void loadKeys()}
          />
        )}
        {keys && (
          <CodexConnect
            keyState={keys.codex}
            onChanged={() => void loadKeys()}
          />
        )}
      </section>

      <section>
        <h2>{t('settings.active_sessions')}</h2>
        {loadErr && <div style={{ color: 'crimson' }}>{loadErr}</div>}
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              <th style={th}>{t('settings.column_id')}</th>
              <th style={th}>{t('settings.column_device')}</th>
              <th style={th}>{t('settings.column_last_used')}</th>
              <th style={th}>{t('settings.column_expires')}</th>
              <th style={th}></th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={s.id}>
                <td style={td}>
                  {s.id}
                  {s.current && <span style={{ marginLeft: 6, fontSize: 11, color: 'green' }}>{t('settings.session_current')}</span>}
                </td>
                <td style={td}>{s.user_agent}</td>
                <td style={td}>{new Date(s.last_used_at).toLocaleString()}</td>
                <td style={td}>{new Date(s.expires_at).toLocaleString()}</td>
                <td style={td}>
                  {!s.current && <button onClick={() => revoke(s.id)}>{t('settings.revoke')}</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
    </main>
  )
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
  useClipboardAutoFill(!!pendingState, 'http://localhost/callback', setCode)

  async function startConnect() {
    setMsg(null)
    setBusy(true)
    try {
      const r = await api.post<{ auth_url: string; verifier: string; state: string }>('/api/auth/claudecode/start', {})
      setPending({ verifier: r.verifier, state: r.state })
      window.open(r.auth_url, '_blank', 'noopener')
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
      window.open(r.auth_url, '_blank', 'noopener')
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

const th: CSSProperties = { textAlign: 'left', padding: '6px 8px', borderBottom: '1px solid var(--border-color)', fontSize: 13 }
const td: CSSProperties = { padding: '6px 8px', borderBottom: '1px solid var(--table-border)', fontSize: 13 }
const section: CSSProperties = { marginBottom: 32 }
const hint: CSSProperties = { color: 'var(--text-muted, #888)', fontSize: 12 }
const pickerRow: CSSProperties = { display: 'flex', gap: 16, alignItems: 'center', padding: '8px 0' }
