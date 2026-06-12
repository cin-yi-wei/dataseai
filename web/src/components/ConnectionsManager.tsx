import { useEffect, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { Connection, ConnectionEngine, ConnectionInput, useConnections } from '../store/connections'
import ConnectionDialog from './ConnectionDialog'
import { useT } from '../i18n'

interface Props {
  onClose: () => void
}

type Editing = Connection | 'new' | { dup: Connection } | null

const SWATCHES = ['#ff5b5b', '#ff9f43', '#2ecc71', '#22c3c3', '#4c8dff', '#a06bff', '#8b94a3']
const UNGROUPED = '__ungrouped__'

export default function ConnectionsManager({ onClose }: Props) {
  const t = useT()
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const remove = useConnections((s) => s.remove)
  const update = useConnections((s) => s.update)
  const [editing, setEditing] = useState<Editing>(null)
  const [selectedId, setSelectedId] = useState<number | null>(null)

  useEffect(() => { void load() }, [load])

  // Default selection: first connection once loaded / after the list changes.
  useEffect(() => {
    if (list.length === 0) { setSelectedId(null); return }
    if (selectedId == null || !list.some((c) => c.id === selectedId)) {
      setSelectedId(list[0].id)
    }
  }, [list, selectedId])

  const selected = list.find((c) => c.id === selectedId) ?? null

  // Group connections by group_name (blank → ungrouped bucket), groups sorted,
  // ungrouped last.
  const groups = useMemo(() => {
    const m = new Map<string, Connection[]>()
    for (const c of list) {
      const g = (c.group_name || '').trim() || UNGROUPED
      if (!m.has(g)) m.set(g, [])
      m.get(g)!.push(c)
    }
    const keys = [...m.keys()].sort((a, b) => {
      if (a === UNGROUPED) return 1
      if (b === UNGROUPED) return -1
      return a.localeCompare(b)
    })
    return keys.map((k) => ({ key: k, items: m.get(k)! }))
  }, [list])

  const dialogInitial = editing && editing !== 'new'
    ? ('dup' in editing ? editing.dup : editing)
    : null
  const isDup = editing && typeof editing === 'object' && 'dup' in editing
  const dialogMode: 'edit' | 'create' = editing === 'new' || isDup ? 'create' : 'edit'

  async function setColor(c: Connection, color: string) {
    try { await update(c.id, toInput(c, { color })) } catch { /* surfaced by store */ }
  }

  return (
    <main style={page}>
      <header style={pageHeader}>
        <h1 style={{ margin: 0, fontSize: 22 }}>{t('connections.title')}</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => setEditing('new')} style={primaryBtn}>{t('connections.new')}</button>
          <button onClick={onClose}>{t('connections.back')}</button>
        </div>
      </header>

      {list.length === 0 ? (
        <div style={emptyState}>{t('connections.no_connections')}</div>
      ) : (
        <div style={layout}>
          {/* ---- left: grouped tree ---- */}
          <div style={tree}>
            {groups.map((g) => (
              <div key={g.key}>
                <div style={groupHdr}>
                  <span style={{ ...groupDot, background: groupColor(g.items) }} />
                  <span style={groupTitle}>{g.key === UNGROUPED ? t('connections.ungrouped') : g.key}</span>
                  <span style={groupCount}>{g.items.length}</span>
                </div>
                {g.items.map((c) => {
                  const env = envOf(c)
                  return (
                    <div
                      key={c.id}
                      onClick={() => setSelectedId(c.id)}
                      style={{ ...treeItem, ...(c.id === selectedId ? treeItemSel : null) }}
                    >
                      <span style={{ ...treeDot, background: c.color || 'var(--accent)' }} />
                      <span style={treeName}>{c.name}</span>
                      {c.ssh_enabled && <span title="SSH">🔒</span>}
                      {env && <span style={{ ...envBadge, ...envStyle(env) }}>{env}</span>}
                    </div>
                  )
                })}
              </div>
            ))}
          </div>

          {/* ---- right: detail ---- */}
          <div style={detail}>
            {selected ? (
              <DetailPanel
                c={selected}
                onEdit={() => setEditing(selected)}
                onDup={() => setEditing({ dup: selected })}
                onDelete={() => { if (confirm(t('connections.delete_confirm', { name: selected.name }))) void remove(selected.id) }}
                onColor={(color) => void setColor(selected, color)}
              />
            ) : (
              <div style={{ color: 'var(--text-muted)', padding: 24 }}>{t('connections.select_hint') || '←'}</div>
            )}
          </div>
        </div>
      )}

      {editing && (
        <ConnectionDialog
          initial={dialogInitial}
          mode={dialogMode}
          onClose={() => setEditing(null)}
          onSaved={() => void load()}
        />
      )}
    </main>
  )
}

