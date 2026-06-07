import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { JsonTreeEditor } from './JsonTreeEditor'

const openRootObject = () => {
  fireEvent.click(screen.getByText('▶'))
}

describe('JsonTreeEditor', () => {
  it('does not turn a cleared number field into locked null while editing', () => {
    const onApply = vi.fn()
    render(<JsonTreeEditor initialValue={{ user_id: 47690 }} onApply={onApply} onCancel={vi.fn()} />)

    openRootObject()
    fireEvent.click(screen.getByText(/^user_id/))

    const editor = screen.getByDisplayValue('47690')
    fireEvent.change(editor, { target: { value: '' } })

    expect(screen.getByDisplayValue('')).toBe(editor)

    fireEvent.change(editor, { target: { value: '123' } })
    fireEvent.blur(editor)
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(onApply).toHaveBeenCalledWith('{"user_id":123}')
  })

  it('converts the string false to boolean false from the type selector', () => {
    const onApply = vi.fn()
    render(<JsonTreeEditor initialValue={{ enabled: 'false' }} onApply={onApply} onCancel={vi.fn()} />)

    openRootObject()
    fireEvent.click(screen.getByText(/^enabled/))

    const typeRow = screen.getByText('Type:').closest('div') as HTMLElement
    fireEvent.change(within(typeRow).getByRole('combobox'), { target: { value: 'boolean' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(onApply).toHaveBeenCalledWith('{"enabled":false}')
  })
})
