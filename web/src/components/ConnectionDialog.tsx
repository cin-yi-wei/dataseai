import { FormEvent, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { ApiError } from '../lib/api'
import { Connection, ConnectionInput, useConnections } from '../store/connections'

interface Props {
  initial?: Connection | null
  mode?: 'edit' | 'create' // when 'create', initial is just a preset; submit will create new
  onClose: () => void
  onSaved: (c: Connection) => void
}

export default function ConnectionDialog({ initial, mode, onClose, onSaved }: Props) {
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
      setError(err instanceof ApiError ? err.message : 'save failed')
    } finally {
      setBusy(false)
    }
  }

  async function runTest() {
    if (!initial || effectiveMode !== 'edit') {
      setTestMsg('save first to test')
      return
    }
    setTestMsg('testing…')
    try {
      const r = await testConn(initial.id)
      setTestMsg(r.ok ? 'connected ✓' : `failed: ${r.message}`)
    } catch (err) {
      setTestMsg(err instanceof ApiError ? err.message : 'test failed')
    }
  }

  return (
    <div style={backdrop}>
      <div data-modal style={modal}>
        <h2 style={{ marginTop: 0 }}>
          {effectiveMode === 'edit' ? 'edit connection' : (isDup ? 'duplicate connection' : 'new connection')}
        </h2>
        <form onSubmit={submit} style={{ display: 'grid', gap: 10 }}>
          <label>name <input value={name} onChange={(e) => setName(e.target.value)} required style={input} /></label>
          <label>host <input value={host} onChange={(e) => setHost(e.target.value)} required style={input} /></label>
          <label>port <input type="number" value={port} onChange={(e) => setPort(parseInt(e.target.value || '0', 10))} required style={input} /></label>
          <label>user <input value={username} onChange={(e) => setUsername(e.target.value)} required style={input} /></label>
          <label>password <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder={effectiveMode === 'edit' ? '(leave blank to keep)' : ''} required={effectiveMode !== 'edit'} style={input} /></label>
          <label>default db <input value={defaultDB} onChange={(e) => setDefaultDB(e.target.value)} style={input} /></label>
          <label>tls
            <select value={tls} onChange={(e) => setTLS(e.target.value as ConnectionInput['tls'])} style={input}>
              <option value="disabled">disabled</option>
              <option value="preferred">preferred</option>
              <option value="required">required</option>
            </select>
          </label>

          {/* SSH tunnel section */}
          <div style={sshSection}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontWeight: 600 }}>
              <input type="checkbox" checked={sshEnabled} onChange={(e) => setSSHEnabled(e.target.checked)} />
              SSH Tunnel
            </label>
            {sshEnabled && (
              <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
                <div style={{ display: 'flex', gap: 8 }}>
                  <label style={{ flex: 2 }}>
                    SSH Host
                    <input value={sshHost} onChange={(e) => setSSHHost(e.target.value)} required={sshEnabled} style={input} />
                  </label>
                  <label style={{ flex: 1 }}>
                    Port
                    <input type="number" value={sshPort} onChange={(e) => setSSHPort(parseInt(e.target.value || '22', 10))} style={input} />
                  </label>
                </div>
                <label>
                  SSH User
                  <input value={sshUser} onChange={(e) => setSSHUser(e.target.value)} required={sshEnabled} style={input} />
                </label>
                <label>
                  SSH Password
                  <input
                    type="password"
                    value={sshPassword}
                    onChange={(e) => setSSHPassword(e.target.value)}
                    placeholder={initial?.ssh_enabled ? '(leave blank to keep)' : ''}
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
            {initial && <button type="button" onClick={runTest}>test</button>}
            <button type="button" onClick={onClose}>cancel</button>
            <button disabled={busy} type="submit">{busy ? 'saving…' : 'save'}</button>
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
  padding: 20, borderRadius: 8, minWidth: 400, maxWidth: '90vw',
  maxHeight: '90vh', overflow: 'auto',
  fontFamily: 'system-ui',
  border: '1px solid var(--border-color)',
}
const input: CSSProperties = { display: 'block', width: '100%', padding: '4px 6px', marginTop: 2, boxSizing: 'border-box' }
const sshSection: CSSProperties = {
  padding: 10, borderRadius: 6,
  background: 'var(--bg-secondary)',
  border: '1px solid var(--border-color)',
  marginTop: 4,
}
