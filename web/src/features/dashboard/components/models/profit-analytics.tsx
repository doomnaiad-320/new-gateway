import { useQuery } from '@tanstack/react-query'
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
import { VChart } from '@visactor/react-vchart'
import { ChevronRight, RefreshCw } from 'lucide-react'
import { Fragment, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useTheme } from '@/context/theme-provider'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import dayjs from '@/lib/dayjs'
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { VCHART_OPTION } from '@/lib/vchart'

import { getProfitAnalytics, type ProfitAnalyticsPoint } from '../../api'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

type UpstreamCostSummary = {
  channelId: number
  name: string
  requests: number
  cost: number
  costConfigured: boolean
}

type ModelProfitSummary = {
  name: string
  requests: number
  revenue: number
  cost: number
  profit: number
  costConfigured: boolean
  upstreamModels: Map<string, UpstreamCostSummary>
}

function getTodayRange(): { start: Date; end: Date } {
  const end = new Date()
  const start = new Date(end)
  start.setHours(0, 0, 0, 0)
  return { start, end }
}

function getLast30DaysRange(): { start: Date; end: Date } {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - 29)
  start.setHours(0, 0, 0, 0)
  return { start, end }
}

function getModelSummaries(
  points: ProfitAnalyticsPoint[]
): ModelProfitSummary[] {
  const models = new Map<string, ModelProfitSummary>()
  for (const point of points) {
    let model = models.get(point.model_name)
    if (!model) {
      model = {
        name: point.model_name,
        requests: 0,
        revenue: 0,
        cost: 0,
        profit: 0,
        costConfigured: true,
        upstreamModels: new Map(),
      }
      models.set(point.model_name, model)
    }

    model.requests += point.count
    model.revenue += point.revenue
    model.costConfigured = model.costConfigured && point.cost_configured
    if (point.cost_configured) {
      model.cost += point.cost
      model.profit += point.profit
    }

    for (const upstream of point.upstream_models) {
      const key = `${upstream.channel_id}\u0000${upstream.upstream_model_name}`
      let detail = model.upstreamModels.get(key)
      if (!detail) {
        detail = {
          channelId: upstream.channel_id,
          name: upstream.upstream_model_name,
          requests: 0,
          cost: 0,
          costConfigured: true,
        }
        model.upstreamModels.set(key, detail)
      }
      detail.requests += upstream.count
      detail.costConfigured = detail.costConfigured && upstream.cost_configured
      if (upstream.cost_configured) detail.cost += upstream.cost
    }
  }

  return [...models.values()]
    .map((model) => ({
      ...model,
      upstreamModels: new Map(
        [...model.upstreamModels.entries()].sort((a, b) => {
          if (a[1].channelId !== b[1].channelId) {
            return a[1].channelId - b[1].channelId
          }
          return a[1].name.localeCompare(b[1].name)
        })
      ),
    }))
    .sort((a, b) => b.revenue - a.revenue)
}

function getDailyProfit(points: ProfitAnalyticsPoint[]) {
  const daily = new Map<
    string,
    { date: string; revenue: number; profit: number | null }
  >()
  for (const point of points) {
    const date = dayjs(point.created_at * 1000).format('YYYY-MM-DD')
    let item = daily.get(date)
    if (!item) {
      item = { date, revenue: 0, profit: 0 }
      daily.set(date, item)
    }
    item.revenue += point.revenue
    if (point.cost_configured && item.profit !== null) {
      item.profit += point.profit
    } else {
      item.profit = null
    }
  }
  return [...daily.values()]
}

function formatAmount(value: number): string {
  return formatBillingCurrencyFromUSD(value, {
    digitsLarge: 4,
    digitsSmall: 4,
    abbreviate: false,
  })
}

function ProfitAmount(props: { value: number; className?: string }) {
  return <span className={props.className}>{formatAmount(props.value)}</span>
}

