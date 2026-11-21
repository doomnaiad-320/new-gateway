/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { UpstreamBalanceAccount } from './upstream-balance-api'
import { UpstreamBalances } from './upstream-balances'

const account: UpstreamBalanceAccount = {
  id: 7,
  name: 'Primary upstream',
  api_base_url: 'https://upstream.example.com',
  user_id: '42',
  username: 'operator',
  enabled: true,
  has_token: true,
  quota: 10,
  used_quota: 2,
  request_count: 12,
  last_synced_at: '2026-11-21T10:00:00Z',
  last_error: '',
}

let queryClient: QueryClient

function renderBalances(isRoot: boolean) {
  queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <UpstreamBalances isRoot={isRoot} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: [account] },
  } as never)
})

afterEach(() => {
  cleanup()
  queryClient.clear()
  vi.restoreAllMocks()
})

describe('upstream balance access and editing', () => {
  it('lets admins view balances and refresh without account-management controls', async () => {
    renderBalances(false)

    expect(await screen.findByText('Primary upstream')).toBeInTheDocument()
    expect(screen.getByText('Current balance')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled()
    const refreshButton = screen.getByRole('button', { name: 'Refresh' })
    const lastUpdated = screen.getByRole('status')
    expect(refreshButton.parentElement).toContainElement(lastUpdated)
    expect(lastUpdated.closest('[data-slot="card"]')).toBeNull()
    expect(lastUpdated).toHaveTextContent(/Last updated:.*\d{4}-\d{2}-\d{2}/)
    expect(
      screen.queryByRole('button', { name: 'Edit upstream account' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete upstream account' })
    ).not.toBeInTheDocument()
  })

  it('keeps the saved token field blank when editing and saving an account', async () => {
    const user = userEvent.setup()
    const update = vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: true, data: account },
    } as never)
    renderBalances(true)

    await user.click(
      await screen.findByRole('button', { name: 'Edit upstream account' })
    )
    expect(screen.getByLabelText('Access token (Optional)')).toHaveValue('')
    expect(screen.getByLabelText('Access token (Optional)')).toHaveAttribute(
      'placeholder',
      'Leave blank to keep the current token'
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(
        '/api/upstream-balance/accounts/7',
        expect.objectContaining({ access_token: '' })
      )
    )
  })

  it('warns when an account uses an unencrypted HTTP URL', async () => {
    const user = userEvent.setup()
    renderBalances(true)

    await user.click(await screen.findByRole('button', { name: 'Add account' }))
    await user.type(
      screen.getByLabelText('API base URL'),
      'http://38.58.181.83:9090'
    )

    expect(
      screen.getByText(
        'HTTP sends the access token without encryption. Use only with a trusted upstream.'
      )
    ).toBeInTheDocument()
  })
})
