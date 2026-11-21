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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UpstreamModelCostsSection } from '../upstream-model-costs-section'

function renderSection(defaultValue = '[]') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <UpstreamModelCostsSection defaultValue={defaultValue} />
    </QueryClientProvider>
  )
}

afterEach(() => vi.restoreAllMocks())

test('an upstream cost rule is saved with its request model and channel match', async () => {
  vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true, message: '' },
  })
  const user = userEvent.setup()
  renderSection()

  const save = screen.getByRole('button', { name: 'Save' })
  expect(save).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Add rule' }))

  fireEvent.change(screen.getByRole('textbox', { name: 'Request model' }), {
    target: { value: 'client-model' },
  })
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Channel ID' }), {
    target: { value: '12' },
  })
  fireEvent.change(screen.getByRole('textbox', { name: 'Upstream model' }), {
    target: { value: 'provider-model' },
  })
  fireEvent.change(
    screen.getByRole('spinbutton', { name: 'Cost per request (USD)' }),
    { target: { value: '0.025' } }
  )

  expect(save).toBeEnabled()
  await user.click(save)
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith('/api/option/', {
      key: 'ProfitUpstreamCosts',
      value:
        '[{"model_name":"client-model","channel_id":12,"upstream_model_name":"provider-model","cost_per_request":0.025}]',
    })
  )
})

test('duplicate request, channel, and upstream model rules cannot be saved', () => {
  renderSection(
    '[{"model_name":"client-model","channel_id":12,"upstream_model_name":"provider-model","cost_per_request":0.1},{"model_name":"client-model","channel_id":12,"upstream_model_name":"provider-model","cost_per_request":0.2}]'
  )

  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(
    screen.getAllByRole('spinbutton', { name: 'Channel ID' })
  ).toHaveLength(2)
  for (const input of screen.getAllByRole('spinbutton', {
    name: 'Channel ID',
  })) {
    expect(input).toHaveAttribute('aria-invalid', 'true')
  }
})
