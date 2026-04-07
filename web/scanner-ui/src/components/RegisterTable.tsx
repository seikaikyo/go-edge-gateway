import { useState, useMemo } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AnalyzedRegister, ScanResult } from '@/lib/api'

interface RegisterTableProps {
  result: ScanResult
}

type SortKey = 'address' | 'inferred_type' | 'is_dynamic' | 'category'
type SortDir = 'asc' | 'desc'

export function RegisterTable({ result }: RegisterTableProps) {
  const [search, setSearch] = useState('')
  const [filterType, setFilterType] = useState<string | null>(null)
  const [filterDynamic, setFilterDynamic] = useState<boolean | null>(null)
  const [sortKey, setSortKey] = useState<SortKey>('address')
  const [sortDir, setSortDir] = useState<SortDir>('asc')

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir(d => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortKey(key)
      setSortDir('asc')
    }
  }

  const filtered = useMemo(() => {
    let regs = result.registers

    if (search) {
      const q = search.toLowerCase()
      regs = regs.filter(
        r =>
          String(r.address).includes(q) ||
          r.inferred_type.includes(q) ||
          r.guess?.category.includes(q) ||
          r.guess?.reason.includes(q)
      )
    }

    if (filterType) {
      regs = regs.filter(r => r.inferred_type === filterType)
    }

    if (filterDynamic !== null) {
      regs = regs.filter(r => r.is_dynamic === filterDynamic)
    }

    regs = [...regs].sort((a, b) => {
      let cmp = 0
      switch (sortKey) {
        case 'address':
          cmp = a.address - b.address
          break
        case 'inferred_type':
          cmp = a.inferred_type.localeCompare(b.inferred_type)
          break
        case 'is_dynamic':
          cmp = (a.is_dynamic ? 1 : 0) - (b.is_dynamic ? 1 : 0)
          break
        case 'category':
          cmp = (a.guess?.category || '').localeCompare(b.guess?.category || '')
          break
      }
      return sortDir === 'asc' ? cmp : -cmp
    })

    return regs
  }, [result.registers, search, filterType, filterDynamic, sortKey, sortDir])

  const types = useMemo(
    () => [...new Set(result.registers.map(r => r.inferred_type))],
    [result.registers]
  )

  return (
    <Card className="bg-card border-border">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
            Register Map
          </CardTitle>
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span>{result.summary.responsive} registers</span>
            <span className="text-success">{result.summary.dynamic} dynamic</span>
            <span>{result.summary.static} static</span>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {/* Filters */}
        <div className="flex gap-2 items-center">
          <Input
            placeholder="Search address, type, category..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="bg-background font-mono text-xs h-8 max-w-[260px]"
          />
          <div className="flex gap-1">
            {types.map(t => (
              <button
                key={t}
                onClick={() => setFilterType(filterType === t ? null : t)}
                className={`px-2 py-1 rounded text-[10px] font-mono transition-colors ${
                  filterType === t
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-secondary text-muted-foreground hover:text-foreground'
                }`}
              >
                {t}
              </button>
            ))}
          </div>
          <div className="flex gap-1">
            <button
              onClick={() => setFilterDynamic(filterDynamic === true ? null : true)}
              className={`px-2 py-1 rounded text-[10px] font-mono transition-colors ${
                filterDynamic === true
                  ? 'bg-success/30 text-success'
                  : 'bg-secondary text-muted-foreground hover:text-foreground'
              }`}
            >
              dynamic
            </button>
            <button
              onClick={() => setFilterDynamic(filterDynamic === false ? null : false)}
              className={`px-2 py-1 rounded text-[10px] font-mono transition-colors ${
                filterDynamic === false
                  ? 'bg-warning/30 text-warning'
                  : 'bg-secondary text-muted-foreground hover:text-foreground'
              }`}
            >
              static
            </button>
          </div>
        </div>

        {/* Table */}
        <div className="max-h-[500px] overflow-auto rounded border border-border">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent border-border">
                <SortableHead label="Addr" sortKey="address" current={sortKey} dir={sortDir} onSort={toggleSort} />
                <TableHead className="text-xs text-muted-foreground">Type</TableHead>
                <SortableHead label="DataType" sortKey="inferred_type" current={sortKey} dir={sortDir} onSort={toggleSort} />
                <TableHead className="text-xs text-muted-foreground">Value</TableHead>
                <SortableHead label="Dynamic" sortKey="is_dynamic" current={sortKey} dir={sortDir} onSort={toggleSort} />
                <TableHead className="text-xs text-muted-foreground">Range</TableHead>
                <SortableHead label="Guess" sortKey="category" current={sortKey} dir={sortDir} onSort={toggleSort} />
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map(reg => (
                <RegisterRow key={`${reg.type}-${reg.address}`} reg={reg} />
              ))}
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-muted-foreground text-xs py-8">
                    No registers match filters
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

        <div className="text-xs text-muted-foreground">
          Showing {filtered.length} of {result.registers.length} registers
        </div>
      </CardContent>
    </Card>
  )
}