export function ProfitAnalytics() {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [range, setRange] = useState(getTodayRange)
  const [expandedModels, setExpandedModels] = useState<Set<string>>(
    () => new Set()
  )
  const [themeReady, setThemeReady] = useState(false)
  const start = Math.floor(range.start.getTime() / 1000)
  const end = Math.floor(range.end.getTime() / 1000)
  const query = useQuery({
    queryKey: ['profit-analytics', start, end],
    queryFn: async () =>
      requireServerSuccess(
        await getProfitAnalytics({ start_timestamp: start, end_timestamp: end })
      ),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })
  const trendRange = useMemo(() => getLast30DaysRange(), [])
  const trendQuery = useQuery({
    queryKey: [
      'profit-analytics-trend',
      trendRange.start.getTime(),
      trendRange.end.getTime(),
    ],
    queryFn: async () =>
      requireServerSuccess(
        await getProfitAnalytics({
          start_timestamp: Math.floor(trendRange.start.getTime() / 1000),
          end_timestamp: Math.floor(trendRange.end.getTime() / 1000),
        })
      ),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  })
  const response = query.data
  const points = response?.data
  const summary = response?.summary ?? {}
  const currentHour = response?.current_hour ?? {}
  const models = useMemo(() => getModelSummaries(points ?? []), [points])
  const daily = useMemo(
    () => getDailyProfit(trendQuery.data?.data ?? []),
    [trendQuery.data?.data]
  )
  const chartSpec = useMemo(
    () => ({
      type: 'area' as const,
      data: [
        {
          id: 'profitTrend',
          values: daily.map((item) => ({
            Date: item.date,
            Profit: item.profit,
          })),
        },
      ],
      xField: 'Date',
      yField: 'Profit',
      color: ['#d97706'],
      legends: { visible: false },
      title: { visible: false },
      tooltip: {
        mark: {
          content: [
            {
              key: t('Net profit'),
              value: (datum: Record<string, unknown>) =>
                datum.Profit == null
                  ? t('Cost not configured')
                  : formatAmount(Number(datum.Profit) || 0),
            },
          ],
        },
      },
      area: { style: { fillOpacity: 0.1, curveType: 'monotone' as const } },
      line: { style: { lineWidth: 2, curveType: 'monotone' as const } },
      point: { visible: false },
      padding: { top: 12, right: 16, bottom: 24, left: 12 },
      background: { fill: 'transparent' },
      animation: false,
    }),
    [daily, t]
  )

  useEffect(() => {
    let active = true
    const loadTheme = async () => {
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (module) => module.ThemeManager
        )
      }
      const themeManager = await themeManagerPromise
      if (!active) return
      themeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    void loadTheme()
    return () => {
      active = false
    }
  }, [resolvedTheme])

  const updateRange = (next: { start?: Date; end?: Date }) => {
    if (next.start && next.end && next.start <= next.end) {
      setRange({ start: next.start, end: next.end })
    }
  }

  const toggleModel = (name: string) => {
    setExpandedModels((current) => {
      const next = new Set(current)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const costConfigured = summary.cost_configured !== 0
  const currentHourConfigured = currentHour.cost_configured !== 0

  let trendContent: ReactNode
  if (trendQuery.isError) {
    trendContent = (
      <div className='text-destructive flex h-full items-center justify-center text-sm'>
        {t('Failed to load profit data. Please try again.')}
      </div>
    )
  } else if (trendQuery.isLoading) {
    trendContent = (
      <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
        {t('Loading profit analytics...')}
      </div>
    )
  } else if (daily.length === 0) {
    trendContent = (
      <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
        {t('No usage data for this period.')}
      </div>
    )
  } else if (themeReady) {
    trendContent = (
      <VChart
        key={`${resolvedTheme}-${daily.length}`}
        spec={{
          ...chartSpec,
          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
          background: 'transparent',
        }}
        option={VCHART_OPTION}
      />
    )
  }

  let mainContent: ReactNode
  if (query.isError) {
    mainContent = (
      <div className='text-destructive rounded-lg border p-5 text-sm'>
        {t('Failed to load profit data. Please try again.')}
      </div>
    )
  } else if (query.isLoading) {
    mainContent = (
      <div className='text-muted-foreground rounded-lg border p-5 text-sm'>
        {t('Loading profit analytics...')}
      </div>
    )
  } else {
    mainContent = (
      <>
        <div className='grid grid-cols-2 gap-3 xl:grid-cols-4'>
          {[
            {
              key: 'total_revenue',
              label: 'Revenue',
              tone: 'text-emerald-600',
            },
            { key: 'total_cost', label: 'Cost', tone: 'text-rose-600' },
            {
              key: 'total_profit',
              label: 'Net profit',
              tone: 'text-amber-600',
            },
          ].map((metric) => (
            <div
              key={metric.key}
              className='min-w-0 rounded-lg border p-3 sm:p-4'
            >
              <div className='text-muted-foreground text-xs sm:text-sm'>
                {t(metric.label)}
              </div>
              <div
                className={`mt-1 truncate text-lg font-semibold tabular-nums sm:text-xl ${metric.tone}`}
              >
                {metric.key === 'total_revenue' || costConfigured ? (
                  <ProfitAmount value={summary[metric.key] ?? 0} />
                ) : (
                  t('Cost not configured')
                )}
              </div>
            </div>
          ))}
          <div className='min-w-0 rounded-lg border p-3 sm:p-4'>
            <div className='text-muted-foreground text-xs sm:text-sm'>
              {t('Current hour net profit')}
            </div>
            <div className='mt-1 truncate text-lg font-semibold tabular-nums sm:text-xl'>
              {currentHourConfigured ? (
                <ProfitAmount value={currentHour.total_profit ?? 0} />
              ) : (
                t('Cost not configured')
              )}
            </div>
          </div>
        </div>

        {!costConfigured && (
          <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-2 text-sm'>
            {t(
              'Some upstream model costs are missing. Configure them in Billing Settings to complete profit analysis.'
            )}
          </div>
        )}

        <section className='overflow-hidden rounded-lg border'>
          <div className='border-b px-3 py-2.5 sm:px-4'>
            <h2 className='text-sm font-semibold'>
              {t('Daily trend (30 days)')}
            </h2>
          </div>
          <div className='h-[260px] p-2 sm:h-80'>{trendContent}</div>
        </section>

        <section className='overflow-hidden rounded-lg border'>
          <div className='border-b px-3 py-2.5 sm:px-4'>
            <h2 className='text-sm font-semibold'>
              {t('Model profit details')}
            </h2>
          </div>
          <Table>
            <TableHeader className='bg-muted/50'>
              <TableRow>
                <TableHead>{t('Request model')}</TableHead>
                <TableHead className='text-right'>{t('Requests')}</TableHead>
                <TableHead className='text-right'>{t('Revenue')}</TableHead>
                <TableHead className='text-right'>{t('Cost')}</TableHead>
                <TableHead className='text-right'>{t('Net profit')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {models.map((model) => (
                <Fragment key={model.name}>
                  <TableRow>
                    <TableCell>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          className='size-7 shrink-0'
                          aria-label={t(
                            'Toggle upstream model details for {{model}}',
                            { model: model.name }
                          )}
                          aria-expanded={expandedModels.has(model.name)}
                          onClick={() => toggleModel(model.name)}
                        >
                          <ChevronRight
                            className={
                              expandedModels.has(model.name)
                                ? 'rotate-90 transition-transform'
                                : 'transition-transform'
                            }
                          />
                        </Button>
                        <span className='truncate font-medium'>
                          {model.name}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className='text-right'>
                      {formatNumber(model.requests)}
                    </TableCell>
                    <TableCell className='text-right'>
                      <ProfitAmount value={model.revenue} />
                    </TableCell>
                    <TableCell className='text-right'>
                      {model.costConfigured ? (
                        <ProfitAmount value={model.cost} />
                      ) : (
                        t('Cost not configured')
                      )}
                    </TableCell>
                    <TableCell className='text-right'>
                      {model.costConfigured ? (
                        <ProfitAmount value={model.profit} />
                      ) : (
                        t('Cost not configured')
                      )}
                    </TableCell>
                  </TableRow>
                  {expandedModels.has(model.name) &&
                    [...model.upstreamModels.values()].map((upstream) => (
                      <TableRow
                        key={`${model.name}-${upstream.channelId}-${upstream.name}`}
                        className='bg-muted/20'
                      >
                        <TableCell>
                          <div className='pl-9'>
                            <div className='text-muted-foreground truncate text-xs'>
                              {upstream.name || t('Unknown upstream model')}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              {t('Channel ID')}: {upstream.channelId}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className='text-right text-xs'>
                          {formatNumber(upstream.requests)}
                        </TableCell>
                        <TableCell />
                        <TableCell className='text-right text-xs'>
                          {upstream.costConfigured ? (
                            <ProfitAmount value={upstream.cost} />
                          ) : (
                            t('Cost not configured')
                          )}
                        </TableCell>
                        <TableCell />
                      </TableRow>
                    ))}
                </Fragment>
              ))}
              {models.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className='text-muted-foreground h-20 text-center'
                  >
                    {t('No usage data for this period.')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </section>
      </>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <CompactDateTimeRangePicker
          start={range.start}
          end={range.end}
          onChange={updateRange}
          className='w-full sm:w-auto'
        />
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() =>
            void Promise.all([query.refetch(), trendQuery.refetch()])
          }
          disabled={query.isFetching || trendQuery.isFetching}
        >
          <RefreshCw
            className={
              query.isFetching || trendQuery.isFetching
                ? 'animate-spin'
                : undefined
            }
            data-icon='inline-start'
          />
          {t('Refresh')}
        </Button>
      </div>

      {mainContent}
    </div>
  )
}
