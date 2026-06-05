import { FormEvent, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useChat } from '../store/chat'
import { useActiveConn } from '../store/activeConn'
import { chatStream } from '../lib/chatWs'
import { useT } from '../i18n'
import WriteProposalCard from './WriteProposalCard'

interface Props {
  database?: string
}

interface ProposalState {
  proposalId: string
  db: string
  table: string
  op: string
  sql: string
  explainSummary: string
  status: 'proposed' | 'executing' | 'executed' | 'failed' | 'cancelled'
  rowsAffected?: number
  errorMessage?: string
  toolUseId?: string
}

export default function ChatPanel({ database }: Props) {
  const t = useT()
  const connId = useActiveConn((s) => s.activeId)
  const messages = useChat((s) => s.messages)
  const busy = useChat((s) => s.busy)
  const error = useChat((s) => s.error)
  const pushUser = useChat((s) => s.pushUser)
  const appendText = useChat((s) => s.appendText)
  const addToolCall = useChat((s) => s.addToolCall)
  const setToolOutput = useChat((s) => s.setToolOutput)
  const reset = useChat((s) => s.reset)
  const setBusy = useChat((s) => s.setBusy)
  const setError = useChat((s) => s.setError)
  const [input, setInput] = useState('')
  const [provider, setProvider] = useState<string>(() => localStorage.getItem('dataseai.chat.provider') ?? '')
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const cancelRef = useRef<(() => void) | null>(null)
  const sendRef = useRef<((p: any) => void) | null>(null)
  const lastProposeToolUseRef = useRef<string | null>(null)
  const [proposals, setProposals] = useState<ProposalState[]>([])

  useEffect(() => {
    localStorage.setItem('dataseai.chat.provider', provider)
  }, [provider])

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [messages])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!input.trim() || connId == null) return
    const text = input.trim()
    setInput('')
    pushUser(text)
    setBusy(true)
    setError(null)
    // Build the messages payload from store state. Tool results were already
    // captured at the previous turn, so we send the full transcript here.
    const transcript: any[] = []
    const synthUserMsg = { role: 'user' as const, blocks: [{ type: 'text' as const, text }] }
    for (const m of [...messages, synthUserMsg]) {
      if (m.role === 'user') {
        const userText = m.blocks.filter((b) => b.type === 'text').map((b: any) => b.text).join('')
        transcript.push({ role: 'user', content: [{ type: 'text', text: userText }] })
        continue
      }
      // Assistant: walk blocks in order, emit text + tool_use into the
      // assistant message; collect tool_results in a following tool message.
      const content: any[] = []
      const toolResults: any[] = []
      for (const b of m.blocks) {
        if (b.type === 'text') {
          if (b.text) content.push({ type: 'text', text: b.text })
        } else if (b.type === 'tool_call') {
          content.push({ type: 'tool_use', id: b.id, name: b.name, input: b.input })
          if (b.output !== undefined) {
            toolResults.push({ type: 'tool_result', tool_use_id: b.id, output: b.output })
          }
        }
      }
      if (content.length) transcript.push({ role: 'assistant', content })
      if (toolResults.length) transcript.push({ role: 'tool', content: toolResults })
    }
    const token = localStorage.getItem('dataseai.token') ?? ''
    const s = chatStream({
      token,
      connId,
      db: database ?? '',
      provider,
      messages: transcript,
      onEvent: (ev) => {
        if (ev.type === 'text') appendText(ev.text ?? '')
        else if (ev.type === 'tool_use') {
          addToolCall({ id: ev.tool_use_id!, name: ev.tool_name ?? '', input: ev.tool_input })
          if (ev.tool_name === 'propose_write') {
            lastProposeToolUseRef.current = ev.tool_use_id ?? null
          }
        }
        else if (ev.type === 'tool_result') {
          setToolOutput(ev.tool_use_id!, ev.output ?? '')
          try {
            const obj = JSON.parse(ev.output ?? '')
            if (obj && (obj.status || obj.error)) {
              setProposals((ps) => ps.map((p) => {
                if (p.toolUseId !== ev.tool_use_id) return p
                if (obj.status === 'executed') return { ...p, status: 'executed', rowsAffected: obj.rows_affected }
                if (obj.status === 'cancelled') return { ...p, status: 'cancelled' }
                if (obj.status === 'failed' || obj.error) return { ...p, status: 'failed', errorMessage: obj.error ?? '' }
                return p
              }))
            }
          } catch {/* not a JSON output */}
        }
        else if (ev.type === 'write_proposed') {
          const tuid = lastProposeToolUseRef.current ?? undefined
          setProposals((ps) => [...ps, {
            proposalId: ev.proposal_id!,
            db: ev.database ?? '',
            table: ev.table ?? '',
            op: ev.operation ?? '',
            sql: ev.sql ?? '',
            explainSummary: ev.explain_summary ?? '',
            status: 'proposed',
            toolUseId: tuid,
          }])
          lastProposeToolUseRef.current = null
        }
        else if (ev.type === 'error') setError(ev.message ?? t('common.error'))
      },
      onClose: () => {
        setBusy(false)
        cancelRef.current = null
      },
    })
    cancelRef.current = s.cancel
    sendRef.current = s.send
  }

  function decideProposal(proposalId: string, accept: boolean) {
    setProposals((ps) => ps.map((p) =>
      p.proposalId !== proposalId ? p :
        { ...p, status: accept ? 'executing' as any : 'cancelled' }
    ))
    sendRef.current?.({ type: 'execute_write', proposal_id: proposalId, accept })
  }

  function handleReset() {
    reset()
    setProposals([])
  }

  return (
    <div style={wrap}>
      <div style={bar}>
        <strong>🤖 {t('chat.title')}</strong>
        {database && <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>db: {database}</span>}
        <span style={{ flex: 1 }} />
        <label style={{ fontSize: 12, color: 'var(--text-muted)' }}>
          {t('chat.model_label')}{' '}
          <select value={provider} onChange={(e) => setProvider(e.target.value)} style={{ fontSize: 12 }}>
            <option value="">{t('chat.model_default')}</option>
            <option value="gemini">{t('chat.model_gemini')}</option>
            <option value="anthropic">{t('chat.model_anthropic')}</option>
            <option value="openai">{t('chat.model_openai')}</option>
            <option value="claudecode">{t('chat.model_claudecode')}</option>
            <option value="codex">{t('chat.model_codex')}</option>
          </select>
        </label>
        <button onClick={handleReset}>{t('chat.clear')}</button>
      </div>
      <div ref={scrollRef} style={msgList}>
        {messages.length === 0 && (
          <div style={{ color: 'var(--text-muted)', padding: 16, textAlign: 'center' }}>
            {t('chat.empty_hint')}
          </div>
        )}
        {messages.map((m, i) => (
          <div key={i} style={{ padding: 8, borderBottom: '1px solid var(--table-border)' }}>
            <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 2 }}>{m.role}</div>
            {m.blocks.map((b, bi) => {
              if (b.type === 'text') {
                if (!b.text) return null
                return m.role === 'assistant'
                  ? <div key={bi} className="dataseai-md" style={mdWrap}><ReactMarkdown remarkPlugins={[remarkGfm]}>{b.text}</ReactMarkdown></div>
                  : <div key={bi} style={{ whiteSpace: 'pre-wrap', fontSize: 14 }}>{b.text}</div>
              }
              return (
                <details key={bi} style={{ marginTop: 6, background: 'var(--bg-secondary)', borderRadius: 4, padding: 4 }}>
                  <summary style={{ fontSize: 12 }}>🔧 {b.name}({JSON.stringify(b.input)})</summary>
                  <pre style={{ fontSize: 11, margin: 4, whiteSpace: 'pre-wrap', maxHeight: 240, overflow: 'auto' }}>{b.output ?? '(pending…)'}</pre>
                </details>
              )
            })}
          </div>
        ))}
        {proposals.map((p) => (
          <WriteProposalCard
            key={p.proposalId}
            proposalId={p.proposalId}
            db={p.db}
            table={p.table}
            op={p.op}
            sql={p.sql}
            explainSummary={p.explainSummary}
            status={p.status}
            rowsAffected={p.rowsAffected}
            errorMessage={p.errorMessage}
            onDecision={decideProposal}
          />
        ))}
        {busy && <div style={{ padding: 8, color: 'var(--text-muted)', fontSize: 13 }}>{t('common.thinking')}</div>}
        {error && <div style={{ padding: 8, color: 'var(--danger)', fontSize: 13 }}>{error}</div>}
      </div>
      <form onSubmit={submit} style={form}>
        <input
          autoFocus
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={connId == null ? t('chat.placeholder_no_conn') : t('chat.placeholder')}
          disabled={connId == null || busy}
          style={{ flex: 1, padding: '6px 8px' }}
        />
        <button disabled={connId == null || busy || !input.trim()}>{t('chat.send')}</button>
      </form>
    </div>
  )
}

const wrap: CSSProperties = {
  display: 'flex', flexDirection: 'column', height: '100%', fontFamily: 'system-ui',
  background: 'var(--bg-primary)', color: 'var(--text-primary)',
}
const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, padding: 6,
  borderBottom: '1px solid var(--border-color)', background: 'var(--bg-secondary)',
}
const msgList: CSSProperties = { flex: 1, overflow: 'auto' }
const mdWrap: CSSProperties = { fontSize: 14, lineHeight: 1.55 }
const form: CSSProperties = {
  display: 'flex', gap: 8, padding: 8, borderTop: '1px solid var(--border-color)',
  background: 'var(--bg-secondary)',
}
