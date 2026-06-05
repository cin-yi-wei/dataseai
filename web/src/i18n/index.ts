import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { messages, type Lang, type MessageKey } from './messages'

interface State {
  lang: Lang
  setLang: (l: Lang) => void
}

function detectInitialLang(): Lang {
  if (typeof navigator !== 'undefined') {
    const n = (navigator.language || '').toLowerCase()
    if (n.startsWith('zh')) return 'zh-TW'
  }
  return 'en'
}

export const useLang = create<State>()(
  persist(
    (set) => ({
      lang: detectInitialLang(),
      setLang: (l) => set({ lang: l }),
    }),
    { name: 'dataseai-lang' },
  ),
)

function format(template: string, params?: Record<string, string | number>): string {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (_, k) => {
    const v = params[k]
    return v === undefined ? `{${k}}` : String(v)
  })
}

export function useT() {
  const lang = useLang((s) => s.lang)
  return (key: MessageKey, params?: Record<string, string | number>): string => {
    const dict = messages[lang] || messages.en
    const raw = dict[key] ?? messages.en[key] ?? key
    return format(raw, params)
  }
}

export const LANGS: Lang[] = ['en', 'zh-TW']
export type { Lang, MessageKey }
