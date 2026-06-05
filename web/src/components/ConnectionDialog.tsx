import { FormEvent, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { ApiError } from '../lib/api'
import { Connection, ConnectionInput, useConnections } from '../store/connections'
import { useT } from '../i18n'

interface Props {
  initial?: Connection | null
  mode?: 'edit' | 'create' // when 'create', initial is just a preset; submit will create new
  onClose: () => void
  onSaved: (c: Connection) => void
}

export default function ConnectionDialog({ initial, mode, onClose, onSaved }: Props) {
  const t = useT()
  const create = useConnections((s) => s.create)
  const update = useConnections((s) => s.update)
  const testConn = useConnections((s) => s.test)
  const effectiveMode: 'edit' | 'create' = mode ?? (initial ? 'edit' : 'create')
  const isDup = effectiveMode === 'create' && !!initial
  const [name, setName] = useState(isDup ? `${initial!.name} (copy)` : (initial?.name ?? ''))
  const [host, setHost] = useState(initial?.host ?? 'localhost')
  const [port, setPort] = useState<number>(initial?.port ?? 3306)
  const [username, setUsername] = useState(initial?.username ?? '')
  const [password, setPassword] = useState('')
  const [defaultDB, setDefaultDB] = useState(initial?.default_db ?? '')
  const [tls, setTLS] = useState<ConnectionInput['tls']>(initial?.tls ?? 'disabled')
  const [sshEnabled, setSSHEnabled] = useState<boolean>(initial?.ssh_enabled ?? false)
  const [sshHost, setSSHHost] = useState(initial?.ssh_host ?? '')
  const [sshPort, setSSHPort] = useState<number>(initial?.ssh_port ?? 22)
  const [sshUser, setSSHUser] = useState(initial?.ssh_user ?? '')
  const [sshPassword, setSSHPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [testMsg, setTestMsg] = useState<string | null>(null)

  useEffect(() => {
    if (!initial) setPassword('')
  }, [initial])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const input: ConnectionInput = {
        name, host, port, username, password, default_db: defaultDB, tls,
        ssh_enabled: sshEnabled, ssh_host: sshHost, ssh_port: sshPort, ssh_user: sshUser, ssh_password: sshPassword,
      }
      const saved = effectiveMode === 'edit' && initial
        ? await update(initial.id, input)
        : await create(input)
      onSaved(saved)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('connection_dialog.save_failed'))
    } finally {
      setBusy(false)
    }
  }

  async function runTest() {
    if (!initial || effectiveMode !== 'edit') {
      setTestMsg(t('connection_dialog.save_first_test'))
      return
    }
    setTestMsg(t('connection_dialog.testing'))
    try {
      const r = await testConn(initial.id)
      setTestMsg(r.ok ? t('connection_dialog.connected') : t('connection_dialog.test_failed', { message: r.message }))
    } catch (err) {
      setTestMsg(err instanceof ApiError ? err.message : t('connection_dialog.save_failed'))
    }
  }

  return (
    <div style={backdrop}>
      <div data-modal style={modal}>
        <h2 style={{ marginTop: 0 }}>
          {effectiveMode === 'edit' ? t('connection_dialog.edit_title') : (isDup ? t('connection_dialog.duplicate_title') : t('connection_dialog.new_title'))}
        </h2>
        <form onSubmit={submit} style={{ display: 'grid', gap: 10 }}>
          <label>{t('connection_dialog.name')} <input value={name} onChange={(e) => setName(e.target.value)} required style={input} /></label>
          <label>{t('connection_dialog.host')} <input value={host} onChange={(e) => setHost(e.target.value)} required style={input} /></label>
          <label>{t('connection_dialog.port')} <input type="number" value={port} onChange={(e) => setPort(parseInt(e.target.value || '0', 10))} required style={input} /></label>
          <label>{t('connection_dialog.user')} <input value={username} onChange={(e) => setUsername(e.target.value)} required style={input} /></label>
          <label>{t('connection_dialog.password')} <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={effectiveMode === 'edit' ? t('connection_dialog.password_keep') : ''} required={effectiveMode !== 'edit'} style={input} /></label>
          <label>{t('connection_dialog.default_db')} <input value={defaultDB} onChange={(e) => setDefaultDB(e.target.value)} style={input} /></label>
          <label>{t('connection_dialog.tls')}
            <select value={tls} onChange={(e) => setTLS(e.target.value as ConnectionInput['tls'])} style={input}>
              <option value="disabled">{t('connection_dialog.tls_disabled')}</option>
              <option value="preferred">{t('connection_dialog.tls_preferred')}</option>
              <option value="required">{t('connection_dialog.tls_required')}</option>
            </select>
          </label>

          {/* SSH tunnel section */}
          <div style={sshSection}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontWeight: 600 }}>
              <input type="checkbox" checked={sshEnabled} onChange={(e) => setSSHEnabled(e.target.checked)} />
              {t('connection_dialog.ssh_tunnel')}
            </label>
            {sshEnabled && (
              <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
                <div style={{ display: 'flex', gap: 8 }}>
                  <label style={{ flex: 2 }}>
                    {t('connection_dialog.ssh_host')}
                    <input value={sshHost} onChange={(e) => setSSHHost(e.target.value)} required={sshEnabled} style={input} />
                  </label>
                  <label style={{ flex: 1 }}>
                    {t('connection_dialog.ssh_port')}
                    <input type="number" value={sshPort} onChange={(e) => setSSHPort(parseInt(e.target.value || '22', 10))} style={input} />
                  </label>
                </div>
                <label>
                  {t('connection_dialog.ssh_user')}
                  <input value={sshUser} onChange={(e) => setSSHUser(e.target.value)} required={sshEnabled} style={input} />
                </label>
                <label>
                  {t('connection_dialog.ssh_password')}
                  <input
                    type="password"
                    value={sshPassword}
                    onChange={(e) => setSSHPassword(e.target.value)}
                    placeholder={initial?.ssh_enabled ? t('connection_dialog.password_keep') : ''}
                    required={sshEnabled && effectiveMode !== 'edit' && !initial?.ssh_enabled}
                    style={input}
                  />
                </label>
              </div>
            )}
          </div>

          {error && <div style={{ color: 'crimson', fontSize: 13 }}>{error}</div>}
          {testMsg && <div style={{ fontSize: 13 }}>{testMsg}</div>}
          <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 8 }}>
            {initial && <button type="button" onClick={runTest}>{t('connection_dialog.test')}</button>}
            <button type="button" onClick={onClose}>{t('common.cancel')}</button>
            <button disabled={busy} type="submit">{busy ? t('common.saving') : t('common.save')}</button>
          </div>
        </form>
      </div>
    </div>
  )
}

const backdrop: CSSProperties = {
  position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)',
  display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100,
}
const modal: CSSProperties = {
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
  padding: 20, borderRadius: 8,
  width: 'min(400px, calc(100vw - 24px))',
  maxWidth: '95vw',
  maxHeight: '90vh', overflow: 'auto',
  fontFamily: 'system-ui',
  border: '1px solid var(--border-color)',
  boxSizing: 'border-box',
}
const input: CSSProperties = { display: 'block', width: '100%', padding: '4px 6px', marginTop: 2, boxSizing: 'border-box' }
const sshSection: CSSProperties = {
  padding: 10, borderRadius: 6,
  background: 'var(--bg-secondary)',
  border: '1px solid var(--border-color)',
  marginTop: 4,
}
