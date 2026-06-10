export async function saveAsFile(blob: Blob, filename: string): Promise<void> {
  try {
    const res = await fetch(`/__wails/save?name=${encodeURIComponent(filename)}`, {
      method: 'POST',
      headers: { 'Content-Type': blob.type || 'application/octet-stream' },
      body: blob,
    })
    if (res.status === 204) return
  } catch {
    // network error — fall through to anchor download
  }
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
