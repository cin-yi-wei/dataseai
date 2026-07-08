import { useEffect, useState } from 'react'
import Login from './routes/Login'
import Register from './routes/Register'
import ForgotPassword from './routes/ForgotPassword'
import Workspace from './routes/Workspace'
import Settings from './routes/Settings'
import AdminPage from './routes/AdminPage'
import { useAuth } from './store/auth'

type View = 'auth-login' | 'auth-register' | 'auth-forgot' | 'workspace' | 'settings' | 'admin'

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
    if (view === 'auth-forgot') return <ForgotPassword onSwitchToLogin={() => setView('auth-login')} />
    return (
      <Login
        onSwitchToRegister={() => setView('auth-register')}
        onSwitchToForgot={() => setView('auth-forgot')}
      />
    )
  }

  if (view === 'settings') return <Settings onClose={() => setView('workspace')} />
  if (view === 'admin') return <AdminPage onClose={() => setView('workspace')} />
  return <Workspace
    onOpenSettings={() => setView('settings')}
    onOpenAdmin={() => setView('admin')}
  />
}
