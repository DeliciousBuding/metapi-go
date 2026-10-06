import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { BackupImportSource } from '../backup-import-source'

afterEach(cleanup)

it('starts with file selection and reveals JSON only on request', () => {
  render(
    <BackupImportSource
      raw=''
      fileName=''
      originKey=''
      external={false}
      disabled={false}
      onFile={vi.fn()}
      onText={vi.fn()}
      onOrigin={vi.fn()}
    />
  )
  expect(screen.getByLabelText('Choose a JSON backup file')).toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Advanced · paste JSON' }))
  expect(
    screen.getByRole('textbox', { name: 'Advanced · paste JSON' })
  ).toBeVisible()
})
