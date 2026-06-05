import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
  info: ErrorInfo | null
}

// ErrorBoundary catches any render-phase crash from its children and renders
// the error + component stack on screen so mobile users can see what failed
// instead of staring at a blank page. Visible by default — no opt-in needed.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null, info: null }

  static getDerivedStateFromError(error: Error): Partial<State> {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    this.setState({ info })
    // eslint-disable-next-line no-console
    console.error('[ErrorBoundary]', error, info)
  }

  reset = () => this.setState({ error: null, info: null })

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div
        style={{
          padding: 16,
          background: '#3b1f1a',
          color: '#ffd9b3',
          minHeight: '100vh',
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
          fontSize: 13,
          lineHeight: 1.5,
        }}
      >
        <h2 style={{ marginTop: 0, color: '#ffb38a' }}>⚠ UI 崩潰</h2>
        <p style={{ color: '#ffd9b3' }}>
          下面是錯誤訊息，可以截圖丟給工程師：
        </p>
        <div
          style={{
            background: 'rgba(0,0,0,0.4)',
            border: '1px solid #6e3b2a',
            padding: 10,
            borderRadius: 4,
            overflowX: 'auto',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          <strong>{this.state.error.name}</strong>: {this.state.error.message}
          {'\n\n'}
          {this.state.error.stack}
          {this.state.info?.componentStack && (
            <>
              {'\n\n--- React component stack ---'}
              {this.state.info.componentStack}
            </>
          )}
        </div>
        <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
          <button onClick={this.reset} style={{ background: '#6e3b2a', color: '#ffd9b3', border: 'none', padding: '6px 12px', borderRadius: 3 }}>
            再試一次
          </button>
          <button onClick={() => location.reload()} style={{ background: '#6e3b2a', color: '#ffd9b3', border: 'none', padding: '6px 12px', borderRadius: 3 }}>
            重新整理
          </button>
        </div>
      </div>
    )
  }
}
