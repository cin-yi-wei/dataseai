declare global {
  interface Window {
    runtime?: unknown
  }
}

function isWailsGUI(): boolean {
  return typeof window !== 'undefined' && window.runtime != null
}

export async function openExternal(url: string): Promise<void> {
  if (!isWailsGUI()) {
    window.open(url, '_blank', 'noopener')
    return
  }
  try {
    const res = await fetch('/__wails/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    })
    if (res.status === 204) return
  } catch {
    // network error — fall through to window.open
  }
  window.open(url, '_blank', 'noopener')
}
