import { Button } from '@/components/ui/button'
import type { ScanResult } from '@/lib/api'

interface ExportButtonProps {
  result: ScanResult
}

export function ExportButton({ result }: ExportButtonProps) {
  const exportCSV = () => {
    const headers = ['address', 'type', 'inferred_type', 'is_dynamic', 'value_min', 'value_max', 'float32_value', 'paired_address', 'category', 'reason']
    const rows = result.registers.map(r => [
      r.address,
      r.type,
      r.inferred_type,
      r.is_dynamic,
      r.value_range.min,
      r.value_range.max,
      r.float32_value ?? '',
      r.paired_address ?? '',
      r.guess?.category ?? '',
      r.guess?.reason ?? '',
    ])

    const csv = [headers, ...rows].map(row => row.map(v => `"${v}"`).join(',')).join('\n')
    download(csv, `modbus-scan-${result.device.replace(':', '-')}.csv`, 'text/csv')
  }

  const exportJSON = () => {
    const json = JSON.stringify(result, null, 2)
    download(json, `modbus-scan-${result.device.replace(':', '-')}.json`, 'application/json')
  }

  return (
    <div className="flex gap-2">
      <Button variant="secondary" size="sm" onClick={exportCSV} className="font-mono text-xs">
        CSV
      </Button>
      <Button variant="secondary" size="sm" onClick={exportJSON} className="font-mono text-xs">
        JSON
      </Button>
    </div>
  )
}

function download(content: string, filename: string, type: string) {
  const blob = new Blob([content], { type })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}
