import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { api } from '../lib/api'
import { useT } from '../i18n'
import AIWritePolicyTable, { type AIPolicy, type TablePolicy } from './AIWritePolicyTable'
import AIWriteAuditList, { type AuditRow } from './AIWriteAuditList'

export type WriteScope = 'ai' | 'dml'

interface Props {
  scope: WriteScope
  title: string
  masterLabel: string
  masterHintOff: string
  showAudit: boolean
}

// WritesSection renders the master switch + per-(conn, db, table) policy
// table for one scope. The DataGrid (dml) and AI (ai) sections share this
// component so the UI stays consistent — only the audit log is rendered
// for the AI scope (DataGrid writes don't currently have a separate audit
// trail).
export default function WritesSection({ scope, title, masterLabel, masterHintOff, showAudit }: Props) {
  const t = useT()
  const [enabled, setEnabled] = useState(false)
  const [audit, setAudit] = useState<AuditRow[]>([])
  const [connections, setConnections] = useState<{ id: number; name: string }[]>([])
  const [selectedConn, setSelectedConn] = useState<number | null>(null)
  const [databases, setDatabases] = useState<string[]>([])
  const [selectedDb, setSelectedDb] = useState<string | null>(null)
  const [policy, setPolicy] = useState<{ configured: TablePolicy[]; unconfigured: string[] }>(
    { configured: [], unconfigured: [] }
  )

  const qs = `?scope=${scope}`

  async function loadMaster() {
    try {
      const r = await api.get<{ enabled: boolean }>(`/api/auth/ai-writes${qs}`)
      setEnabled(r.enabled)
    } catch {/* leave default */}
  }
  async function toggleMaster(v: boolean) {
    await api.put(`/api/auth/ai-writes${qs}`, { enabled: v })
    setEnabled(v)
    if (v) {
      await loadConnections()
      if (showAudit) await loadAudit()
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
    const r = await api.get<typeof policy>(
      `/api/auth/ai-policy${qs}&conn=${connId}&db=${encodeURIComponent(db)}`,
    )
    setPolicy({ configured: r.configured ?? [], unconfigured: r.unconfigured ?? [] })
  }
  async function loadAudit() {
    const rows = await api.get<AuditRow[]>(`/api/auth/ai-audit${qs}&limit=50`)
    setAudit(rows ?? [])
  }
  async function upsertPolicy(connId: number, db: string, table: string, p: AIPolicy) {
    await api.put(`/api/auth/ai-policy${qs}`, { conn: connId, db, table, policy: p })
    await loadPolicy(connId, db)
  }
  async function batchPolicy(connId: number, db: string, tables: string[], p: AIPolicy) {
    await api.put(`/api/auth/ai-policy/batch${qs}`, { conn: connId, db, tables, policy: p })
    await loadPolicy(connId, db)
  }
  // Flip a single flag across every configured row, leaving other flags
  // alone. We loop per-table upserts in parallel and reload once at the
  // end so the policy list updates in one paint. Each row's other-flag
  // values are taken from policy.configured.
  async function setColumnForAll(connId: number, db: string, flag: keyof AIPolicy, value: boolean) {
    await Promise.all(policy.configured.map((row) =>
      api.put(`/api/auth/ai-policy${qs}`, {
        conn: connId, db, table: row.table,
        policy: { ...row.policy, [flag]: value },
      }),
    ))
    await loadPolicy(connId, db)
  }

  useEffect(() => { void loadMaster() // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scope])
  useEffect(() => { if (selectedConn != null) void loadDatabases(selectedConn) }, [selectedConn])
  useEffect(() => {
    if (selectedConn != null && selectedDb != null) void loadPolicy(selectedConn, selectedDb)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedConn, selectedDb])
  useEffect(() => { if (enabled) void loadConnections() // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled])
  useEffect(() => { if (enabled && showAudit) void loadAudit() // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled])

  return (
    <section style={section}>
      <h3 style={{
        fontSize: 14, margin: '0 0 10px',
        fontWeight: 600,
        color: 'var(--text-secondary, var(--text-primary))',
        textTransform: 'uppercase',
        letterSpacing: 0.4,
      }}>{title}</h3>
      <label>
        <input type="checkbox" checked={enabled} onChange={(e) => void toggleMaster(e.target.checked)} />
        {' '}{masterLabel}
      </label>
      {!enabled && <p style={hint}>{masterHintOff}</p>}
      {enabled && (
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
              key={`${scope}-${selectedConn}-${selectedDb}`}
              connId={selectedConn}
              db={selectedDb}
              configured={policy.configured}
              unconfigured={policy.unconfigured}
              onUpsert={upsertPolicy}
              onBatch={batchPolicy}
              onColumnSetAll={setColumnForAll}
            />
          )}
          {showAudit && (
            <details style={{ marginTop: 12, padding: '6px 10px', border: '1px solid var(--border-color)', borderRadius: 4 }}>
              <summary style={{ cursor: 'pointer', fontSize: 13, color: 'var(--text-muted)' }}>
                {t('settings.ai_writes.audit_title')}
                {audit.length > 0 && <span style={{ marginLeft: 6 }}>({audit.length})</span>}
              </summary>
              <div style={{ marginTop: 8 }}>
                <AIWriteAuditList rows={audit} />
              </div>
            </details>
          )}
        </div>
      )}
    </section>
  )
}

const section: CSSProperties = {
  marginBottom: 16, marginLeft: 12,
  padding: '12px 14px',
  background: 'var(--bg-elevated, transparent)',
  border: '1px solid var(--border-color)',
  borderLeft: '3px solid var(--border-strong, var(--border-color))',
  borderRadius: 4,
}
const hint: CSSProperties = { fontSize: 13, color: 'var(--text-muted)' }
const pickerRow: CSSProperties = { display: 'flex', gap: 12, margin: '8px 0', flexWrap: 'wrap' }
