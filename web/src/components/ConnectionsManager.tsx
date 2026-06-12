import { useEffect, useMemo, useState } from 'react'
import { useIsNarrow } from '../lib/useIsNarrow'
import type { CSSProperties } from 'react'
import { Connection, ConnectionEngine, ConnectionInput, useConnections } from '../store/connections'
import { useGroupColors } from '../store/groupColors'
import ConnectionDialog from './ConnectionDialog'
import { useT } from '../i18n'

interface Props {
  onClose: () => void
}

type Compose = { mode: 'create' | 'edit'; initial: Connection | null } | null

const SWATCHES = ['#ff5b5b', '#ff9f43', '#2ecc71', '#22c3c3', '#4c8dff', '#a06bff', '#8b94a3']
const UNGROUPED = '__ungrouped__'

export default function ConnectionsManager({ onClose }: Props) {
  const t = useT()
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const remove = useConnections((s) => s.remove)
  const update = useConnections((s) => s.update)
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [composing, setComposing] = useState<Compose>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [pickerGroup, setPickerGroup] = useState<string | null>(null)
  const groupColors = useGroupColors((s) => s.colors)
  const setGroupColor = useGroupColors((s) => s.setColor)
  const narrow = useIsNarrow()
  const [mobileDetail, setMobileDetail] = useState(false)

  function selectConn(id: number) { setSelectedId(id); setComposing(null); if (narrow) setMobileDetail(true) }
  function startCompose(c: Compose) { setComposing(c); if (narrow) setMobileDetail(true) }
  function toggleGroup(key: string) {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key); else next.add(key)
      return next
    })
  }

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

  async function setColor(c: Connection, color: string) {
    try { await update(c.id, toInput(c, { color })) } catch { /* surfaced by store */ }
  }

  return (
    <main style={{ ...page, padding: narrow ? 12 : 24 }}>
      <header style={pageHeader}>
        <h1 style={{ margin: 0, fontSize: 22 }}>{t('connections.title')}</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => startCompose({ mode: 'create', initial: null })} style={primaryBtn}>{t('connections.new')}</button>
          <button onClick={onClose}>{t('connections.back')}</button>
        </div>
      </header>

      {list.length === 0 && !composing ? (
        <div style={emptyState}>{t('connections.no_connections')}</div>
      ) : (
        <div style={{ ...layout, gridTemplateColumns: narrow ? '1fr' : '280px 1fr' }}>
          {/* ---- left: grouped tree ---- */}
          {(!narrow || !mobileDetail) && (
          <div style={{ ...tree, ...(narrow ? treeNarrow : null) }}>
            {groups.map((g) => {
              const isCollapsed = collapsed.has(g.key)
              return (
                <div key={g.key}>
                  <div style={groupHdr} onClick={() => toggleGroup(g.key)} title={isCollapsed ? 'expand' : 'collapse'}>
                    <span style={chev}>{isCollapsed ? '▸' : '▾'}</span>
                    <span
                      onClick={(e) => { e.stopPropagation(); setPickerGroup(pickerGroup === g.key ? null : g.key) }}
                      title="set group color"
                      style={{ ...groupDot, background: groupColors[g.key] || groupColor(g.items), cursor: 'pointer', outline: pickerGroup === g.key ? '2px solid var(--accent)' : undefined }}
                    />
                    <span style={groupTitle}>{g.key === UNGROUPED ? t('connections.ungrouped') : g.key}</span>
                    <span style={groupCount}>{g.items.length}</span>
                  </div>
                  {pickerGroup === g.key && (
                    <div style={{ display: 'flex', gap: 6, padding: '4px 8px 8px 24px', flexWrap: 'wrap' }} onClick={(e) => e.stopPropagation()}>
                      {SWATCHES.map((s) => (
                        <span key={s} onClick={() => { setGroupColor(g.key, s); setPickerGroup(null) }} title={s}
                          style={{ width: 20, height: 20, borderRadius: 6, background: s, cursor: 'pointer',
                            border: (groupColors[g.key] || '').toLowerCase() === s ? '2px solid var(--text-primary)' : '2px solid transparent' }} />
                      ))}
                    </div>
                  )}
                  {!isCollapsed && (
                    <div style={groupItems}>
                      {g.items.map((c) => (
                        <div
                          key={c.id}
                          onClick={() => selectConn(c.id)}
                          style={{ ...treeItem, ...(c.id === selectedId ? treeItemSel : null) }}
                        >
                          <span style={{ ...treeDot, background: c.color || 'var(--accent)' }} />
                          <span style={treeName}>{c.name}</span>
                          {isWarn(c) && <span title="warning">⚠</span>}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
          )}

          {/* ---- right: detail / inline form ---- */}
          {(!narrow || mobileDetail) && (
          <div style={detail}>
            {narrow && (
              <button onClick={() => { setMobileDetail(false); setComposing(null) }} style={{ marginBottom: 12 }}>← {t('connections.back')}</button>
            )}
            {composing ? (
              <ConnectionDialog
                embedded
                initial={composing.initial}
                mode={composing.mode}
                onClose={() => setComposing(null)}
                onSaved={(c) => { void load(); setComposing(null); setSelectedId(c.id) }}
              />
            ) : selected ? (
              <DetailPanel
                c={selected}
                onEdit={() => setComposing({ mode: 'edit', initial: selected })}
                onDup={() => setComposing({ mode: 'create', initial: selected })}
                onDelete={() => { if (confirm(t('connections.delete_confirm', { name: selected.name }))) void remove(selected.id) }}
                onColor={(color) => void setColor(selected, color)}
              />
            ) : (
              <div style={{ color: 'var(--text-muted)', padding: 24 }}>{t('connections.select_hint') || '←'}</div>
            )}
          </div>
          )}
        </div>
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
  const warn = isWarn(c)
  return (
    <div>
      <h2 style={detailTitle}>
        <span style={{ ...titleDot, background: c.color || 'var(--accent)' }} />
        {c.name}
        {warn && <span title="warning" style={{ color: '#ff5b5b' }}>⚠</span>}
      </h2>

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

// isWarn marks a connection as dangerous when the user picked the red color —
// grouping/labels are handled by group_name, so the warning is purely colour.
function isWarn(c: Connection): boolean {
  return (c.color || '').toLowerCase() === '#ff5b5b'
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
const treeNarrow: CSSProperties = { borderRight: 'none' }
const groupHdr: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, margin: '14px 4px 6px', padding: '2px 4px',
  cursor: 'pointer', borderRadius: 6, userSelect: 'none',
}
const chev: CSSProperties = { fontSize: 15, color: 'var(--text-secondary, var(--text-muted))', width: 14, flexShrink: 0, lineHeight: 1 }
const groupItems: CSSProperties = {
  marginLeft: 14, paddingLeft: 18, borderLeft: '1px solid var(--border-color)',
}
const groupDot: CSSProperties = { width: 9, height: 9, borderRadius: 3, flexShrink: 0 }
const groupTitle: CSSProperties = {
  fontSize: 13, fontWeight: 800, letterSpacing: 0.6, textTransform: 'uppercase', color: 'var(--text-primary)',
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
const field: CSSProperties = {
  display: 'grid', gridTemplateColumns: '120px 1fr', gap: 8, padding: '10px 0',
  borderBottom: '1px solid var(--border-color)', fontSize: 14, alignItems: 'center',
}
const fieldK: CSSProperties = { color: 'var(--text-muted)' }
const fieldV: CSSProperties = { overflow: 'hidden', textOverflow: 'ellipsis' }
