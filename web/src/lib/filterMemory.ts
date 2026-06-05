import type { Filter } from '../components/FilterBar'

const PREFIX = 'dataseai.filters'
const HISTORY_MAX = 5

function keyCurrent(connId: number, db: string, table: string): string {
  return `${PREFIX}.current.${connId}.${db}.${table}`
}

function keyHistory(connId: number, db: string, table: string): string {
  return `${PREFIX}.history.${connId}.${db}.${table}`
}

function safeParse<T>(s: string | null): T | null {
  if (!s) return null
  try {
    return JSON.parse(s) as T
  } catch {
    return null
  }
}

export function getCurrentFilters(connId: number, db: string, table: string): Filter[] | null {
  return safeParse<Filter[]>(localStorage.getItem(keyCurrent(connId, db, table)))
}

export function setCurrentFilters(connId: number, db: string, table: string, filters: Filter[]): void {
  if (filters.length === 0) {
    localStorage.removeItem(keyCurrent(connId, db, table))
    return
  }
  localStorage.setItem(keyCurrent(connId, db, table), JSON.stringify(filters))
}

export function getHistory(connId: number, db: string, table: string): Filter[][] {
  return safeParse<Filter[][]>(localStorage.getItem(keyHistory(connId, db, table))) ?? []
}

function filtersEqual(a: Filter[], b: Filter[]): boolean {
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i].column !== b[i].column ||
        a[i].operator !== b[i].operator ||
        a[i].value !== b[i].value ||
        a[i].enabled !== b[i].enabled) {
      return false
    }
  }
  return true
}

export function pushHistory(connId: number, db: string, table: string, filters: Filter[]): void {
  if (filters.length === 0) return
  const prev = getHistory(connId, db, table)
  const deduped = prev.filter((entry) => !filtersEqual(entry, filters))
  const next = [filters, ...deduped].slice(0, HISTORY_MAX)
  localStorage.setItem(keyHistory(connId, db, table), JSON.stringify(next))
}

export function summarizeFilters(filters: Filter[]): string {
  const parts = filters
    .filter((f) => f.enabled)
    .map((f) => {
      if (f.operator === 'IS NULL' || f.operator === 'IS NOT NULL') {
        return `${f.column} ${f.operator}`
      }
      const v = f.value.length > 20 ? f.value.slice(0, 20) + '…' : f.value
      return `${f.column} ${f.operator} ${v}`
    })
  if (parts.length === 0) return '(empty)'
  if (parts.length <= 2) return parts.join(' · ')
  return `${parts.slice(0, 2).join(' · ')} · +${parts.length - 2}`
}
