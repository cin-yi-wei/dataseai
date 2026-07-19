import { api } from './api'

export interface Conversation {
  id: number
  name: string
  conn_id: number
  db_name: string
  updated_at: number
}

export interface StoredMessage {
  role: string
  blocks: any[]
}

export const chatConvApi = {
  list: (connId: number, db: string) =>
    api
      .get<{ conversations: Conversation[] }>(
        `/api/chat/conversations?conn_id=${connId}&db=${encodeURIComponent(db)}`,
      )
      .then((r) => r.conversations ?? []),
  create: (connId: number, db: string, name: string) =>
    api
      .post<{ conversation: Conversation }>('/api/chat/conversations', { conn_id: connId, db, name })
      .then((r) => r.conversation),
  rename: (id: number, name: string) => api.put(`/api/chat/conversations/${id}`, { name }),
  del: (id: number) => api.del(`/api/chat/conversations/${id}`),
  getMessages: (id: number) =>
    api
      .get<{ messages: StoredMessage[] }>(`/api/chat/conversations/${id}/messages`)
      .then((r) => r.messages ?? []),
  saveMessages: (id: number, messages: StoredMessage[]) =>
    api.put(`/api/chat/conversations/${id}/messages`, { messages }),
}

// Default name for a new room: 未命名-1, 未命名-2, … avoiding existing names.
export function nextUntitledName(existing: Conversation[]): string {
  let n = 1
  const names = new Set(existing.map((c) => c.name))
  while (names.has(`未命名-${n}`)) n++
  return `未命名-${n}`
}
