export function splitSQL(text: string): string[] {
  const out: string[] = []
  let cur = ''
  let inSingle = false
  let inDouble = false
  let inBacktick = false
  let inLineComment = false
  let inBlockComment = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    const n = text[i + 1]
    if (inLineComment) {
      cur += c
      if (c === '\n') inLineComment = false
      continue
    }
    if (inBlockComment) {
      cur += c
      if (c === '*' && n === '/') { cur += n; i++; inBlockComment = false }
      continue
    }
    if (inSingle) {
      cur += c
      if (c === '\\' && n !== undefined) { cur += n; i++; continue }
      if (c === "'") inSingle = false
      continue
    }
    if (inDouble) {
      cur += c
      if (c === '\\' && n !== undefined) { cur += n; i++; continue }
      if (c === '"') inDouble = false
      continue
    }
    if (inBacktick) {
      cur += c
      if (c === '`') inBacktick = false
      continue
    }
    if (c === '-' && n === '-') { inLineComment = true; cur += c; continue }
    if (c === '/' && n === '*') { inBlockComment = true; cur += c; continue }
    if (c === "'") { inSingle = true; cur += c; continue }
    if (c === '"') { inDouble = true; cur += c; continue }
    if (c === '`') { inBacktick = true; cur += c; continue }
    if (c === ';') {
      const s = cur.trim()
      if (s) out.push(s)
      cur = ''
      continue
    }
    cur += c
  }
  const s = cur.trim()
  if (s) out.push(s)
  return out
}
