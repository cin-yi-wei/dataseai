import { useEffect, useState } from 'react'

// useIsNarrow reports whether the viewport is narrower than the breakpoint
// (default 760px) so components can switch to a single-column mobile layout.
export function useIsNarrow(breakpoint = 760): boolean {
  const [narrow, setNarrow] = useState(
    typeof window !== 'undefined' ? window.innerWidth < breakpoint : false,
  )
  useEffect(() => {
    const onResize = () => setNarrow(window.innerWidth < breakpoint)
    window.addEventListener('resize', onResize)
    onResize()
    return () => window.removeEventListener('resize', onResize)
  }, [breakpoint])
  return narrow
}
