// Lightweight, non-blocking toast — a small floating pill that fades out.
// Replaces blocking window.alert for transient notices like "copied" so the
// user never has to click OK.

let container: HTMLDivElement | null = null

export function toast(message: string): void {
  if (typeof document === 'undefined') return
  if (!container || !document.body.contains(container)) {
    container = document.createElement('div')
    container.setAttribute('data-toast-container', '')
    container.style.cssText =
      'position:fixed;left:50%;bottom:32px;transform:translateX(-50%);' +
      'z-index:3000;display:flex;flex-direction:column;gap:8px;align-items:center;pointer-events:none'
    document.body.appendChild(container)
  }
  const el = document.createElement('div')
  el.textContent = message
  el.style.cssText =
    'background:rgba(28,28,30,0.95);color:#fff;padding:8px 16px;border-radius:999px;' +
    'font-size:13px;font-family:system-ui;box-shadow:0 4px 16px rgba(0,0,0,0.3);' +
    'opacity:0;transform:translateY(6px);transition:opacity .15s ease,transform .15s ease'
  container.appendChild(el)
  requestAnimationFrame(() => {
    el.style.opacity = '1'
    el.style.transform = 'translateY(0)'
  })
  setTimeout(() => {
    el.style.opacity = '0'
    el.style.transform = 'translateY(6px)'
    setTimeout(() => el.remove(), 200)
  }, 1500)
}
