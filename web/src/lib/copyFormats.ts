// web/src/lib/copyFormats.ts

export interface CopyFormatsInput {
  cellValue: any
  columnName: string
  rowData: any[]
  columnNames: string[]
  tableName: string
  dbName: string
}

/**
 * Copy cell value as-is (string representation)
 */
export function copyAsText(value: any): string {
  if (value === null) return 'NULL'
  if (value === undefined) return ''
  return String(value)
}

/**
 * Copy entire row as tab-separated values
 */
export function copyAsTabSeparated(rowData: any[]): string {
  return rowData.map((v) => (v === null ? '' : String(v))).join('\t')
}

/**
 * Copy entire column values as tab-separated
 */
export function copyColumnAsTabSeparated(
  _columnName: string,
  allRows: any[][],
  columnIdx: number,
): string {
  // Note: columnName parameter is for API compatibility but not used in implementation
  return allRows.map((row) => {
    const v = row[columnIdx]
    return v === null ? '' : String(v)
  }).join('\t')
}

/**
 * Copy as JSON (pretty-printed for objects, string for scalars)
 */
export function copyAsJson(value: any): string {
  if (value === null) return 'null'
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return JSON.stringify(value)
    }
  }
  return JSON.stringify(value, null, 2)
}

/**
 * Copy as TSV for Excel (single cell)
 */
export function copyAsTsv(value: any): string {
  // Single value already tab-ready, or as tab-quoted string
  if (value === null) return ''
  const str = String(value)
  // Excel TSV: quote if contains tab/newline/quote
  if (str.includes('\t') || str.includes('\n') || str.includes('"')) {
    return '"' + str.replace(/"/g, '""') + '"'
  }
  return str
}

/**
 * Copy as Markdown code block
 */
export function copyAsMarkdown(value: any): string {
  const content = value === null ? 'null' : String(value)
  return '```\n' + content + '\n```'
}

/**
 * Copy as INSERT statement (single row)
 */
export function copyAsInsertStatement(
  rowData: any[],
  columnNames: string[],
  tableName: string,
): string {
  const cols = columnNames.join(', ')
  const values = rowData
    .map((v) => {
      if (v === null) return 'NULL'
      if (typeof v === 'number') return String(v)
      if (typeof v === 'boolean') return v ? '1' : '0'
      // String: escape single quotes
      return "'" + String(v).replace(/'/g, "''") + "'"
    })
    .join(', ')
  return `INSERT INTO ${tableName} (${cols}) VALUES (${values})`
}
