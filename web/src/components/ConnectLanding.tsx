import { useEffect, useMemo } from 'react'
import type { CSSProperties } from 'react'
import { Connection, ConnectionEngine, useConnections } from '../store/connections'
import { useActiveConn } from '../store/activeConn'
import { useTabs } from '../store/tabs'
import { useGroupColors } from '../store/groupColors'
import { useT } from '../i18n'

const UNGROUPED = '__ungrouped__'
const ENGINE_LABELS: Record<string, string> = {
  mysql: 'MySQL', postgres: 'PostgreSQL', mssql: 'SQL Server', bytehouse: 'ByteHouse',
  sqlite: 'SQLite', mariadb: 'MariaDB', tidb: 'TiDB', cockroachdb: 'CockroachDB',
  redshift: 'Redshift', singlestore: 'SingleStore', duckdb: 'DuckDB', snowflake: 'Snowflake',
  clickhouse: 'ClickHouse', planetscale: 'PlanetScale', oracle: 'Oracle',
}
const engineLabel = (e: ConnectionEngine | undefined) => ENGINE_LABELS[e || 'mysql'] || 'MySQL'
const isWarn = (c: Connection) => (c.color || '').toLowerCase() === '#ff5b5b'

// ConnectLanding fills the workspace when no connection is active: pick a
// connection here (grouped + coloured) and clicking one connects directly.
export default function ConnectLanding() {
  const list = useConnections((s) => s.list)
  const load = useConnections((s) => s.load)
  const setActive = useActiveConn((s) => s.setActive)
  const closeAll = useTabs((s) => s.closeAll)
  const groupColors = useGroupColors((s) => s.colors)
  const t = useT()

  useEffect(() => { void load() }, [load])

  const groups = useMemo(() => {
    const m = new Map<string, Connection[]>()
    for (const c of list) {
      const g = (c.group_name || '').trim() || UNGROUPED
      if (!m.has(g)) m.set(g, [])
      m.get(g)!.push(c)
    }
    const keys = [...m.keys()].sort((a, b) => (a === UNGROUPED ? 1 : b === UNGROUPED ? -1 : a.localeCompare(b)))
    return keys.map((k) => ({ key: k, items: m.get(k)! }))
  }, [list])

  function connect(id: number) { closeAll(); setActive(id) }

  if (list.length === 0) {
    return <div style={center}>{t('connections.no_connections')}</div>
  }

  return (
    <div style={wrap}>
      <h2 style={{ margin: '4px 0 14px', fontSize: 18 }}>{t('sidebar.pick_connection')}</h2>
      {groups.map((g) => (
        <div key={g.key} style={{ marginBottom: 18 }}>
          <div style={ghdr}>
            <span style={{ ...gdot, background: groupColors[g.key] || groupColorOf(g.items) }} />
            <span style={gtitle}>{g.key === UNGROUPED ? (t('connections.ungrouped') || 'Ungrouped') : g.key}</span>
            <span style={gcount}>{g.items.length}</span>
          </div>
          <div style={grid}>
            {g.items.map((c) => (
              <div key={c.id} style={card} onClick={() => connect(c.id)} title="connect">
                <div style={cardTop}>
                  <span style={{ ...cdot, background: c.color || 'var(--accent)' }} />
                  <span style={cname}>{c.name}</span>
                  {isWarn(c) && <span title="warning" style={{ color: '#ff5b5b' }}>⚠</span>}
                </div>
                <div style={meta}>{engineLabel(c.engine)} · {c.host}:{c.port}</div>
                {c.default_db && <div style={meta}>db: {c.default_db}</div>}
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function groupColorOf(items: Connection[]): string {
  return items.find((x) => x.color)?.color || 'var(--text-muted)'
}

const wrap: CSSProperties = {
  height: '100%', overflowY: 'auto', padding: 28, maxWidth: 900, margin: '0 auto', width: '100%',
  fontFamily: 'system-ui', color: 'var(--text-primary)',
}
const center: CSSProperties = {
  display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%',
  color: 'var(--text-muted)', fontFamily: 'system-ui',
}
const ghdr: CSSProperties = { display: 'flex', alignItems: 'center', gap: 8, margin: '0 0 8px' }
const gdot: CSSProperties = { width: 10, height: 10, borderRadius: 3, flexShrink: 0 }
const gtitle: CSSProperties = { fontSize: 13, fontWeight: 800, letterSpacing: 0.6, textTransform: 'uppercase', color: 'var(--text-primary)' }
const gcount: CSSProperties = { fontSize: 11, color: 'var(--text-muted)', background: 'var(--bg-elevated)', padding: '1px 8px', borderRadius: 12 }
const grid: CSSProperties = { display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: 12 }
const card: CSSProperties = {
  background: 'var(--bg-elevated)', border: '1px solid var(--border-color)', borderRadius: 10,
  padding: '12px 14px', cursor: 'pointer', display: 'flex', flexDirection: 'column', gap: 4,
}
const cardTop: CSSProperties = { display: 'flex', alignItems: 'center', gap: 8, fontWeight: 600, fontSize: 15 }
const cdot: CSSProperties = { width: 10, height: 10, borderRadius: 3, flexShrink: 0 }
const cname: CSSProperties = { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1, minWidth: 0 }
const meta: CSSProperties = { fontSize: 12, color: 'var(--text-muted)', fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }
