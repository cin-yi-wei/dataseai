import { useEffect, useState } from 'react'
import Login from './routes/Login'
import Register from './routes/Register'
import Workspace from './routes/Workspace'
import Settings from './routes/Settings'
import { useAuth } from './store/auth'

type View = 'auth-login' | 'auth-register' | 'workspace' | 'settings'

export default function App() {
  const { user, ready, bootstrap } = useAuth()
  const [view, setView] = useState<View>('auth-login')

  useEffect(() => {
    void bootstrap()
  }, [bootstrap])

  if (!ready) {
    return <main style={{ fontFamily: 'system-ui', padding: 24 }}>loading…</main>
  }

  if (!user) {
    if (view === 'auth-register') return <Register onSwitchToLogin={() => setView('auth-login')} />
    return <Login onSwitchToRegister={() => setView('auth-register')} />
  }

  if (view === 'settings') return <Settings onClose={() => setView('workspace')} />
  return <Workspace onOpenSettings={() => setView('settings')} />
}
