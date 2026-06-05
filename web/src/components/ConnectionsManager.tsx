import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { Connection, useConnections } from '../store/connections'
import ConnectionDialog from './ConnectionDialog'
import { useT } from '../i18n'

interface Props {
  onClose: () => void
}

type Editing = Connection | 'new' | { dup: Connection } | null

export default function ConnectionsManager({ onClose }: Props) {
  const t = useT()
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const remove = useConnections((s) => s.remove)
  const [editing, setEditing] = useState<Editing>(null)

  const dialogInitial = editing && editing !== 'new'
    ? ('dup' in editing ? editing.dup : editing)
    : null
  const isDup = editing && typeof editing === 'object' && 'dup' in editing
  const dialogMode: 'edit' | 'create' = editing === 'new' || isDup ? 'create' : 'edit'

  useEffect(() => { void load() }, [load])

  return (
    <main style={page}>
      <header style={pageHeader}>
        <h1 style={{ margin: 0, fontSize: 22 }}>{t('connections.title')}</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button onClick={() => setEditing('new')} style={primaryBtn}>{t('connections.new')}</button>
          <button onClick={onClose}>{t('connections.back')}</button>
        </div>
      </header>

      {list.length === 0 && (
        <div style={emptyState}>
          {t('connections.no_connections')}
        </div>
      )}

      <div style={grid}>
        {list.map((c) => (
          <div key={c.id} style={card}>
            <div style={cardHeader}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, flex: 1, minWidth: 0 }}>
                <div style={{
                  width: 10, height: 10, borderRadius: '50%',
                  background: c.color || 'var(--accent)',
                  flexShrink: 0,
                }} />
                <strong style={{ fontSize: 15, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {c.name}
                </strong>
              </div>
              {c.ssh_enabled && (
                <span style={badge} title="SSH Tunnel">🔒 SSH</span>
              )}
            </div>

            <div style={cardBody}>
              <div style={row}>
                <span style={rowLabel}>{t('connections.column_host')}:</span>
                <span style={rowValue}>{c.host}:{c.port}</span>
              </div>
              <div style={row}>
                <span style={rowLabel}>{t('connections.column_user')}:</span>
                <span style={rowValue}>{c.username}</span>
              </div>
              {c.default_db && (
                <div style={row}>
                  <span style={rowLabel}>{t('connection_dialog.default_db')}:</span>
                  <span style={rowValue}>{c.default_db}</span>
                </div>
              )}
              <div style={row}>
                <span style={rowLabel}>{t('connections.column_tls')}:</span>
                <span style={rowValue}>{c.tls}</span>
              </div>
            </div>

            <div style={cardActions}>
              <button onClick={() => setEditing(c)} style={actionBtn}>{t('common.edit')}</button>
              <button
                onClick={() => setEditing({ dup: c })}
                title={t('connections.duplicate_tooltip')}
                style={actionBtn}
              >
                {t('common.duplicate')}
              </button>
              <button
                onClick={() => { if (confirm(t('connections.delete_confirm', { name: c.name }))) void remove(c.id) }}
                style={{ ...actionBtn, color: 'var(--danger)' }}
              >
                {t('common.delete')}
              </button>
            </div>
          </div>
        ))}
      </div>

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
  background: 'var(--accent)', color: 'white', border: '1px solid var(--accent)',
  fontWeight: 600,
}
const emptyState: CSSProperties = {
  textAlign: 'center', padding: 48, color: 'var(--text-muted)',
  fontSize: 14, background: 'var(--bg-elevated)', borderRadius: 8,
  border: '1px dashed var(--border-color)',
}
const grid: CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
  gap: 14,
}
const card: CSSProperties = {
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border-color)',
  borderRadius: 8,
  padding: 14,
  display: 'flex', flexDirection: 'column', gap: 10,
  minWidth: 0, overflow: 'hidden',
}
const cardHeader: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8,
  paddingBottom: 10,
  borderBottom: '1px solid var(--border-color)',
}
const cardBody: CSSProperties = {
  display: 'flex', flexDirection: 'column', gap: 6, fontSize: 13,
}
const row: CSSProperties = {
  display: 'flex', gap: 8, alignItems: 'baseline', minWidth: 0,
}
const rowLabel: CSSProperties = {
  color: 'var(--text-muted)', fontSize: 12, minWidth: 80, flexShrink: 0,
}
const rowValue: CSSProperties = {
  fontFamily: 'monospace', fontSize: 12,
  flex: 1, minWidth: 0,
  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
}
const cardActions: CSSProperties = {
  display: 'flex', gap: 6, marginTop: 'auto', flexWrap: 'wrap',
  paddingTop: 10,
  borderTop: '1px solid var(--border-color)',
}
const actionBtn: CSSProperties = {
  flex: '1 1 60px', minWidth: 60, fontSize: 12, padding: '4px 8px',
}
const badge: CSSProperties = {
  fontSize: 10, padding: '2px 8px', borderRadius: 12,
  background: 'rgba(34, 197, 94, 0.15)', color: '#22c55e',
  whiteSpace: 'nowrap',
}