function DetailPanel({ c, onEdit, onDup, onDelete, onColor }: {
  c: Connection
  onEdit: () => void
  onDup: () => void
  onDelete: () => void
  onColor: (color: string) => void
}) {
  const t = useT()
  const env = envOf(c)
  const isProd = env === 'PROD'
  return (
    <div>
      <h2 style={detailTitle}>
        <span style={{ ...titleDot, background: c.color || 'var(--accent)' }} />
        {c.name}
        {env && <span style={{ ...envBadge, ...envStyle(env) }}>{env}{isProd ? ' ⚠' : ''}</span>}
      </h2>
      {isProd && (
        <div style={prodWarn}>{t('connections.prod_warn') || '⚠ Production — 寫入操作會二次確認'}</div>
      )}

      <Field k={t('connections.column_engine') || 'Engine'} v={engineLabel(c.engine)} />
      <Field k={t('connections.column_host')} v={`${c.host}:${c.port}`} mono />
      {c.default_db && <Field k={t('connection_dialog.default_db')} v={c.default_db} mono />}
      <Field k={t('connections.column_user')} v={c.username} mono />
      <Field k={t('connections.column_tls')} v={c.tls} />
      <Field k={t('connections.group') || 'Group'} v={c.group_name || (t('connections.ungrouped') || '—')} />
      {c.ssh_enabled && <Field k="SSH" v={`${c.ssh_user || ''}@${c.ssh_host || ''}:${c.ssh_port || 22}`} mono />}

      <div style={{ ...field, borderBottom: 'none' }}>
        <span style={fieldK}>{t('connections.color') || 'Color'}</span>
        <span style={{ display: 'flex', gap: 8 }}>
          {SWATCHES.map((s) => (
            <span
              key={s}
              onClick={() => onColor(s)}
              title={s}
              style={{
                width: 24, height: 24, borderRadius: 7, background: s, cursor: 'pointer',
                border: (c.color || '').toLowerCase() === s ? '2px solid var(--text-primary)' : '2px solid transparent',
              }}
            />
          ))}
        </span>
      </div>

      <div style={{ display: 'flex', gap: 8, marginTop: 18, flexWrap: 'wrap' }}>
        <button onClick={onEdit}>{t('common.edit')}</button>
        <button onClick={onDup} title={t('connections.duplicate_tooltip')}>{t('common.duplicate')}</button>
        <button onClick={onDelete} style={{ color: 'var(--danger)' }}>{t('common.delete')}</button>
      </div>
    </div>
  )
}

function Field({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <div style={field}>
      <span style={fieldK}>{k}</span>
      <span style={{ ...fieldV, fontFamily: mono ? 'monospace' : undefined }}>{v}</span>
    </div>
  )
}

// envOf guesses the environment badge from the connection name/group.
function envOf(c: Connection): 'PROD' | 'RELEASE' | 'DEV' | '' {
  const s = `${c.name} ${c.group_name || ''}`.toLowerCase()
  if (/\bprod|production\b/.test(s)) return 'PROD'
  if (/release|staging|\bstg\b|\brc\b/.test(s)) return 'RELEASE'
  if (/\bdev\b|local|localhost|sandbox/.test(s)) return 'DEV'
  return ''
}

function envStyle(env: string): CSSProperties {
  if (env === 'PROD') return { background: '#3a1417', color: '#ff8a8a', border: '1px solid #5a1f24' }
  if (env === 'RELEASE') return { background: '#3a2c12', color: '#ffc46b', border: '1px solid #5a4420' }
  return { background: '#15303a', color: '#7fd0ff', border: '1px solid #1f4a5a' }
}

// groupColor: use the first colored connection in a group as the group dot.
function groupColor(items: Connection[]): string {
  const c = items.find((x) => x.color)
  return c?.color || 'var(--text-muted)'
}

