import { useState } from 'react'
import type { CSSProperties } from 'react'
import { useT } from '../i18n'

export interface AIPolicy {
  insert: boolean
  update: boolean
  delete: boolean
  ddl: boolean
}

export interface TablePolicy {
  table: string
  policy: AIPolicy
}

interface Props {
  connId: number
  db: string
  configured: TablePolicy[]
  unconfigured: string[]
  onUpsert: (connId: number, db: string, table: string, policy: AIPolicy) => void
  onBatch: (connId: number, db: string, tables: string[], policy: AIPolicy) => void
}

const FLAGS = ['insert', 'update', 'delete', 'ddl'] as const
type Flag = (typeof FLAGS)[number]

const FLAG_TO_COL: Record<Flag, 'ins' | 'upd' | 'del' | 'ddl'> = {
  insert: 'ins', update: 'upd', delete: 'del', ddl: 'ddl',
}

export default function AIWritePolicyTable({ connId, db, configured, unconfigured, onUpsert, onBatch }: Props) {
  const t = useT()
  return (
    <div>
      <h4>{t('settings.ai_writes.configured', { count: configured.length })}</h4>
      <table style={tbl}>
        <thead>
          <tr>
            <th style={th}>table</th>
            <th style={th}>{t('settings.ai_writes.col.ins')}</th>
            <th style={th}>{t('settings.ai_writes.col.upd')}</th>
            <th style={th}>{t('settings.ai_writes.col.del')}</th>
            <th style={th}>{t('settings.ai_writes.col.ddl')}</th>
            <th style={th}>{t('settings.ai_writes.col.select_all')}</th>
          </tr>
        </thead>
        <tbody>
          {configured.map((row) => (
            <ConfiguredRow
              key={row.table}
              row={row}
              onChange={(p) => onUpsert(connId, db, row.table, p)}
            />
          ))}
        </tbody>
      </table>

      <h4>{t('settings.ai_writes.unconfigured', { count: unconfigured.length })}</h4>
      <UnconfiguredBatch
        connId={connId}
        db={db}
        tables={unconfigured}
        onBatch={onBatch}
      />
    </div>
  )
}

function ConfiguredRow({ row, onChange }: { row: TablePolicy; onChange: (p: AIPolicy) => void }) {
  const set = (flag: Flag, value: boolean) => {
    onChange({ ...row.policy, [flag]: value })
  }
  const allOn = FLAGS.every((f) => row.policy[f])
  const toggleAll = () => {
    const v = !allOn
    onChange({ insert: v, update: v, delete: v, ddl: v })
  }
  return (
    <tr>
      <td style={td}>{row.table}</td>
      {FLAGS.map((f) => (
        <td key={f} style={td}>
          <input
            type="checkbox"
            checked={row.policy[f]}
            onChange={(e) => set(f, e.target.checked)}
            aria-label={`${row.table} ${f}`}
          />
        </td>
      ))}
      <td style={td}>
        <input
          type="checkbox"
          checked={allOn}
          onChange={toggleAll}
          data-testid="select-all"
          aria-label={`${row.table} select-all`}
        />
      </td>
    </tr>
  )
}

function UnconfiguredBatch({ connId, db, tables, onBatch }: { connId: number; db: string; tables: string[]; onBatch: Props['onBatch'] }) {
  const t = useT()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [draft, setDraft] = useState<AIPolicy>({ insert: false, update: false, delete: false, ddl: false })

  const flip = (table: string) => {
    setSelected((s) => {
      const next = new Set(s)
      if (next.has(table)) next.delete(table); else next.add(table)
      return next
    })
  }

  return (
    <div>
      <div style={batchBar}>
        {FLAGS.map((f) => (
          <label key={f} style={{ marginRight: 8 }}>
            <input
              type="checkbox"
              checked={draft[f]}
              data-testid={`batch-${FLAG_TO_COL[f]}`}
              onChange={(e) => setDraft((d) => ({ ...d, [f]: e.target.checked }))}
            />
            {t(`settings.ai_writes.col.${FLAG_TO_COL[f]}` as const)}
          </label>
        ))}
        <button
          disabled={selected.size === 0}
          onClick={() => onBatch(connId, db, Array.from(selected), draft)}
        >
          {t('settings.ai_writes.batch_apply', { n: selected.size })}
        </button>
      </div>
      <table style={tbl}>
        <tbody>
          {tables.map((tname) => (
            <tr key={tname}>
              <td style={td}>
                <input
                  type="checkbox"
                  checked={selected.has(tname)}
                  onChange={() => flip(tname)}
                  aria-label={`select ${tname}`}
                />
              </td>
              <td style={td}>{tname}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

const tbl: CSSProperties = { width: '100%', borderCollapse: 'collapse', fontSize: 13 }
const th: CSSProperties = { textAlign: 'left', padding: '4px 8px', borderBottom: '1px solid var(--border-color)' }
const td: CSSProperties = { padding: '4px 8px', borderBottom: '1px solid var(--table-border, var(--border-color))' }
const batchBar: CSSProperties = { padding: '4px 0', display: 'flex', alignItems: 'center', gap: 8 }
