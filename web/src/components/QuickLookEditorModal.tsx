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
        style={{
          backgroundColor: 'white',
          borderRadius: 8,
          padding: 24,
          maxWidth: '80vw',
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
        }}
      >
        <h2 style={{ marginTop: 0, marginBottom: 16, fontSize: 16 }}>
          Quick Look {columnName} {isJsonType ? '(JSON)' : ''}
        </h2>

        {error && <div style={{ color: 'crimson', marginBottom: 12, fontSize: 13 }}>{error}</div>}

        <div style={{ flex: 1, overflow: 'hidden', marginBottom: 16 }}>
          <JsonTreeEditor initialValue={value} onApply={handleApply} onCancel={onCancel} />
        </div>

        {loading && <div style={{ textAlign: 'center', color: '#999' }}>Saving...</div>}
      </div>
    </div>
  )
}
