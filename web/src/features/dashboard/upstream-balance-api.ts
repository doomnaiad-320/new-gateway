import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

export type UpstreamBalanceAccount = {
  id: number
  name: string
  api_base_url: string
  user_id: string
  username: string
  enabled: boolean
  has_token: boolean
  quota: number
  used_quota: number
  request_count: number
  last_synced_at: string | null
  last_error: string
}

export type UpstreamBalanceAccountInput = {
  name: string
  api_base_url: string
  user_id: string
  access_token: string
  enabled: boolean
}

type AccountResponse = { success: boolean; data: UpstreamBalanceAccount }
type AccountListResponse = {
  success: boolean
  data: UpstreamBalanceAccount[]
}

export async function getUpstreamBalanceAccounts() {
  const response = await api.get<AccountListResponse>(
    '/api/upstream-balance/accounts'
  )
  return requireServerSuccess(response.data).data
}

export async function refreshUpstreamBalanceAccounts() {
  const response = await api.post<AccountListResponse>(
    '/api/upstream-balance/refresh'
  )
  return requireServerSuccess(response.data).data
}

export async function createUpstreamBalanceAccount(
  input: UpstreamBalanceAccountInput
) {
  const response = await api.post<AccountResponse>(
    '/api/upstream-balance/accounts',
    input
  )
  return requireServerSuccess(response.data).data
}

export async function updateUpstreamBalanceAccount(
  id: number,
  input: UpstreamBalanceAccountInput
) {
  const response = await api.put<AccountResponse>(
    `/api/upstream-balance/accounts/${id}`,
    input
  )
  return requireServerSuccess(response.data).data
}

export async function deleteUpstreamBalanceAccount(id: number) {
  const response = await api.delete<{ success: boolean }>(
    `/api/upstream-balance/accounts/${id}`
  )
  requireServerSuccess(response.data)
}

export async function testUpstreamBalanceAccount(
  input: UpstreamBalanceAccountInput
) {
  const response = await api.post<AccountResponse>(
    '/api/upstream-balance/accounts/test',
    input
  )
  return requireServerSuccess(response.data).data
}

export async function testSavedUpstreamBalanceAccount(id: number) {
  const response = await api.post<AccountResponse>(
    `/api/upstream-balance/accounts/${id}/test`
  )
  return requireServerSuccess(response.data).data
}
