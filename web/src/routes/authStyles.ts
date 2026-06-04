import type { CSSProperties } from 'react'

export const authPage: CSSProperties = {
  minHeight: '100vh',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  padding: 16,
  fontFamily: 'system-ui, -apple-system, sans-serif',
  background:
    'radial-gradient(circle at 20% 0%, rgba(167, 139, 250, 0.25), transparent 60%),' +
    'radial-gradient(circle at 80% 100%, rgba(14, 165, 233, 0.25), transparent 60%),' +
    'var(--bg-primary)',
}

export const authCard: CSSProperties = {
  width: '100%',
  maxWidth: 400,
  padding: 32,
  borderRadius: 16,
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border-color)',
  boxShadow: '0 20px 60px rgba(0, 0, 0, 0.25)',
}

export const authInput: CSSProperties = {
  width: '100%',
  padding: '12px 14px',
  fontSize: 14,
  borderRadius: 8,
  border: '1px solid var(--border-strong)',
  background: 'var(--bg-input)',
  color: 'var(--text-primary)',
  boxSizing: 'border-box',
}

export const authButton: CSSProperties = {
  width: '100%',
  padding: '12px 14px',
  fontSize: 14,
  fontWeight: 600,
  borderRadius: 8,
  border: 'none',
  background: 'var(--accent)',
  color: 'white',
  cursor: 'pointer',
  marginTop: 4,
}

export const authError: CSSProperties = {
  padding: '10px 12px',
  fontSize: 13,
  borderRadius: 8,
  background: 'rgba(220, 53, 69, 0.12)',
  border: '1px solid rgba(220, 53, 69, 0.35)',
  color: 'var(--danger)',
}

export const authLink: CSSProperties = {
  color: 'var(--accent)',
  textDecoration: 'none',
  fontWeight: 600,
}
