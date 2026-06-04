import { useState } from 'react'
import { JsonTreeEditor } from './JsonTreeEditor'

interface QuickLookEditorModalProps {
  value: any
  columnName: string
  onApply: (newValue: string) => Promise<void>
  onCancel: () => void
}

export function QuickLookEditorModal({ value, columnName, onApply, onCancel }: QuickLookEditorModalProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isJsonType = typeof value === 'string' && value.trim().startsWith('{') && value.trim().endsWith('}')

  const handleApply = async (newValue: string) => {
    setLoading(true)
    setError(null)
    try {
      await onApply(newValue)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      setLoading(false)
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
          color: 'var(--text-primary)',
          borderRadius: 8,
          padding: 24,
          maxWidth: '90vw',
          width: 800,
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
          border: '1px solid var(--border-color)',
        }}
      >
        <h2 style={{ marginTop: 0, marginBottom: 16, fontSize: 16 }}>
          Quick Look {columnName} {isJsonType ? '(JSON)' : ''}
        </h2>

        {error && <div style={{ color: 'var(--danger)', marginBottom: 12, fontSize: 13 }}>{error}</div>}

        <div style={{ flex: 1, overflow: 'hidden', marginBottom: 16 }}>
          <JsonTreeEditor initialValue={value} rootName={columnName} onApply={handleApply} onCancel={onCancel} />
        </div>

        {loading && <div style={{ textAlign: 'center', color: 'var(--text-muted)' }}>Saving...</div>}
      </div>
    </div>
  )
}
