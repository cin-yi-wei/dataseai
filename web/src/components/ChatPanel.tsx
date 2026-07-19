import { FormEvent, useEffect, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useChat } from '../store/chat'
import { useActiveConn } from '../store/activeConn'
import { chatStream } from '../lib/chatWs'
import { chatConvApi, nextUntitledName, type Conversation } from '../lib/chatConv'
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
  const setActiveDB = useActiveConn((s) => s.setActiveDB)
  const messages = useChat((s) => s.messages)
  const busy = useChat((s) => s.busy)
  const error = useChat((s) => s.error)
  const pushUser = useChat((s) => s.pushUser)
  const appendText = useChat((s) => s.appendText)
  const addToolCall = useChat((s) => s.addToolCall)
  const setToolOutput = useChat((s) => s.setToolOutput)
  const setMessages = useChat((s) => s.setMessages)
  const convId = useChat((s) => s.convId)
  const setConvId = useChat((s) => s.setConvId)
  const reset = useChat((s) => s.reset)
  const setBusy = useChat((s) => s.setBusy)
  const setError = useChat((s) => s.setError)
  const [conversations, setConversations] = useState<Conversation[]>([])
  // Mobile master-detail: 'list' = conversation directory, 'chat' = the room.
  const [view, setView] = useState<'list' | 'chat'>('list')
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(max-width: 768px)').matches,
  )
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 768px)')
    const on = () => setIsMobile(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
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

  const scopeDb = database ?? ''

  // Load this (connection, db) scope's conversations; open the most recent.
  useEffect(() => {
    if (connId == null) { setConversations([]); setConvId(null); reset(); return }
    let cancelled = false
    chatConvApi.list(connId, scopeDb).then((list) => {
      if (cancelled) return
      setConversations(list)
      if (list.length > 0) void loadConv(list[0].id)
      else { setConvId(null); reset() }
    }).catch(() => {})
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connId, scopeDb])

  async function loadConv(id: number) {
    try {
      const stored = await chatConvApi.getMessages(id)
      setConvId(id)
      setMessages(stored.map((m) => ({ role: m.role as any, blocks: (m.blocks ?? []) as any })))
      setProposals([])
      setError(null)
    } catch { /* ignore */ }
  }

  // Select a room from the list; on mobile also switch to the chat page.
  function selectConv(id: number) {
    void loadConv(id)
    if (isMobile) setView('chat')
  }

  async function newConv() {
    if (connId == null) return
    try {
      const c = await chatConvApi.create(connId, scopeDb, nextUntitledName(conversations))
      setConversations((cs) => [c, ...cs])
      setConvId(c.id)
      reset()
      setProposals([])
      if (isMobile) setView('chat')
    } catch { /* ignore */ }
  }

  async function renameConv(id: number) {
    const cur = conversations.find((c) => c.id === id)
    const name = window.prompt(t('chat.rename_prompt'), cur?.name ?? '')
    const trimmed = name?.trim()
    if (!trimmed) return
    try {
      await chatConvApi.rename(id, trimmed)
      setConversations((cs) => cs.map((c) => (c.id === id ? { ...c, name: trimmed } : c)))
    } catch { /* ignore */ }
  }

  async function deleteConv(id: number) {
    if (!window.confirm(t('chat.delete_confirm'))) return
    try {
      await chatConvApi.del(id)
      const remaining = conversations.filter((c) => c.id !== id)
      setConversations(remaining)
      if (id === convId) {
        if (remaining.length > 0) void loadConv(remaining[0].id)
        else { setConvId(null); reset() }
      }
    } catch { /* ignore */ }
  }

  // Persist the current transcript to a conversation.
  function saveCurrent(id: number | null) {
    if (id == null) return
    const msgs = useChat.getState().messages.map((m) => ({ role: m.role, blocks: m.blocks as any[] }))
    void chatConvApi.saveMessages(id, msgs).catch(() => {})
  }

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight })
  }, [messages])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!input.trim() || connId == null) return
    const text = input.trim()
    setInput('')
    // Ensure a conversation exists so the turn gets persisted.
    let activeConv = convId
    if (activeConv == null) {
      try {
        const c = await chatConvApi.create(connId, scopeDb, nextUntitledName(conversations))
        setConversations((cs) => [c, ...cs])
        setConvId(c.id)
        activeConv = c.id
      } catch { /* keep going without persistence */ }
    }
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
          // Every tool_use MUST be paired with a tool_result or the LLM API
          // rejects the whole transcript. A turn interrupted mid-tool leaves
          // an output-less tool_call; synthesize a placeholder result so the
          // conversation stays valid and later messages don't fail forever.
          toolResults.push({
            type: 'tool_result',
            tool_use_id: b.id,
            output: b.output !== undefined ? b.output : '(no result — previous turn was interrupted)',
          })
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
        // Persist the completed turn.
        saveCurrent(activeConv)
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

  const currentName = conversations.find((c) => c.id === convId)?.name

  return (
    <div style={wrap}>
      {(!isMobile || view === 'list') && (
        <div style={isMobile ? listPaneMobile : listPane}>
          <div style={listHeader}>
            <strong style={{ fontSize: 14 }}>💬 {t('chat.conversations_title')}</strong>
            <span style={{ flex: 1 }} />
            <button onClick={() => void newConv()} disabled={connId == null} style={convBtn} title={t('chat.new_conversation')}>＋ {t('common.add')}</button>
          </div>
          <div style={listScroll}>
            {connId == null && <div style={emptyList}>{t('chat.placeholder_no_conn')}</div>}
            {connId != null && conversations.length === 0 && <div style={emptyList}>{t('chat.no_conversations')}</div>}
            {conversations.map((c) => (
              <div key={c.id} onClick={() => selectConv(c.id)} style={c.id === convId ? convRowActive : convRow}>
                <div style={convAvatar}>💬</div>
                <div style={convRowMain}>
                  <div style={convRowTop}>
                    <span style={convRowName}>{c.name}</span>
                    <span style={convRowTime}>{formatConvTime(c.updated_at)}</span>
                  </div>
                  <div style={convRowPreview}>{c.preview || t('chat.no_messages')}</div>
                </div>
                <div style={convRowActions}>
                  <button onClick={(e) => { e.stopPropagation(); void renameConv(c.id) }} style={rowIconBtn} title={t('chat.rename')}>✎</button>
                  <button onClick={(e) => { e.stopPropagation(); void deleteConv(c.id) }} style={rowIconBtn} title={t('chat.delete')}>🗑</button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      {(!isMobile || view === 'chat') && (
      <div style={chatPane}>
      <div style={bar}>
        {isMobile && <button onClick={() => setView('list')} style={clearBtn}>← {t('chat.back_to_list')}</button>}
        <strong style={titleStyle}>{currentName ? `🤖 ${currentName}` : `🤖 ${t('chat.title')}`}</strong>
        {database && <span style={dbBadge}>db: {database}</span>}
        <span style={{ flex: 1, minWidth: 0 }} />
        <label style={modelLabelStyle}>
          <span style={{ marginRight: 4 }}>{t('chat.model_label')}</span>
          <select value={provider} onChange={(e) => setProvider(e.target.value)} style={modelSelectStyle}>
            <option value="">{t('chat.model_default')}</option>
            <option value="gemini">{t('chat.model_gemini')}</option>
            <option value="anthropic">{t('chat.model_anthropic')}</option>
            <option value="openai">{t('chat.model_openai')}</option>
            <option value="claudecode">{t('chat.model_claudecode')}</option>
            <option value="codex">{t('chat.model_codex')}</option>
          </select>
        </label>
        <button onClick={handleReset} style={clearBtn}>{t('chat.clear')}</button>
      </div>
      <div ref={scrollRef} style={msgList}>
        {messages.length === 0 && (
          <div style={{ color: 'var(--text-muted)', padding: 16, textAlign: 'center' }}>
            {t('chat.empty_hint')}
          </div>
        )}
        {messages.map((m, i) => {
          const isUser = m.role === 'user'
          return (
            <div key={i} style={isUser ? rowUser : rowAsst}>
              {!isUser && <div style={avatarAsst}>🤖</div>}
              <div style={isUser ? bubbleUser : bubbleAsst}>
                {m.blocks.map((b, bi) => {
                  if (b.type === 'text') {
                    if (!b.text) return null
                    return isUser
                      ? <div key={bi} style={{ whiteSpace: 'pre-wrap', fontSize: 14 }}>{b.text}</div>
                      : <div key={bi} className="dataseai-md" style={mdWrap}><ReactMarkdown remarkPlugins={[remarkGfm]}>{b.text}</ReactMarkdown></div>
                  }
                  const picker = renderToolPicker(b.name, b.output, {
                    onPickDB: (db) => {
                      setActiveDB(db)
                      setInput((prev) => prev || `使用 ${db}`)
                    },
                    onPickTable: (name) => setInput((prev) => prev || `看一下 ${name}`),
                  })
                  return (
                    <div key={bi}>
                      <details style={{ marginTop: 6, background: 'var(--bg-primary)', borderRadius: 6, padding: 4 }}>
                        <summary style={{ fontSize: 12 }}>🔧 {b.name}({JSON.stringify(b.input)})</summary>
                        <pre style={{ fontSize: 11, margin: 4, whiteSpace: 'pre-wrap', maxHeight: 240, overflow: 'auto' }}>{b.output ?? '(pending…)'}</pre>
                      </details>
                      {picker}
                    </div>
                  )
                })}
              </div>
              {isUser && <div style={avatarUser}>🙂</div>}
            </div>
          )
        })}
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
        {busy && (
          <div style={rowAsst}>
            <div style={avatarAsst}>🤖</div>
            <div style={{ ...bubbleAsst, color: 'var(--text-muted)' }}>{t('common.thinking')}</div>
          </div>
        )}
        {error && <div style={{ padding: 8, color: 'var(--danger)', fontSize: 13, textAlign: 'center' }}>{error}</div>}
      </div>
      <form onSubmit={submit} style={form}>
        <input
          autoFocus
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={connId == null ? t('chat.placeholder_no_conn') : t('chat.placeholder')}
          disabled={connId == null || busy}
          style={{ flex: 1, padding: '8px 12px', borderRadius: 999, border: '1px solid var(--border-strong)' }}
        />
        <button disabled={connId == null || busy || !input.trim()}>{t('chat.send')}</button>
      </form>
      </div>
      )}
    </div>
  )
}

const wrap: CSSProperties = {
  display: 'flex', flexDirection: 'row', height: '100%', fontFamily: 'system-ui',
  background: 'var(--bg-primary)', color: 'var(--text-primary)', minHeight: 0,
}
// Left conversation-directory pane.
const listPane: CSSProperties = {
  width: 260, flexShrink: 0, display: 'flex', flexDirection: 'column', minHeight: 0,
  borderRight: '1px solid var(--border-color)', background: 'var(--bg-secondary)',
}
const listPaneMobile: CSSProperties = {
  flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', minHeight: 0,
  background: 'var(--bg-secondary)',
}
const listHeader: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 6, padding: '8px 10px',
  borderBottom: '1px solid var(--border-color)',
}
const listScroll: CSSProperties = { flex: 1, overflow: 'auto', minHeight: 0 }
const emptyList: CSSProperties = { padding: 16, color: 'var(--text-muted)', fontSize: 13, textAlign: 'center' }
const convRow: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 10, padding: '10px 12px', cursor: 'pointer',
  borderBottom: '1px solid var(--table-border)', fontSize: 14,
}
const convRowActive: CSSProperties = { ...convRow, background: 'var(--bg-active)' }
const convAvatar: CSSProperties = {
  width: 40, height: 40, borderRadius: '50%', flexShrink: 0, fontSize: 20,
  display: 'flex', alignItems: 'center', justifyContent: 'center',
  background: 'var(--bg-primary)', border: '1px solid var(--border-color)',
}
const convRowMain: CSSProperties = { flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 2 }
const convRowTop: CSSProperties = { display: 'flex', alignItems: 'baseline', gap: 6 }
const convRowName: CSSProperties = { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: 600 }
const convRowTime: CSSProperties = { flexShrink: 0, fontSize: 11, color: 'var(--text-muted)' }
const convRowPreview: CSSProperties = { fontSize: 12, color: 'var(--text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }
const convRowActions: CSSProperties = { display: 'flex', flexDirection: 'column', gap: 2, flexShrink: 0 }
const rowIconBtn: CSSProperties = { fontSize: 11, padding: '1px 4px', background: 'transparent', border: 'none', cursor: 'pointer', opacity: 0.55 }

function formatConvTime(unix: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const now = new Date()
  const pad = (n: number) => n.toString().padStart(2, '0')
  return d.toDateString() === now.toDateString()
    ? `${pad(d.getHours())}:${pad(d.getMinutes())}`
    : `${d.getMonth() + 1}/${d.getDate()}`
}
// Right chat pane.
const chatPane: CSSProperties = {
  flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0,
}
const bar: CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 8, padding: 6, flexWrap: 'wrap',
  borderBottom: '1px solid var(--border-color)', background: 'var(--bg-secondary)',
}
const convBtn: CSSProperties = { fontSize: 13, padding: '3px 8px', flexShrink: 0 }
const titleStyle: CSSProperties = { whiteSpace: 'nowrap', flexShrink: 0, fontSize: 14 }
const dbBadge: CSSProperties = {
  fontSize: 12, color: 'var(--text-muted)', flexShrink: 1, minWidth: 0,
  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
}
const modelLabelStyle: CSSProperties = {
  fontSize: 12, color: 'var(--text-muted)', flexShrink: 0,
  display: 'flex', alignItems: 'center',
}
const modelSelectStyle: CSSProperties = {
  fontSize: 12, maxWidth: 160,
}
const clearBtn: CSSProperties = { whiteSpace: 'nowrap', flexShrink: 0 }
const msgList: CSSProperties = {
  flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 12, padding: 12,
}
const mdWrap: CSSProperties = { fontSize: 14, lineHeight: 1.55 }

// Chat-bubble layout
const rowBase: CSSProperties = { display: 'flex', gap: 8, alignItems: 'flex-start', maxWidth: '100%' }
const rowAsst: CSSProperties = { ...rowBase, justifyContent: 'flex-start' }
const rowUser: CSSProperties = { ...rowBase, justifyContent: 'flex-end' }
const bubbleBase: CSSProperties = {
  maxWidth: '78%', padding: '8px 12px', borderRadius: 14, fontSize: 14, lineHeight: 1.5,
  wordBreak: 'break-word', overflowWrap: 'anywhere',
}
const bubbleAsst: CSSProperties = {
  ...bubbleBase, background: 'var(--bg-secondary)', color: 'var(--text-primary)',
  border: '1px solid var(--border-color)', borderTopLeftRadius: 4,
}
const bubbleUser: CSSProperties = {
  ...bubbleBase, background: 'var(--accent)', color: '#fff', borderTopRightRadius: 4,
}
const avatarBase: CSSProperties = {
  width: 28, height: 28, borderRadius: '50%', flexShrink: 0, display: 'flex',
  alignItems: 'center', justifyContent: 'center', fontSize: 15,
}
const avatarAsst: CSSProperties = { ...avatarBase, background: 'var(--bg-secondary)', border: '1px solid var(--border-color)' }
const avatarUser: CSSProperties = { ...avatarBase, background: 'var(--accent)' }
const form: CSSProperties = {
  display: 'flex', gap: 8, padding: 8, borderTop: '1px solid var(--border-color)',
  background: 'var(--bg-secondary)',
}

const pickerWrap: CSSProperties = {
  display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 6, marginBottom: 6,
}
const pickerChip: CSSProperties = {
  padding: '3px 8px',
  background: 'transparent', color: 'var(--accent, #4a8fd5)',
  border: '1px solid var(--accent, #4a8fd5)', borderRadius: 4,
  fontSize: 12, cursor: 'pointer', fontFamily: 'monospace',
  whiteSpace: 'nowrap',
}

// When the LLM lists databases or tables, render each item as a tap-to-pick
// chip. Picking a database also pins the sidebar's activeDB so the chat
// scope locks to it. Picking a table only pre-fills the input — the user
// still presses Send to confirm what they want to do with the table.
interface PickerCallbacks {
  onPickDB: (db: string) => void
  onPickTable: (name: string) => void
}
function renderToolPicker(name: string, output: string | undefined, cb: PickerCallbacks) {
  if (!output) return null
  let items: string[] = []
  let onPick: (v: string) => void
  try {
    const parsed = JSON.parse(output)
    if (name === 'list_databases' && Array.isArray(parsed?.databases)) {
      items = parsed.databases.filter((d: unknown): d is string => typeof d === 'string')
      onPick = cb.onPickDB
    } else if (name === 'list_tables' && Array.isArray(parsed?.tables)) {
      items = parsed.tables
        .map((t: any) => typeof t === 'string' ? t : t?.name)
        .filter((n: unknown): n is string => typeof n === 'string')
      onPick = cb.onPickTable
    } else {
      return null
    }
  } catch {
    return null
  }
  if (items.length === 0) return null
  return (
    <div style={pickerWrap}>
      {items.map((it) => (
        <button key={it} type="button" onClick={() => onPick(it)} style={pickerChip}>
          {it}
        </button>
      ))}
    </div>
  )
}
