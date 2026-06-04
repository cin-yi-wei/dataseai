import { useState } from 'react'

interface JsonNodeInfo {
  path: string[] // e.g., ['payload', 'amount']
  type: 'string' | 'number' | 'boolean' | 'null' | 'object' | 'array'
  value: any
  isExpanded: boolean
}

interface JsonTreeEditorProps {
  initialValue: any
  onApply: (newValue: string) => void
  onCancel: () => void
}

export function JsonTreeEditor({ initialValue, onApply, onCancel }: JsonTreeEditorProps) {
  const [jsonValue, setJsonValue] = useState(() => {
    if (typeof initialValue === 'string') {
      try {
        return JSON.parse(initialValue)
      } catch {
        return initialValue
      }
    }
    return initialValue
  })
  const [selectedPath, setSelectedPath] = useState<string[]>([])
  const [isRawMode, setIsRawMode] = useState(false)
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set())

  const getNodeAtPath = (obj: any, path: string[]): any => {
    return path.reduce((cur, key) => cur?.[key], obj)
  }

  const getNodeType = (value: any): JsonNodeInfo['type'] => {
    if (value === null) return 'null'
    if (Array.isArray(value)) return 'array'
    if (typeof value === 'object') return 'object'
    if (typeof value === 'string') return 'string'
    if (typeof value === 'number') return 'number'
    if (typeof value === 'boolean') return 'boolean'
    return 'null'
  }

  const selectedValue = getNodeAtPath(jsonValue, selectedPath)
  const selectedType = getNodeType(selectedValue)

  const toggleExpand = (path: string[]) => {
    const key = path.join('/')
    setExpandedPaths((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  const handleRawChange = (rawText: string) => {
    try {
      const parsed = JSON.parse(rawText)
      setJsonValue(parsed)
      setIsRawMode(false)
    } catch (e) {
      window.alert('Invalid JSON: ' + (e instanceof Error ? e.message : 'parse error'))
    }
  }

  const handleScalarEdit = (newValue: any) => {
    if (selectedPath.length === 0) {
      setJsonValue(newValue)
    } else {
      const newJson = JSON.parse(JSON.stringify(jsonValue)) // Deep clone
      let obj = newJson
      for (let i = 0; i < selectedPath.length - 1; i++) {
        obj = obj[selectedPath[i]]
      }
      obj[selectedPath[selectedPath.length - 1]] = newValue
      setJsonValue(newJson)
    }
  }

  const renderTree = (obj: any, path: string[] = []): JSX.Element => {
    const type = getNodeType(obj)
    const pathKey = path.join('/')
    const isExpanded = expandedPaths.has(pathKey)
    const isSelected = pathKey === selectedPath.join('/')

    if (type === 'object' || type === 'array') {
      const entries = type === 'object' ? Object.entries(obj) : obj.map((v: any, i: number) => [i.toString(), v])
      return (
        <div key={pathKey}>
          <div
            onClick={() => setSelectedPath(path)}
            onDoubleClick={() => toggleExpand(path)}
            style={{
              padding: '4px 8px',
              cursor: 'pointer',
              backgroundColor: isSelected ? '#e0e0e0' : 'transparent',
              borderLeft: isSelected ? '3px solid #0066cc' : '3px solid transparent',
            }}
          >
            <span onClick={() => toggleExpand(path)} style={{ marginRight: 4, fontWeight: 'bold' }}>
              {isExpanded ? '▼' : '▶'}
            </span>
            <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
              {path[path.length - 1] || 'ROOT'} ({type})
            </span>
          </div>
          {isExpanded &&
            entries.map(([key, value]: [string, any]) => renderTree(value, [...path, key]))}
        </div>
      )
    } else {
      return (
        <div
          key={pathKey}
          onClick={() => setSelectedPath(path)}
          style={{
            padding: '4px 8px',
            cursor: 'pointer',
            backgroundColor: isSelected ? '#e0e0e0' : 'transparent',
            borderLeft: isSelected ? '3px solid #0066cc' : '3px solid transparent',
          }}
        >
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {path[path.length - 1]} ({type})
          </span>
        </div>
      )
    }
  }

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(jsonValue, null, 2))
    } catch {
      window.alert('Failed to copy')
    }
  }

  if (isRawMode) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <textarea
          defaultValue={JSON.stringify(jsonValue, null, 2)}
          onBlur={(e) => handleRawChange(e.currentTarget.value)}
          style={{
            flex: 1,
            minHeight: 300,
            padding: 8,
            fontFamily: 'monospace',
            fontSize: 12,
            border: '1px solid #ccc',
            borderRadius: 4,
          }}
        />
        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button onClick={onCancel} style={{ padding: '6px 12px' }}>
            Cancel
          </button>
          <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
            Copy
          </button>
          <button onClick={() => setIsRawMode(false)} style={{ padding: '6px 12px' }}>
            Format
          </button>
          <button
            onClick={() => onApply(JSON.stringify(jsonValue))}
            style={{ padding: '6px 12px', backgroundColor: '#0066cc', color: 'white', border: 'none', borderRadius: 4 }}
          >
            Apply
          </button>
        </div>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', gap: 8, height: 300, border: '1px solid #ccc', borderRadius: 4, overflow: 'hidden' }}>
        {/* Left tree */}
        <div style={{ flex: 1, overflowY: 'auto', borderRight: '1px solid #eee', padding: 8 }}>
          {renderTree(jsonValue)}
        </div>

        {/* Right editor */}
        <div style={{ flex: 1, padding: 8, overflowY: 'auto' }}>
          {selectedPath.length > 0 && (
            <>
              <div style={{ fontSize: 12, color: '#666', marginBottom: 8 }}>
                <strong>Type:</strong> {selectedType}
              </div>
              {selectedType === 'string' && (
                <textarea
                  defaultValue={String(selectedValue || '')}
                  onChange={(e) => handleScalarEdit(e.target.value)}
                  style={{
                    width: '100%',
                    minHeight: 100,
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid #ccc',
                    borderRadius: 3,
                  }}
                />
              )}
              {selectedType === 'number' && (
                <input
                  type="number"
                  defaultValue={selectedValue}
                  onChange={(e) => handleScalarEdit(e.target.value === '' ? null : Number(e.target.value))}
                  style={{
                    width: '100%',
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid #ccc',
                    borderRadius: 3,
                  }}
                />
              )}
              {selectedType === 'boolean' && (
                <select
                  defaultValue={String(selectedValue)}
                  onChange={(e) => handleScalarEdit(e.target.value === 'true')}
                  style={{
                    width: '100%',
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid #ccc',
                    borderRadius: 3,
                  }}
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              )}
              {selectedType === 'null' && <div style={{ color: '#999' }}>null (cannot edit)</div>}
              {(selectedType === 'object' || selectedType === 'array') && (
                <div style={{ color: '#999', fontSize: 12 }}>
                  [{selectedType}] - select a leaf node to edit
                </div>
              )}
            </>
          )}
        </div>
      </div>

      <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
        <button onClick={onCancel} style={{ padding: '6px 12px' }}>
          Cancel
        </button>
        <button onClick={handleCopy} style={{ padding: '6px 12px' }}>
          Copy
        </button>
        <button onClick={() => setIsRawMode(true)} style={{ padding: '6px 12px' }}>
          Raw
        </button>
        <button
          onClick={() => onApply(JSON.stringify(jsonValue))}
          style={{ padding: '6px 12px', backgroundColor: '#0066cc', color: 'white', border: 'none', borderRadius: 4 }}
        >
          Apply
        </button>
      </div>
    </div>
  )
}
