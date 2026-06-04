import { useEffect, useRef } from 'react'

interface CopyTextModalProps {
  text: string
  title: string
  onCancel: () => void
}

export function CopyTextModal({ text, title, onCancel }: CopyTextModalProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.select()
    }
  }, [])

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
          backgroundColor: 'var(--bg-primary)',
          borderRadius: 8,
          padding: 24,
          maxWidth: '80vw',
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
        }}
      >
        <h2 style={{ marginTop: 0, marginBottom: 16, fontSize: 16 }}>{title}</h2>
        <p style={{ margin: '0 0 12px 0', fontSize: 13, color: '#666' }}>
          文本已选中，按 Ctrl+C (或 Cmd+C) 复制：
        </p>
        <textarea
          ref={textareaRef}
          value={text}
          readOnly
          style={{
            flex: 1,
            minHeight: 200,
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
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
