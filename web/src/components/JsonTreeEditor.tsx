import { useState } from 'react'

interface JsonNodeInfo {
  path: string[] // e.g., ['payload', 'amount']
  type: 'string' | 'number' | 'boolean' | 'null' | 'object' | 'array'
  value: any
  isExpanded: boolean
}

interface JsonTreeEditorProps {
  initialValue: any
  rootName?: string
  onApply: (newValue: string) => void
  onCancel: () => void
}

export function JsonTreeEditor({ initialValue, rootName, onApply, onCancel }: JsonTreeEditorProps) {
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

  const handleKeyRename = (newKey: string) => {
    if (selectedPath.length === 0 || !newKey) return
    const newJson = JSON.parse(JSON.stringify(jsonValue))
    let parent = newJson
    for (let i = 0; i < selectedPath.length - 1; i++) {
      parent = parent[selectedPath[i]]
    }
    const oldKey = selectedPath[selectedPath.length - 1]
    if (oldKey === newKey) return
    // Preserve order: rebuild the parent object with renamed key
    if (Array.isArray(parent)) return // can't rename array indices
    const entries = Object.entries(parent)
    const newParent: Record<string, any> = {}
    for (const [k, v] of entries) {
      if (k === oldKey) {
        newParent[newKey] = v
      } else {
        newParent[k] = v
      }
    }
    let target = newJson
    for (let i = 0; i < selectedPath.length - 1; i++) {
      target = target[selectedPath[i]]
    }
    // Replace parent in-place
    Object.keys(parent).forEach((k) => delete (parent as any)[k])
    Object.assign(parent, newParent)
    setJsonValue(newJson)
    // Update selectedPath
    const newPath = [...selectedPath]
    newPath[newPath.length - 1] = newKey
    setSelectedPath(newPath)
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
            onClick={(e) => { e.stopPropagation(); setSelectedPath(path) }}
            onDoubleClick={(e) => { e.stopPropagation(); toggleExpand(path) }}
            style={{
              padding: '4px 8px',
              paddingLeft: 8 + path.length * 16,
              cursor: 'pointer',
              backgroundColor: isSelected ? 'var(--bg-hover)' : 'transparent',
              borderLeft: isSelected ? '3px solid var(--accent)' : '3px solid transparent',
            }}
          >
            <span onClick={(e) => { e.stopPropagation(); toggleExpand(path) }} style={{ marginRight: 4, fontWeight: 'bold', display: 'inline-block', width: 12 }}>
              {isExpanded ? '▼' : '▶'}
            </span>
            <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
              {path[path.length - 1] || rootName || 'ROOT'} <span style={{ color: 'var(--text-muted)' }}>({type})</span>
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
          onClick={(e) => { e.stopPropagation(); setSelectedPath(path) }}
          style={{
            padding: '4px 8px',
            paddingLeft: 8 + path.length * 16 + 16,
            cursor: 'pointer',
            backgroundColor: isSelected ? 'var(--bg-hover)' : 'transparent',
            borderLeft: isSelected ? '3px solid var(--accent)' : '3px solid transparent',
          }}
        >
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>
            {path[path.length - 1]} <span style={{ color: 'var(--text-muted)' }}>({type})</span>
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
            border: '1px solid var(--border-strong)',
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
            style={{ padding: '6px 12px', backgroundColor: 'var(--accent)', color: 'white', border: 'none', borderRadius: 4 }}
          >
            Apply
          </button>
        </div>
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', gap: 8, height: 300, border: '1px solid var(--border-strong)', borderRadius: 4, overflow: 'hidden' }}>
        {/* Left tree */}
        <div style={{ flex: 1, overflowY: 'auto', borderRight: '1px solid var(--border-color)', padding: 8 }}>
          {renderTree(jsonValue)}
        </div>

        {/* Right editor */}
        <div style={{ flex: 1, padding: 8, overflowY: 'auto' }}>
          <>
            {selectedPath.length > 0 && (
              <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 8 }}>
                <strong>Key:</strong>{' '}
                <input
                  key={`key-${selectedPath.join('/')}`}
                  type="text"
                  defaultValue={selectedPath[selectedPath.length - 1]}
                  onBlur={(e) => handleKeyRename(e.currentTarget.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      handleKeyRename((e.target as HTMLInputElement).value)
                    }
                  }}
                  style={{
                    padding: '2px 6px',
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid var(--border-strong)',
                    borderRadius: 3,
                    width: '60%',
                  }}
                />
              </div>
            )}
              <div style={{ fontSize: 12, color: 'var(--text-secondary)', marginBottom: 8 }}>
                <strong>Type:</strong>{' '}
                {selectedType === 'object' || selectedType === 'array' ? selectedType : (
                  <select
                    value={selectedType}
                    onChange={(e) => {
                      const t = e.target.value
                      let newVal: any
                      if (t === 'null') newVal = null
                      else if (t === 'number') newVal = selectedValue !== null ? Number(selectedValue) || 0 : 0
                      else if (t === 'boolean') newVal = selectedValue !== null ? Boolean(selectedValue) : false
                      else newVal = selectedValue !== null ? String(selectedValue) : ''
                      handleScalarEdit(newVal)
                    }}
                    style={{ fontFamily: 'monospace', fontSize: 11, padding: '1px 4px' }}
                  >
                    <option value="string">string</option>
                    <option value="number">number</option>
                    <option value="boolean">boolean</option>
                    <option value="null">null</option>
                  </select>
                )}
              </div>
              {selectedType === 'string' && (
                <textarea
                  key={selectedPath.join('/')}
                  value={String(selectedValue ?? '')}
                  onChange={(e) => handleScalarEdit(e.target.value)}
                  style={{
                    width: '100%',
                    minHeight: 100,
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid var(--border-strong)',
                    borderRadius: 3,
                  }}
                />
              )}
              {selectedType === 'number' && (
                <input
                  key={selectedPath.join('/')}
                  type="number"
                  defaultValue={selectedValue ?? ''}
                  onBlur={(e) => {
                    if (e.target.value !== '' && !isNaN(Number(e.target.value))) {
                      handleScalarEdit(Number(e.target.value))
                    }
                  }}
                  style={{
                    width: '100%',
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid var(--border-strong)',
                    borderRadius: 3,
                  }}
                />
              )}
              {selectedType === 'boolean' && (
                <select
                  key={selectedPath.join('/')}
                  value={String(selectedValue)}
                  onChange={(e) => handleScalarEdit(e.target.value === 'true')}
                  style={{
                    width: '100%',
                    padding: 6,
                    fontFamily: 'monospace',
                    fontSize: 11,
                    border: '1px solid var(--border-strong)',
                    borderRadius: 3,
                  }}
                >
                  <option value="true">true</option>
                  <option value="false">false</option>
                </select>
              )}
              {selectedType === 'null' && <div style={{ color: 'var(--text-muted)', fontSize: 12 }}>null — use Type selector above to convert</div>}
              {(selectedType === 'object' || selectedType === 'array') && (
                <div style={{ color: 'var(--text-muted)', fontSize: 12 }}>
                  [{selectedType}] - select a leaf node to edit
                </div>
              )}
          </>
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
          style={{ padding: '6px 12px', backgroundColor: 'var(--accent)', color: 'white', border: 'none', borderRadius: 4 }}
        >
          Apply
        </button>
      </div>
    </div>
  )
}
