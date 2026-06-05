import { useState } from 'react'
import { useT } from '../i18n'

interface EditCellModalProps {
  value: any
  columnName: string
  columnType?: string
  onApply: (newValue: string) => Promise<void>
  onCancel: () => void
}

type Format = 'JSON' | 'Text'

function detectFormat(value: any): Format {
  if (value === null || value === undefined) return 'Text'
  if (typeof value !== 'string') return 'Text'
  const trimmed = value.trim()
  if (
    (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
    (trimmed.startsWith('[') && trimmed.endsWith(']'))
  ) {
    try {
      JSON.parse(trimmed)
      return 'JSON'
    } catch {
      return 'Text'
    }
  }
  return 'Text'
}

export function EditCellModal({ value, columnName, columnType, onApply, onCancel }: EditCellModalProps) {
  const t = useT()
  const detectedFormat = detectFormat(value)
  const format = detectedFormat
  const [text, setText] = useState(() => {
    if (value === null || value === undefined) return ''
    if (typeof value === 'string') {
      if (detectedFormat === 'JSON') {
        try {
          return JSON.stringify(JSON.parse(value), null, 2)
        } catch {
          return value
        }
      }
      return value
    }
    return String(value)
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleApply = async () => {
    try {
      setError(null)
      setLoading(true)
      if (format === 'JSON') {
        // Validate and minify JSON
        const parsed = JSON.parse(text)
        await onApply(JSON.stringify(parsed))
      } else {
        await onApply(text)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('edit.operation_failed'))
      setLoading(false)
    }
  }

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      window.alert(t('edit.failed_to_copy'))
    }
  }

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
      }}
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div
        data-modal
        style={{
          backgroundColor: 'var(--bg-primary)',
          borderRadius: 8,
          padding: 24,
          width: 'min(900px, 92vw)',
          height: 'min(720px, 88vh)',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
          <h2 style={{ margin: 0, marginRight: 12, fontSize: 16 }}>{t('edit.title', { column: columnName })}</h2>
          {columnType && (
            <span style={{
              padding: '4px 10px',
              fontSize: 12,
              border: '1px solid #ccc',
              borderRadius: 4,
              backgroundColor: '#f5f5f5',
              color: '#555',
              fontFamily: 'monospace',
            }}>
              {columnType}
            </span>
          )}
        </div>

        {error && <div style={{ color: 'crimson', marginBottom: 12, fontSize: 13 }}>{error}</div>}

        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onCancel()
          }}
          style={{
            flex: 1,
            width: '100%',
            minHeight: 0,
            padding: 12,
            fontFamily: 'monospace',
            fontSize: 13,
            border: '1px solid var(--border-color)',
            borderRadius: 4,
            resize: 'none',
            marginBottom: 16,
            boxSizing: 'border-box',
            background: 'var(--bg-elevated, transparent)',
            color: 'var(--text-primary)',
          }}
        />

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button onClick={onCancel} style={{ padding: '6px 12px' }}>
            {t('common.cancel')}
          </button>
          <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
            {t('common.copy')}
          </button>
          <button
            onClick={handleApply}
            disabled={loading}
            style={{
              padding: '6px 12px',
              backgroundColor: '#0066cc',
              color: 'white',
              border: 'none',
              borderRadius: 4,
              cursor: loading ? 'not-allowed' : 'pointer',
              opacity: loading ? 0.6 : 1,
            }}
          >
            {loading ? t('common.saving') : format === 'JSON' ? t('edit.minify_apply') : t('common.apply')}
          </button>
        </div>
      </div>
    </div>
  )
}
