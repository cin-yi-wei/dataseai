import { useState, useCallback } from 'react'

interface MenuPosition {
  x: number
  y: number
}

export function useContextMenu() {
  const [position, setPosition] = useState<MenuPosition | null>(null)
  const [cellInfo, setCellInfo] = useState<{ rowIdx: number; colIdx: number } | null>(null)
  const [cellValue, setCellValue] = useState<any>(null)

  const handleContextMenu = useCallback(
    (e: React.MouseEvent, rowIdx: number, colIdx: number, value: any) => {
      e.preventDefault()
      setPosition({ x: e.clientX, y: e.clientY })
      setCellInfo({ rowIdx, colIdx })
      setCellValue(value)
    },
    [],
  )

  const closeMenu = useCallback(() => {
    setPosition(null)
    setCellInfo(null)
    setCellValue(null)
  }, [])

  return { position, cellInfo, cellValue, handleContextMenu, closeMenu }
}
