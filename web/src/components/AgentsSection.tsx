import { FormEvent, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { ApiError } from '../lib/api'
import { useAgents } from '../store/agents'
import { useT } from '../i18n'

export function connectorCommand(token: string, origin = window.location.origin) {
  const broker = origin.replace(/^https:/, 'wss:').replace(/^http:/, 'ws:')
  return `dataseai-connector.exe run --token=${token} --server=${broker}/agent --executor=mysql`
}

export default function AgentsSection() {
  const t = useT()
  const agents = useAgents((s) => s.list)
  const load = useAgents((s) => s.load)
  const create = useAgents((s) => s.create)
  const remove = useAgents((s) => s.remove)
  const loading = useAgents((s) => s.loading)
  const error = useAgents((s) => s.error)
  const [name, setName] = useState('')
  const [token, setToken] = useState<string | null>(null)
  const [msg, setMsg] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const command = token ? connectorCommand(token) : ''

  useEffect(() => {
    void load()
    const id = window.setInterval(() => { void load() }, 2000)
    return () => window.clearInterval(id)
  }, [load])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setBusy(true)
    setMsg(null)
    try {
      const r = await create(name.trim())
      setToken(r.token)
      setName('')
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : t('settings.agents.create_failed'))
    } finally {
      setBusy(false)
    }
  }

  async function deleteAgent(id: number) {
    setMsg(null)
    try {
      await remove(id)
    } catch (err) {
      setMsg(err instanceof ApiError ? err.message : t('settings.agents.delete_failed'))
    }
  }

  return (
    <section style={sectionStyle}>
      <h3 style={h3Style}>{t('settings.agents.title')}</h3>
      <form onSubmit={submit} style={{ display: 'flex', gap: 8, marginBottom: 10, flexWrap: 'wrap' }}>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('settings.agents.name_placeholder')}
          style={{ flex: '1 1 220px' }}
        />
        <button type="submit" disabled={busy || !name.trim()}>{t('settings.agents.create')}</button>
      </form>
      {token && (
        <div style={tokenBox}>
          <div style={{ fontSize: 12, color: 'var(--text-muted)', marginBottom: 4 }}>{t('settings.agents.token_once')}</div>
          <code style={{ wordBreak: 'break-all' }}>{token}</code>
          <div style={{ fontSize: 12, color: 'var(--text-muted)', margin: '10px 0 4px' }}>{t('settings.agents.run_command')}</div>
          <code style={commandBox}>{command}</code>
          <div style={{ marginTop: 8 }}>
            <button type="button" onClick={() => void navigator.clipboard?.writeText(command)}>
              {t('settings.agents.copy_command')}
            </button>
          </div>
        </div>
      )}
      {(error || msg) && <div style={{ color: 'crimson', fontSize: 13, marginBottom: 8 }}>{error || msg}</div>}
      {loading && agents.length === 0 && <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>{t('common.loading')}</div>}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {agents.map((a) => (
          <div key={a.id} style={agentRow}>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontWeight: 600, display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                <span>{a.name}</span>
                <span style={a.online ? onlineBadge : offlineBadge}>
                  {a.online ? t('settings.agents.online') : t('settings.agents.offline')}
                </span>
              </div>
              <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                #{a.id} · {a.last_seen_at ? t('settings.agents.last_seen', { time: new Date(a.last_seen_at).toLocaleString() }) : t('settings.agents.never_seen')}
                {a.last_os ? ` · ${a.last_os}` : ''}
                {a.last_version ? ` · ${a.last_version}` : ''}
              </div>
            </div>
            <button onClick={() => { if (confirm(t('settings.agents.delete_confirm', { name: a.name }))) void deleteAgent(a.id) }}>
              {t('common.delete')}
            </button>
          </div>
        ))}
        {!loading && agents.length === 0 && (
          <div style={{ color: 'var(--text-muted)', fontSize: 13 }}>{t('settings.agents.empty')}</div>
        )}
      </div>
    </section>
  )
}

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
const tokenBox: CSSProperties = {
  padding: 10,
  marginBottom: 10,
  background: 'var(--bg-secondary)',
  border: '1px solid var(--border-color)',
  borderRadius: 6,
}
const commandBox: CSSProperties = {
  display: 'block',
  padding: 8,
  background: 'var(--bg-primary)',
  border: '1px solid var(--border-color)',
  borderRadius: 4,
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-all',
}
const agentRow: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  gap: 10,
  padding: '8px 10px',
  border: '1px solid var(--border-color)',
  borderRadius: 6,
}
const onlineBadge: CSSProperties = {
  fontSize: 11,
  padding: '1px 6px',
  borderRadius: 999,
  color: '#116329',
  background: '#dafbe1',
  border: '1px solid #aceebb',
}
const offlineBadge: CSSProperties = {
  fontSize: 11,
  padding: '1px 6px',
  borderRadius: 999,
  color: 'var(--text-muted)',
  background: 'var(--bg-secondary)',
  border: '1px solid var(--border-color)',
}
