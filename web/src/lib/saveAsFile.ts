declare global {
  interface Window {
    runtime?: unknown
  }
}

// Wails injects window.runtime at bootstrap. When absent we are in a
// plain browser and should NOT POST /__wails/save (the dataseai server
// has no such route and the request would slow the download for no
// reason).
function isWailsGUI(): boolean {
  return typeof window !== 'undefined' && window.runtime != null
}

function fallbackAnchorDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // Revoke after a tick so the browser has time to grab the blob.
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

export async function saveAsFile(blob: Blob, filename: string): Promise<void> {
  if (!isWailsGUI()) {
    fallbackAnchorDownload(blob, filename)
    return
  }
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
  fallbackAnchorDownload(blob, filename)
}
