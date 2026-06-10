export async function openExternal(url: string): Promise<void> {
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
