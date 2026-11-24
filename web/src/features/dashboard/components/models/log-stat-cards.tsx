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
import { useQuery } from '@tanstack/react-query'
import { TrendingUp, type LucideIcon } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { getProfitAnalytics, getUserQuotaDates } from '@/features/dashboard/api'
import { useModelStatCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import {
  buildQueryParams,
  calculateDashboardStats,
  getDefaultDays,
} from '@/features/dashboard/lib'
import type {
  QuotaDataItem,
  DashboardFilters,
} from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { formatCompactNumber, formatNumber, formatQuota } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

interface LogStatCardsProps {
  filters?: DashboardFilters
  onDataUpdate?: (data: QuotaDataItem[], loading: boolean) => void
}

const MAX_INLINE_STAT_CHARS = 7

interface LogStatCardItem {
  key: string
  title: string
  value: string
  fullValue: string
  desc: string
  icon: LucideIcon
  iconTone: IconBadgeTone
  loading?: boolean
  error?: boolean
}

function formatStatNumber(value: number, locale: Intl.LocalesArgument) {
  const fullValue = formatNumber(value, locale)
  const displayValue =
    fullValue.length > MAX_INLINE_STAT_CHARS
      ? formatCompactNumber(value, locale)
      : fullValue

  return {
    displayValue,
    fullValue,
  }
}

export function LogStatCards(props: LogStatCardsProps) {
  const { t, i18n } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = !!(user?.role && user.role >= 10)
  const statCardsConfig = useModelStatCardsConfig()
  const [stats, setStats] = useState<ReturnType<
    typeof calculateDashboardStats
  > | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const [timeRangeMinutes, setTimeRangeMinutes] = useState(0)

  const { filters, onDataUpdate } = props
  const timeRange = useMemo(
    () =>
      computeTimeRange(
        getDefaultDays(filters?.time_granularity),
        filters?.start_timestamp,
        filters?.end_timestamp
      ),
    [
      filters?.end_timestamp,
      filters?.start_timestamp,
      filters?.time_granularity,
    ]
  )
  const queryParams = useMemo(
    () => buildQueryParams(timeRange, filters),
    [filters, timeRange]
  )
  const profitQuery = useQuery({
    queryKey: [
      'dashboard-model-profit',
      timeRange.start_timestamp,
      timeRange.end_timestamp,
      filters?.username,
    ],
    queryFn: async () =>
      requireServerSuccess(
        await getProfitAnalytics({
          start_timestamp: timeRange.start_timestamp,
          end_timestamp: timeRange.end_timestamp,
          username: filters?.username,
        })
      ),
    enabled: isAdmin,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })

  useEffect(() => {
    const abortController = new AbortController()
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)

    setError(false)
    onDataUpdate?.([], true)

    const timeDiff = (timeRange.end_timestamp - timeRange.start_timestamp) / 60
    setTimeRangeMinutes(timeDiff)

    void getUserQuotaDates(queryParams, isAdmin)
      .then((res) => {
        if (abortController.signal.aborted) return
        const data = res?.data || []
        setStats(calculateDashboardStats(data))
        onDataUpdate?.(data, false)
      })
      .catch(() => {
        if (abortController.signal.aborted) return
        setStats(null)
        setError(true)
        onDataUpdate?.([], false)
      })
      .finally(() => {
        if (!abortController.signal.aborted) {
          setLoading(false)
        }
      })

    return () => {
      abortController.abort()
    }
  }, [isAdmin, onDataUpdate, queryParams, timeRange])

  const adaptedStats = {
    rpm: stats?.totalCount ?? 0,
    quota: stats?.totalQuota ?? 0,
    tpm: stats?.totalTokens ?? 0,
    cacheRead: stats?.totalCacheReadTokens ?? 0,
    cacheCreation: stats?.totalCacheCreationTokens ?? 0,
  }

  const items: LogStatCardItem[] = statCardsConfig.map((config) => {
    const rawValue = config.getValue(adaptedStats, timeRangeMinutes)
    const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
    const formatted =
      config.key === 'quota'
        ? {
            displayValue: formatQuota(rawValue),
            fullValue: formatQuota(rawValue),
          }
        : formatStatNumber(rawValue, locale)

    return {
      key: config.key,
      title: config.title,
      value: formatted.displayValue,
      fullValue: formatted.fullValue,
      desc: config.description,
      icon: config.icon,
      iconTone: config.iconTone,
    }
  })
  if (isAdmin) {
    const profitSummary = profitQuery.data?.summary
    const costConfigured = Number(profitSummary?.cost_configured ?? 1) !== 0
    const totalProfit = Number(profitSummary?.total_profit ?? 0) || 0
    const formattedProfit = formatBillingCurrencyFromUSD(totalProfit, {
      digitsLarge: 4,
      digitsSmall: 4,
      abbreviate: false,
    })
    const profitValue = costConfigured
      ? formattedProfit
      : t('Cost not configured')

    items.splice(2, 0, {
      key: 'profit',
      title: t('Total Profit'),
      value: profitValue,
      fullValue: profitValue,
      desc: t('Net profit'),
      icon: TrendingUp,
      iconTone: 'warning',
      loading: profitQuery.isLoading,
      error: profitQuery.isError,
    })
  }

  return (
    <div className='sm:overflow-hidden sm:rounded-lg sm:border'>
      <div
        className={cn(
          'grid min-w-0 grid-cols-2 gap-2 p-2 sm:grid-cols-3 sm:gap-0 sm:divide-x sm:divide-border/60 sm:p-0',
          isAdmin ? 'lg:grid-cols-8' : 'lg:grid-cols-7'
        )}
      >
        {items.map((it, idx) => {
          const Icon = it.icon
          let valueContent
          if (loading || it.loading) {
            valueContent = (
              <div className='mt-1 flex flex-col gap-1 sm:mt-2 sm:gap-1.5'>
                <Skeleton className='h-5 w-16 sm:h-7 sm:w-20' />
                <Skeleton className='hidden h-3.5 w-28 md:block' />
              </div>
            )
          } else if (error || it.error) {
            valueContent = (
              <>
                <div className='text-muted-foreground mt-1 font-mono text-base leading-tight font-bold tracking-tight tabular-nums sm:mt-2 sm:text-2xl sm:leading-normal'>
                  --
                </div>
                <div className='text-muted-foreground/40 mt-1 hidden text-xs md:block'>
                  {it.desc}
                </div>
              </>
            )
          } else {
            valueContent = (
              <>
                <div
                  className='text-foreground mt-1 max-w-full truncate font-mono text-base leading-tight font-bold tracking-tight tabular-nums sm:mt-2 sm:text-2xl sm:leading-normal'
                  title={it.fullValue}
                >
                  {it.value}
                </div>
                <div className='text-muted-foreground/60 mt-1 hidden text-xs md:block'>
                  {it.desc}
                </div>
              </>
            )
          }

          return (
            <div
              key={it.key}
              role='group'
              aria-label={it.title}
              aria-busy={loading || Boolean(it.loading)}
              className={cn(
                'min-w-0 rounded-md border bg-muted/20 px-2.5 py-2 sm:rounded-none sm:border-0 sm:bg-transparent sm:px-3 sm:py-4 xl:px-4',
                idx === items.length - 1 &&
                  items.length % 2 !== 0 &&
                  'col-span-2 sm:col-span-1'
              )}
            >
              <div className='flex min-w-0 items-center gap-1.5 sm:gap-2'>
                <IconBadge
                  tone={it.iconTone}
                  size='stat'
                  className='size-4 rounded-sm sm:size-7 sm:rounded-md [&>svg]:size-2.5 sm:[&>svg]:size-3.5'
                >
                  <Icon />
                </IconBadge>
                <div
                  className='text-muted-foreground truncate text-[11px] leading-4 font-medium tracking-wide uppercase sm:text-xs sm:tracking-wider'
                  title={it.title}
                >
                  {it.title}
                </div>
              </div>

              <div className='min-w-0 pl-[22px] sm:pl-0'>{valueContent}</div>
            </div>
          )
        })}
      </div>
      {/*<p className='text-muted-foreground border-t px-3 py-2 text-xs'>*/}
      {/*  {t(*/}
      {/*    'Cache usage is recorded only after the upgrade; historical data is not backfilled.'*/}
      {/*  )}*/}
      {/*</p>*/}
    </div>
  )
}
