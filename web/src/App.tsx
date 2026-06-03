import { useEffect, useState } from 'react'

export default function App() {
  const [health, setHealth] = useState<string>('loading...')
  useEffect(() => {
    fetch('/api/health')
      .then((r) => r.json())
      .then((j) => setHealth(`mysqlweb ${j.version} — uptime ${j.uptime_s}s`))
      .catch(() => setHealth('backend unreachable'))
  }, [])
  return (
    <main style={{ fontFamily: 'system-ui', padding: 24 }}>
      <h1>mysqlweb</h1>
      <p>{health}</p>
    </main>
  )
}
