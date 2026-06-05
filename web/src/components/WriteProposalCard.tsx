import type { CSSProperties, ReactNode } from 'react'
import { useT } from '../i18n'

export type ProposalStatus = 'proposed' | 'executing' | 'executed' | 'failed' | 'cancelled'

interface Props {
  proposalId: string
  db: string
  table: string
  op: string
  sql: string
  explainSummary: string
  status: ProposalStatus
  rowsAffected?: number
  errorMessage?: string
  onDecision: (proposalId: string, accept: boolean) => void
}

export default function WriteProposalCard(p: Props) {
  const t = useT()
  let explainNode: ReactNode = null
  if (p.explainSummary) {
    try {
      const obj = JSON.parse(p.explainSummary)
      if (obj.error) {
        explainNode = <p style={warn}>{t('chat.proposal.explain_failed', { error: obj.error })}</p>
      } else if (Array.isArray(obj.rows) && obj.rows.length) {
        explainNode = (
          <pre style={pre}>{t('chat.proposal.explain')}: {JSON.stringify(obj.rows, null, 2)}</pre>
        )
      }
    } catch {
      // ignore parse error — silently skip explain display
    }
  }

  const isDone = p.status !== 'proposed'
  return (
    <div style={card}>
      <header style={title}>
        {t('chat.proposal.title', { op: p.op, db: p.db, table: p.table })}
      </header>
      <pre style={pre}>{p.sql}</pre>
      {explainNode}
      {!isDone && (
        <div style={actions}>
          <button onClick={() => p.onDecision(p.proposalId, true)}>{t('chat.proposal.execute')}</button>
          <button onClick={() => p.onDecision(p.proposalId, false)}>{t('chat.proposal.cancel')}</button>
        </div>
      )}
      {p.status === 'executing' && <p style={muted}>… executing</p>}
      {p.status === 'executed'  && <p style={ok}>{t('chat.proposal.executed', { rows: p.rowsAffected ?? 0 })}</p>}
      {p.status === 'failed'    && <p style={bad}>{t('chat.proposal.failed', { error: p.errorMessage ?? '' })}</p>}
      {p.status === 'cancelled' && <p style={muted}>{t('chat.proposal.cancelled')}</p>}
    </div>
  )
}

const card: CSSProperties = {
  background: 'var(--bg-warning)',
  color: 'var(--fg-warning)',
  border: '1px solid var(--border-warning)',
  borderRadius: 6,
  padding: 10,
  margin: '6px 0',
}
const title: CSSProperties = { fontWeight: 600, marginBottom: 6 }
const pre: CSSProperties = { background: 'rgba(0,0,0,0.25)', padding: 8, borderRadius: 4, fontFamily: 'monospace', fontSize: 12, overflowX: 'auto', margin: '4px 0' }
const actions: CSSProperties = { display: 'flex', gap: 8, marginTop: 6 }
const ok: CSSProperties = { color: '#3a8', margin: 0, fontWeight: 600 }
const bad: CSSProperties = { color: '#c44', margin: 0, fontWeight: 600 }
const muted: CSSProperties = { color: 'var(--text-muted, #888)', margin: 0 }
const warn: CSSProperties = { color: '#e90', fontSize: 12, margin: 0 }
