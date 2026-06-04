import { useState, useEffect } from 'react'

interface SimpleCellEditorProps {
  initialValue: any
  onApply: (newValue: string) => void
  onCancel: () => void
}

export function SimpleCellEditor({ initialValue, onApply, onCancel }: SimpleCellEditorProps) {
  const [value, setValue] = useState(initialValue == null ? '' : String(initialValue))

  useEffect(() => {
    // Auto-focus input
    const input = document.querySelector('[data-simple-editor-input]') as HTMLTextAreaElement
    if (input) {
      input.focus()
      input.setSelectionRange(value.length, value.length)
    }
  }, [])

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      window.alert('Failed to copy')
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <textarea
        data-simple-editor-input
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
            onApply(value)
          }
          if (e.key === 'Escape') onCancel()
        }}
        style={{
          flex: 1,
          minHeight: 200,
          padding: 8,
          fontFamily: 'monospace',
          fontSize: 12,
          border: '1px solid #ccc',
          borderRadius: 4,
          resize: 'vertical',
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
          onClick={() => onApply(value)}
          style={{ padding: '6px 12px', backgroundColor: '#0066cc', color: 'white', border: 'none', borderRadius: 4 }}
        >
          Apply
        </button>
      </div>
    </div>
  )
}
