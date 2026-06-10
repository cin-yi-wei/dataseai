const KEY = 'dataseai.pinnedTables'

type Store = Record<string, string[]>

function load(): Store {
  try {
    const raw = localStorage.getItem(KEY)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

function save(s: Store) {
  try {
    localStorage.setItem(KEY, JSON.stringify(s))
  } catch {
    // ignore quota errors
  }
}

function scopeKey(connId: number, db: string): string {
  return `${connId}::${db}`
}

export function getPinned(connId: number, db: string): Set<string> {
  const s = load()
  return new Set(s[scopeKey(connId, db)] ?? [])
}

export function togglePinned(connId: number, db: string, table: string): Set<string> {
  const s = load()
  const k = scopeKey(connId, db)
  const cur = new Set(s[k] ?? [])
  if (cur.has(table)) cur.delete(table)
  else cur.add(table)
  s[k] = Array.from(cur)
  save(s)
  return cur
}
