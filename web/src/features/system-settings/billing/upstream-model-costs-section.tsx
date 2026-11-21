import { useQueryClient } from '@tanstack/react-query'
import { Plus, Save, Trash2 } from 'lucide-react'
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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { useUpdateOption } from '../hooks/use-update-option'

type CostRule = {
  id: string
  model_name: string
  channel_id: string
  upstream_model_name: string
  cost_per_request: string
}

function parseRules(value: string): CostRule[] {
  try {
    const parsed: unknown = JSON.parse(value)
    if (!Array.isArray(parsed)) return []
    return parsed.map((rule, index) => {
      const item = typeof rule === 'object' && rule !== null ? rule : {}
      return {
        id: String(index),
        model_name: String(Reflect.get(item, 'model_name') ?? ''),
        channel_id: String(Reflect.get(item, 'channel_id') ?? ''),
        upstream_model_name: String(
          Reflect.get(item, 'upstream_model_name') ?? ''
        ),
        cost_per_request: String(Reflect.get(item, 'cost_per_request') ?? ''),
      }
    })
  } catch {
    return []
  }
}

function serializeRules(rules: CostRule[]): string {
  return JSON.stringify(
    rules.map((rule) => ({
      model_name: rule.model_name.trim(),
      channel_id: Number(rule.channel_id),
      upstream_model_name: rule.upstream_model_name.trim(),
      cost_per_request: Number(rule.cost_per_request),
    }))
  )
}

export function UpstreamModelCostsSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const initialRules = useMemo(
    () => parseRules(props.defaultValue),
    [props.defaultValue]
  )
  const nextRuleId = useRef(initialRules.length)
  const [rules, setRules] = useState(initialRules)
  const [savedValue, setSavedValue] = useState(() =>
    serializeRules(initialRules)
  )

  useEffect(() => {
    nextRuleId.current = Math.max(nextRuleId.current, initialRules.length)
    setRules(initialRules)
    setSavedValue(serializeRules(initialRules))
  }, [initialRules])

  const value = serializeRules(rules)
  const keys = rules.map(
    (rule) =>
      `${rule.model_name.trim()}\u0000${rule.channel_id.trim()}\u0000${rule.upstream_model_name.trim()}`
  )
  const duplicateKeys = new Set(
    keys.filter((key, index) => keys.indexOf(key) !== index)
  )
  const valid = rules.every(
    (rule) =>
      rule.model_name.trim() !== '' &&
      /^\d+$/.test(rule.channel_id) &&
      Number(rule.channel_id) > 0 &&
      rule.upstream_model_name.trim() !== '' &&
      rule.cost_per_request.trim() !== '' &&
      Number.isFinite(Number(rule.cost_per_request)) &&
      Number(rule.cost_per_request) >= 0
  )
  const dirty = value !== savedValue

  const updateRule = (index: number, patch: Partial<CostRule>) => {
    setRules((current) =>
      current.map((rule, ruleIndex) =>
        ruleIndex === index ? { ...rule, ...patch } : rule
      )
    )
  }

  const save = async () => {
    await updateOption.mutateAsync({ key: 'ProfitUpstreamCosts', value })
    setSavedValue(value)
    await queryClient.invalidateQueries({ queryKey: ['profit-analytics'] })
    await queryClient.invalidateQueries({
      queryKey: ['profit-analytics-trend'],
    })
  }

  return (
    <section className='space-y-4'>
      <div className='text-muted-foreground space-y-1 text-sm'>
        <p>
          {t(
            'Add one rule per request model, channel ID, and upstream model. Costs are in USD per request.'
          )}
        </p>
        <p>
          {t(
            'Cost rules are stored in the database and apply to historical and future analytics.'
          )}
        </p>
        <p>
          {t(
            'Historical rows without an upstream model cannot be matched to a cost rule.'
          )}
        </p>
      </div>

      <div className='overflow-hidden rounded-lg border'>
        <Table className='min-w-[760px]'>
          <TableHeader className='bg-muted/50'>
            <TableRow>
              <TableHead>{t('Request model')}</TableHead>
              <TableHead>{t('Channel ID')}</TableHead>
              <TableHead>{t('Upstream model')}</TableHead>
              <TableHead>{t('Cost per request (USD)')}</TableHead>
              <TableHead className='w-12' />
            </TableRow>
          </TableHeader>
          <TableBody>
            {rules.map((rule, index) => {
              const key = `${rule.model_name.trim()}\u0000${rule.channel_id.trim()}\u0000${rule.upstream_model_name.trim()}`
              const duplicate = duplicateKeys.has(key)
              return (
                <TableRow key={rule.id}>
                  <TableCell>
                    <Input
                      aria-label={t('Request model')}
                      aria-invalid={rule.model_name.trim() === ''}
                      value={rule.model_name}
                      placeholder='model-name'
                      onChange={(event) =>
                        updateRule(index, { model_name: event.target.value })
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      aria-label={t('Channel ID')}
                      aria-invalid={
                        duplicate ||
                        !/^\d+$/.test(rule.channel_id) ||
                        Number(rule.channel_id) <= 0
                      }
                      type='number'
                      min='1'
                      step='1'
                      value={rule.channel_id}
                      onChange={(event) =>
                        updateRule(index, { channel_id: event.target.value })
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      aria-label={t('Upstream model')}
                      aria-invalid={rule.upstream_model_name.trim() === ''}
                      value={rule.upstream_model_name}
                      placeholder='provider-model'
                      onChange={(event) =>
                        updateRule(index, {
                          upstream_model_name: event.target.value,
                        })
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      aria-label={t('Cost per request (USD)')}
                      aria-invalid={
                        rule.cost_per_request.trim() === '' ||
                        !Number.isFinite(Number(rule.cost_per_request)) ||
                        Number(rule.cost_per_request) < 0
                      }
                      type='number'
                      min='0'
                      step='any'
                      value={rule.cost_per_request}
                      onChange={(event) =>
                        updateRule(index, {
                          cost_per_request: event.target.value,
                        })
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon'
                      aria-label={t('Remove row')}
                      title={t('Remove row')}
                      onClick={() =>
                        setRules((current) =>
                          current.filter((_, rowIndex) => rowIndex !== index)
                        )
                      }
                    >
                      <Trash2 />
                    </Button>
                  </TableCell>
                </TableRow>
              )
            })}
            {rules.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className='text-muted-foreground h-20 text-center'
                >
                  {t('No upstream cost rules configured.')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className='flex flex-wrap items-center justify-between gap-2'>
        <Button
          type='button'
          variant='outline'
          onClick={() =>
            setRules((current) => [
              ...current,
              {
                id: String(nextRuleId.current++),
                model_name: '',
                channel_id: '',
                upstream_model_name: '',
                cost_per_request: '',
              },
            ])
          }
        >
          <Plus data-icon='inline-start' />
          {t('Add rule')}
        </Button>
        <Button
          type='button'
          onClick={() => void save()}
          disabled={
            !dirty || !valid || duplicateKeys.size > 0 || updateOption.isPending
          }
        >
          <Save data-icon='inline-start' />
          {t('Save')}
        </Button>
      </div>
    </section>
  )
}
