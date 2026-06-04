import { useState } from 'react'

interface QuickLookEditorModalProps {
  value: any
  columnName: string
  onApply: (newValue: string) => Promise<void>
  onCancel: () => void
}

export function QuickLookEditorModal({ value, onApply, onCancel }: QuickLookEditorModalProps) {
  const [jsonText, setJsonText] = useState(() => {
    if (typeof value === 'string') {
      try {
        return JSON.stringify(JSON.parse(value), null, 2)
      } catch {
        return value
      }
    }
    return JSON.stringify(value, null, 2)
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleMinifyAndApply = async () => {
    try {
      setError(null)
      JSON.parse(jsonText) // Validate
      const minified = JSON.stringify(JSON.parse(jsonText))
      setLoading(true)
      await onApply(minified)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Invalid JSON')
      setLoading(false)
    }
  }

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(jsonText)
    } catch {
      window.alert('Failed to copy')
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
          maxWidth: '90vw',
          maxHeight: '90vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
          <h2 style={{ margin: 0, marginRight: 16, fontSize: 16 }}>Quick Look Editor</h2>
          <select
            defaultValue="JSON"
            style={{
              padding: '6px 8px',
              fontSize: 13,
              border: '1px solid #ccc',
              borderRadius: 4,
            }}
          >
            <option value="JSON">JSON</option>
          </select>
        </div>

        {error && <div style={{ color: 'crimson', marginBottom: 12, fontSize: 13 }}>{error}</div>}

        <textarea
          value={jsonText}
          onChange={(e) => setJsonText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onCancel()
          }}
          style={{
            flex: 1,
            minHeight: 400,
            padding: 12,
            fontFamily: 'monospace',
            fontSize: 12,
            border: '1px solid #ccc',
            borderRadius: 4,
            resize: 'none',
            marginBottom: 16,
          }}
        />

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button onClick={onCancel} style={{ padding: '6px 12px' }}>
            Cancel
          </button>
          <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
            Copy
          </button>
          <button
            onClick={handleMinifyAndApply}
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
            {loading ? 'Saving...' : 'Minify & Apply'}
          </button>
        </div>
      </div>
    </div>
  )
}