function toInput(c: Connection, over: Partial<ConnectionInput>): ConnectionInput {
  return {
    name: c.name, engine: c.engine, host: c.host, port: c.port, username: c.username,
    password: '', default_db: c.default_db, tls: c.tls, color: c.color, group_name: c.group_name,
    ssh_enabled: c.ssh_enabled, ssh_host: c.ssh_host, ssh_port: c.ssh_port, ssh_user: c.ssh_user,
    via_agent_id: c.via_agent_id ?? null,
    ...over,
  }
}

const ENGINE_LABELS: Record<string, string> = {
  mysql: 'MySQL', postgres: 'PostgreSQL', mssql: 'SQL Server', bytehouse: 'ByteHouse',
  sqlite: 'SQLite', mariadb: 'MariaDB', tidb: 'TiDB', cockroachdb: 'CockroachDB',
  redshift: 'Redshift', singlestore: 'SingleStore', duckdb: 'DuckDB', snowflake: 'Snowflake',
  clickhouse: 'ClickHouse', planetscale: 'PlanetScale', oracle: 'Oracle',
}
function engineLabel(engine: ConnectionEngine | undefined): string {
  return ENGINE_LABELS[engine || 'mysql'] || 'MySQL'
}

const page: CSSProperties = {
  fontFamily: 'system-ui', padding: 24, maxWidth: 1100, margin: '0 auto',
  minHeight: '100vh', background: 'var(--bg-primary)', color: 'var(--text-primary)',
  boxSizing: 'border-box',
}
const pageHeader: CSSProperties = {
  display: 'flex', justifyContent: 'space-between', alignItems: 'center',
  marginBottom: 20, gap: 12, flexWrap: 'wrap',
}
const primaryBtn: CSSProperties = {
  background: 'var(--accent)', color: 'white', border: '1px solid var(--accent)', fontWeight: 600,
}
const emptyState: CSSProperties = {
  textAlign: 'center', padding: 48, color: 'var(--text-muted)', fontSize: 14,
  background: 'var(--bg-elevated)', borderRadius: 8, border: '1px dashed var(--border-color)',
}
const layout: CSSProperties = {
  display: 'grid', gridTemplateColumns: '280px 1fr', gap: 0,
  border: '1px solid var(--border-color)', borderRadius: 10, overflow: 'hidden', minHeight: 440,
}
const tree: CSSProperties = {
  background: 'var(--bg-elevated)', borderRight: '1px solid var(--border-color)', padding: 10, overflowY: 'auto',
}
const groupHdr: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, margin: '14px 8px 6px',
}
const groupDot: CSSProperties = { width: 9, height: 9, borderRadius: 3, flexShrink: 0 }
const groupTitle: CSSProperties = {
  fontSize: 11, fontWeight: 700, letterSpacing: 0.4, textTransform: 'uppercase', color: 'var(--text-muted)',
}
const groupCount: CSSProperties = {
  marginLeft: 'auto', fontSize: 11, color: 'var(--text-muted)',
  background: 'var(--bg-primary)', padding: '1px 8px', borderRadius: 12,
}
const treeItem: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 9, padding: '8px 10px',
  borderRadius: 8, cursor: 'pointer', fontSize: 14,
}
const treeItemSel: CSSProperties = { background: 'var(--bg-primary)', outline: '1px solid var(--accent)' }
const treeDot: CSSProperties = { width: 9, height: 9, borderRadius: 3, flexShrink: 0 }
const treeName: CSSProperties = { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1, minWidth: 0 }
const detail: CSSProperties = { padding: 22 }
const detailTitle: CSSProperties = { margin: '0 0 6px', fontSize: 20, display: 'flex', alignItems: 'center', gap: 10 }
const titleDot: CSSProperties = { width: 12, height: 12, borderRadius: 4, flexShrink: 0 }
const prodWarn: CSSProperties = {
  fontSize: 12.5, color: '#ff8a8a', background: '#2a1518', border: '1px solid #5a1f24',
  borderRadius: 8, padding: '8px 12px', margin: '8px 0 14px',
}
const field: CSSProperties = {
  display: 'grid', gridTemplateColumns: '120px 1fr', gap: 8, padding: '10px 0',
  borderBottom: '1px solid var(--border-color)', fontSize: 14, alignItems: 'center',
}
const fieldK: CSSProperties = { color: 'var(--text-muted)' }
const fieldV: CSSProperties = { overflow: 'hidden', textOverflow: 'ellipsis' }
const envBadge: CSSProperties = {
  fontSize: 10, padding: '2px 7px', borderRadius: 999, fontWeight: 700, letterSpacing: 0.4, whiteSpace: 'nowrap',
}