function SortableHead({
  label,
  sortKey,
  current,
  dir,
  onSort,
}: {
  label: string
  sortKey: SortKey
  current: SortKey
  dir: SortDir
  onSort: (key: SortKey) => void
}) {
  return (
    <TableHead
      className="text-xs text-muted-foreground cursor-pointer hover:text-foreground select-none"
      onClick={() => onSort(sortKey)}
    >
      {label}
      {current === sortKey && (
        <span className="ml-1">{dir === 'asc' ? '\u2191' : '\u2193'}</span>
      )}
    </TableHead>
  )
}

function RegisterRow({ reg }: { reg: AnalyzedRegister }) {
  const displayValue = reg.float32_value != null
    ? reg.float32_value.toFixed(2)
    : reg.raw_values[reg.raw_values.length - 1]

  return (
    <TableRow className="border-border hover:bg-secondary/50">
      <TableCell className="font-mono text-xs py-1.5">
        {reg.address}
        {reg.paired_address != null && (
          <span className="text-muted-foreground">-{reg.paired_address}</span>
        )}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground py-1.5">{reg.type}</TableCell>
      <TableCell className="py-1.5">
        <TypeBadge type={reg.inferred_type} />
      </TableCell>
      <TableCell className="font-mono text-xs py-1.5">{displayValue}</TableCell>
      <TableCell className="py-1.5">
        {reg.is_dynamic ? (
          <span className="inline-block w-2 h-2 rounded-full bg-success animate-pulse" />
        ) : (
          <span className="inline-block w-2 h-2 rounded-full bg-muted-foreground/30" />
        )}
      </TableCell>
      <TableCell className="font-mono text-[10px] text-muted-foreground py-1.5">
        {reg.value_range.min}-{reg.value_range.max}
      </TableCell>
      <TableCell className="py-1.5">
        {reg.guess && <CategoryBadge category={reg.guess.category} />}
      </TableCell>
    </TableRow>
  )
}

function TypeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    float32_hi: 'bg-purple-500/20 text-purple-400',
    int16: 'bg-blue-500/20 text-blue-400',
    uint16: 'bg-cyan-500/20 text-cyan-400',
    bool: 'bg-yellow-500/20 text-yellow-400',
  }
  return (
    <Badge className={`${colors[type] || 'bg-secondary text-foreground'} border-0 text-[10px]`}>
      {type}
    </Badge>
  )
}

function CategoryBadge({ category }: { category: string }) {
  const colors: Record<string, string> = {
    temperature: 'bg-red-500/20 text-red-400',
    pressure: 'bg-orange-500/20 text-orange-400',
    'pressure/level': 'bg-orange-500/20 text-orange-400',
    percentage: 'bg-green-500/20 text-green-400',
    'rpm/speed': 'bg-blue-500/20 text-blue-400',
    counter: 'bg-indigo-500/20 text-indigo-400',
    'on-off status': 'bg-yellow-500/20 text-yellow-400',
    'config/mode': 'bg-gray-500/20 text-gray-400',
    'config flag': 'bg-gray-500/20 text-gray-400',
    parameter: 'bg-gray-500/20 text-gray-400',
    measurement: 'bg-teal-500/20 text-teal-400',
  }
  return (
    <Badge className={`${colors[category] || 'bg-secondary text-foreground'} border-0 text-[10px]`}>
      {category}
    </Badge>
  )
}
