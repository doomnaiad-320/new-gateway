import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  formatBillingCurrencyFromUSD,
  getCurrencyDisplay,
} from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import {
  getServerErrorMessage,
  translateServerText,
} from '@/lib/server-error-message'

import {
  createUpstreamBalanceAccount,
  deleteUpstreamBalanceAccount,
  getUpstreamBalanceAccounts,
  refreshUpstreamBalanceAccounts,
  testUpstreamBalanceAccount,
  testSavedUpstreamBalanceAccount,
  updateUpstreamBalanceAccount,
  type UpstreamBalanceAccount,
  type UpstreamBalanceAccountInput,
} from './upstream-balance-api'

const ACCOUNT_QUERY_KEY = ['upstream-balance-accounts']

const EMPTY_FORM: UpstreamBalanceAccountInput = {
  name: '',
  api_base_url: '',
  user_id: '',
  access_token: '',
  enabled: true,
}

export function UpstreamBalances({ isRoot }: { isRoot: boolean }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<UpstreamBalanceAccount | null>(null)
  const [form, setForm] = useState(EMPTY_FORM)
  const [deleteTarget, setDeleteTarget] =
    useState<UpstreamBalanceAccount | null>(null)

  const accountsQuery = useQuery({
    queryKey: ACCOUNT_QUERY_KEY,
    queryFn: getUpstreamBalanceAccounts,
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  })

  const refreshMutation = useMutation({
    mutationFn: refreshUpstreamBalanceAccounts,
    onSuccess: (accounts) => {
      queryClient.setQueryData(ACCOUNT_QUERY_KEY, accounts)
      toast.success(t('Upstream balances refreshed'))
    },
    onError: (error) => toast.error(getServerErrorMessage(error)),
  })

  const saveMutation = useMutation({
    mutationFn: (input: UpstreamBalanceAccountInput) =>
      editing
        ? updateUpstreamBalanceAccount(editing.id, input)
        : createUpstreamBalanceAccount(input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ACCOUNT_QUERY_KEY })
      setDialogOpen(false)
      setForm(EMPTY_FORM)
      toast.success(t('Upstream account saved'))
    },
    onError: (error) => toast.error(getServerErrorMessage(error)),
  })

  const testMutation = useMutation({
    mutationFn: (input: UpstreamBalanceAccountInput) =>
      editing && !input.access_token && canTestSavedAccount
        ? testSavedUpstreamBalanceAccount(editing.id)
        : testUpstreamBalanceAccount(input),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ACCOUNT_QUERY_KEY })
      toast.success(t('Connection successful'))
    },
    onError: (error) => {
      void queryClient.invalidateQueries({ queryKey: ACCOUNT_QUERY_KEY })
      toast.error(getServerErrorMessage(error))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: deleteUpstreamBalanceAccount,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ACCOUNT_QUERY_KEY })
      setDeleteTarget(null)
      toast.success(t('Upstream account deleted'))
    },
    onError: (error) => toast.error(getServerErrorMessage(error)),
  })

  const accounts = accountsQuery.data ?? []
  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencySymbol =
    currencyMeta.kind === 'tokens' ? '$' : currencyMeta.symbol
  const latestSyncedAt = accounts.reduce<string | null>((latest, account) => {
    if (!account.last_synced_at) return latest

    const candidate = dayjs(account.last_synced_at)
    if (!candidate.isValid()) return latest
    if (!latest || candidate.valueOf() > dayjs(latest).valueOf()) {
      return account.last_synced_at
    }
    return latest
  }, null)
  const canTestSavedAccount = Boolean(
    editing &&
    form.api_base_url.trim().replace(/\/+$/, '') === editing.api_base_url &&
    form.user_id.trim() === editing.user_id
  )

  let accountsContent: ReactNode
  if (accountsQuery.isLoading) {
    accountsContent = (
      <div className='text-muted-foreground rounded-lg border px-4 py-12 text-center text-sm'>
        {t('Loading upstream accounts...')}
      </div>
    )
  } else if (accountsQuery.isError) {
    accountsContent = (
      <div
        role='alert'
        className='text-destructive rounded-lg border px-4 py-12 text-center text-sm'
      >
        {getServerErrorMessage(accountsQuery.error)}
      </div>
    )
  } else if (accounts.length === 0) {
    accountsContent = (
      <EmptyState
        bordered
        title={t('No upstream accounts configured')}
        description={t(
          'Add an upstream account to start monitoring its balance.'
        )}
        action={
          isRoot ? (
            <Button type='button' onClick={openCreateDialog}>
              <Plus data-icon='inline-start' />
              {t('Add account')}
            </Button>
          ) : undefined
        }
      />
    )
  } else {
    accountsContent = (
      <div className='grid grid-cols-1 gap-2 lg:grid-cols-[repeat(auto-fill,minmax(18rem,20rem))]'>
        {accounts.map((account) => {
          let connectionLabel: string
          let connectionColor: string
          if (account.last_error) {
            connectionLabel = t('Connection failed')
            connectionColor = 'bg-destructive'
          } else if (!account.enabled) {
            connectionLabel = t('Disabled')
            connectionColor = 'bg-muted-foreground'
          } else if (account.last_synced_at) {
            connectionLabel = t('Connected')
            connectionColor = 'bg-emerald-500'
          } else {
            connectionLabel = t('Not yet checked')
            connectionColor = 'bg-muted-foreground'
          }

          const formattedBalance = account.last_synced_at
            ? formatBillingCurrencyFromUSD(account.quota, { compact: false })
            : '—'
          const symbolIndex = formattedBalance.indexOf(currencySymbol)

          return (
            <Card
              key={account.id}
              size='sm'
              className='w-full max-w-80 min-w-0 justify-self-start data-[size=sm]:gap-2 data-[size=sm]:py-2'
            >
              <CardHeader className='border-b group-data-[size=sm]/card:[.border-b]:pb-2'>
                <div className='flex min-w-0 items-center justify-between gap-3'>
                  <div className='min-w-0'>
                    <CardTitle className='truncate'>{account.name}</CardTitle>
                  </div>
                  <div className='flex shrink-0 items-center gap-1'>
                    <span
                      className={`size-2 rounded-full ${connectionColor}`}
                      aria-label={connectionLabel}
                      title={account.last_error || connectionLabel}
                    />
                    {isRoot && (
                      <>
                        <Button
                          type='button'
                          size='icon-sm'
                          variant='ghost'
                          aria-label={t('Edit upstream account')}
                          onClick={() => openEditDialog(account)}
                        >
                          <Pencil />
                        </Button>
                        <Button
                          type='button'
                          size='icon-sm'
                          variant='ghost'
                          aria-label={t('Delete upstream account')}
                          onClick={() => setDeleteTarget(account)}
                        >
                          <Trash2 className='text-destructive' />
                        </Button>
                      </>
                    )}
                  </div>
                </div>
              </CardHeader>
              <CardContent className='space-y-2'>
                {account.last_error && (
                  <p className='text-destructive border-destructive/30 bg-destructive/5 flex items-start gap-2 rounded-md border px-3 py-2 text-xs'>
                    <AlertTriangle className='mt-0.5 size-3.5 shrink-0' />
                    <span>{translateServerText(t, account.last_error)}</span>
                  </p>
                )}
                <div className='mt-2 text-center'>
                  <div className='text-muted-foreground text-xs'>
                    {t('Current balance')}
                  </div>
                  <div className='mt-0.5 truncate text-2xl font-semibold tabular-nums'>
                    {symbolIndex < 0 ? (
                      formattedBalance
                    ) : (
                      <>
                        {formattedBalance.slice(0, symbolIndex)}
                        <span className='me-1 text-sm font-medium'>
                          {currencySymbol}
                        </span>
                        {formattedBalance.slice(
                          symbolIndex + currencySymbol.length
                        )}
                      </>
                    )}
                  </div>
                </div>
                <div className='grid grid-cols-2 border-t pt-2'>
                  <div className='min-w-0 border-r pr-3'>
                    <Metric
                      label={t('Used')}
                      value={
                        account.last_synced_at
                          ? formatBillingCurrencyFromUSD(account.used_quota)
                          : '—'
                      }
                    />
                  </div>
                  <div className='min-w-0 pl-3'>
                    <Metric
                      label={t('Requests')}
                      value={formatNumber(account.request_count)}
                    />
                  </div>
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>
    )
  }

  function openCreateDialog() {
    setEditing(null)
    setForm(EMPTY_FORM)
    setDialogOpen(true)
  }

  function openEditDialog(account: UpstreamBalanceAccount) {
    setEditing(account)
    setForm({
      name: account.name,
      api_base_url: account.api_base_url,
      user_id: account.user_id,
      access_token: '',
      enabled: account.enabled,
    })
    setDialogOpen(true)
  }

  function updateForm<K extends keyof UpstreamBalanceAccountInput>(
    key: K,
    value: UpstreamBalanceAccountInput[K]
  ) {
    setForm((current) => ({ ...current, [key]: value }))
  }

  function submitForm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    saveMutation.mutate(form)
  }

  function testConnection() {
    testMutation.mutate(form)
  }

  return (
    <section className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h2 className='text-lg font-semibold'>{t('Upstream Balance')}</h2>
          <p className='text-muted-foreground text-sm'>
            {t('Balances for configured upstream API accounts')}
          </p>
        </div>
        <div className='flex flex-wrap items-center justify-end gap-2'>
          {isRoot && (
            <Button type='button' size='sm' onClick={openCreateDialog}>
              <Plus data-icon='inline-start' />
              {t('Add account')}
            </Button>
          )}
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => refreshMutation.mutate()}
            disabled={refreshMutation.isPending || accountsQuery.isLoading}
          >
            <RefreshCw
              data-icon='inline-start'
              className={refreshMutation.isPending ? 'animate-spin' : undefined}
            />
            {t('Refresh')}
          </Button>
          <div role='status' className='text-xs'>
            <span className='text-muted-foreground'>{t('Last updated:')} </span>
            <span
              className={
                latestSyncedAt
                  ? 'font-medium text-emerald-700 tabular-nums dark:text-emerald-400'
                  : 'text-muted-foreground'
              }
            >
              {latestSyncedAt
                ? dayjs(latestSyncedAt).format('YYYY-MM-DD HH:mm:ss')
                : t('Not yet refreshed')}
            </span>
          </div>
        </div>
      </div>

      {accountsContent}

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) setForm(EMPTY_FORM)
        }}
      >
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {editing ? t('Edit upstream account') : t('Add upstream account')}
            </DialogTitle>
            <DialogDescription>
              {t('Account details are stored in the application database.')}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submitForm} className='space-y-4'>
            <div className='space-y-2'>
              <Label htmlFor='upstream-account-name'>{t('Account name')}</Label>
              <Input
                id='upstream-account-name'
                required
                maxLength={128}
                autoComplete='off'
                value={form.name}
                onChange={(event) => updateForm('name', event.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='upstream-account-url'>{t('API base URL')}</Label>
              <Input
                id='upstream-account-url'
                required
                type='url'
                maxLength={512}
                placeholder='https://api.example.com'
                autoComplete='url'
                value={form.api_base_url}
                onChange={(event) =>
                  updateForm('api_base_url', event.target.value)
                }
              />
              {/^http:\/\//i.test(form.api_base_url.trim()) && (
                <p role='note' className='text-destructive text-xs'>
                  {t(
                    'HTTP sends the access token without encryption. Use only with a trusted upstream.'
                  )}
                </p>
              )}
            </div>
            <div className='space-y-2'>
              <Label htmlFor='upstream-account-user-id'>
                {t('Upstream user ID')}
              </Label>
              <Input
                id='upstream-account-user-id'
                required
                maxLength={128}
                autoComplete='off'
                value={form.user_id}
                onChange={(event) => updateForm('user_id', event.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='upstream-account-token'>
                {t('Access token')}
                {editing ? ` (${t('Optional')})` : ''}
              </Label>
              <Input
                id='upstream-account-token'
                required={!editing}
                type='password'
                autoComplete='new-password'
                value={form.access_token}
                placeholder={
                  editing ? t('Leave blank to keep the current token') : ''
                }
                onChange={(event) =>
                  updateForm('access_token', event.target.value)
                }
              />
            </div>
            <div className='flex items-center gap-2'>
              <Checkbox
                id='upstream-account-enabled'
                checked={form.enabled}
                onCheckedChange={(checked) =>
                  updateForm('enabled', checked === true)
                }
              />
              <Label htmlFor='upstream-account-enabled'>{t('Enabled')}</Label>
            </div>
            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={testConnection}
                disabled={
                  testMutation.isPending ||
                  !form.name ||
                  !form.api_base_url ||
                  !form.user_id ||
                  (!editing && !form.access_token) ||
                  (Boolean(editing && !form.access_token) &&
                    !canTestSavedAccount)
                }
              >
                {testMutation.isPending
                  ? t('Testing...')
                  : t('Test connection')}
              </Button>
              <Button type='submit' disabled={saveMutation.isPending}>
                {saveMutation.isPending ? t('Saving...') : t('Save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('Delete upstream account?')}
        desc={t('This account and its stored credential will be removed.')}
        destructive
        confirmText={t('Delete')}
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
      />
    </section>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div
        className='mt-1 truncate text-sm font-medium tabular-nums'
        title={value}
      >
        {value}
      </div>
    </div>
  )
}
