import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { api } from '@/lib/api'

interface ScanFormProps {
  onScanStarted: () => void
}

export function ScanForm({ onScanStarted }: ScanFormProps) {
  const [mode, setMode] = useState<'tcp' | 'rtu'>('tcp')

  // TCP
  const [host, setHost] = useState('192.168.1.200')
  const [port, setPort] = useState(502)

  // RTU
  const [serialPort, setSerialPort] = useState('')
  const [serialPorts, setSerialPorts] = useState<string[]>([])
  const [baudRate, setBaudRate] = useState(9600)
  const [parity, setParity] = useState('N')

  // Common
  const [unitId, setUnitId] = useState(1)
  const [addrStart, setAddrStart] = useState(0)
  const [addrEnd, setAddrEnd] = useState(9999)
  const [samples, setSamples] = useState(5)
  const [scanTypes, setScanTypes] = useState({
    holding: true,
    input: true,
    coil: false,
    discrete: false,
  })
  const [loading, setLoading] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (mode === 'rtu') {
      api.listSerialPorts().then(data => {
        setSerialPorts(data.ports)
        if (data.ports.length > 0 && !serialPort) {
          setSerialPort(data.ports[0])
        }
      }).catch(() => {})
    }
  }, [mode])

  const toggleType = (type: keyof typeof scanTypes) => {
    setScanTypes(prev => ({ ...prev, [type]: !prev[type] }))
  }

  const selectedTypes = Object.entries(scanTypes)
    .filter(([, v]) => v)
    .map(([k]) => k)

  const canSubmit = mode === 'rtu' ? !!serialPort : !!host

  const doScan = async (scanMode: 'full' | 'quick') => {
    setError(null)
    setLoading(scanMode)
    try {
      const req = {
        mode,
        ...(mode === 'tcp'
          ? { host, port }
          : { serial_port: serialPort, baud_rate: baudRate, data_bits: 8, stop_bits: 1, parity }),
        unit_id: unitId,
        scan_types: selectedTypes,
        address_start: addrStart,
        address_end: addrEnd,
        samples,
      }
      if (scanMode === 'quick') {
        await api.quickScan(req)
      } else {
        await api.scan(req)
      }
      onScanStarted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Scan failed')
    } finally {
      setLoading(null)
    }
  }

  const refreshPorts = () => {
    api.listSerialPorts().then(data => {
      setSerialPorts(data.ports)
    }).catch(() => {})
  }

  return (
    <Card className="bg-card border-border">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
          Scan Config
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Mode toggle */}
        <div>
          <Label className="text-xs text-muted-foreground">Connection</Label>
          <div className="flex gap-2 mt-1">
            {(['tcp', 'rtu'] as const).map(m => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className={`px-4 py-1.5 rounded text-xs font-mono font-medium transition-colors ${
                  mode === m
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-secondary text-muted-foreground hover:text-foreground'
                }`}
              >
                {m === 'tcp' ? 'Modbus TCP' : 'Modbus RTU'}
              </button>
            ))}
          </div>
        </div>

        {/* TCP fields */}
        {mode === 'tcp' && (
          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">
              <Label className="text-xs text-muted-foreground">Host</Label>
              <Input
                value={host}
                onChange={e => setHost(e.target.value)}
                placeholder="192.168.1.200"
                className="bg-background font-mono text-sm"
              />
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">Port</Label>
              <Input
                type="number"
                value={port}
                onChange={e => setPort(Number(e.target.value))}
                className="bg-background font-mono text-sm"
              />
            </div>
          </div>
        )}

        {/* RTU fields */}
        {mode === 'rtu' && (
          <div className="space-y-3">
            <div>
              <div className="flex items-center justify-between">
                <Label className="text-xs text-muted-foreground">Serial Port</Label>
                <button
                  onClick={refreshPorts}
                  className="text-xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  Refresh
                </button>
              </div>
              {serialPorts.length > 0 ? (
                <div className="flex flex-col gap-1 mt-1">
                  {serialPorts.map(p => (
                    <button
                      key={p}
                      onClick={() => setSerialPort(p)}
                      className={`px-3 py-1.5 rounded text-xs font-mono text-left transition-colors ${
                        serialPort === p
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-secondary text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      {p}
                    </button>
                  ))}
                </div>
              ) : (
                <Input
                  value={serialPort}
                  onChange={e => setSerialPort(e.target.value)}
                  placeholder="/dev/cu.usbserial-xxx"
                  className="bg-background font-mono text-sm mt-1"
                />
              )}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label className="text-xs text-muted-foreground">Baud Rate</Label>
                <div className="flex gap-1 mt-1 flex-wrap">
                  {[9600, 19200, 38400, 115200].map(b => (
                    <button
                      key={b}
                      onClick={() => setBaudRate(b)}
                      className={`px-2 py-1 rounded text-xs font-mono transition-colors ${
                        baudRate === b
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-secondary text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      {b}
                    </button>
                  ))}
                </div>
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">Parity</Label>
                <div className="flex gap-1 mt-1">
                  {[['N', 'None'], ['E', 'Even'], ['O', 'Odd']].map(([val, label]) => (
                    <button
                      key={val}
                      onClick={() => setParity(val)}
                      className={`px-2 py-1 rounded text-xs font-mono transition-colors ${
                        parity === val
                          ? 'bg-primary text-primary-foreground'
                          : 'bg-secondary text-muted-foreground hover:text-foreground'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Unit ID */}
        <div className="grid grid-cols-1 gap-3">
          <div>
            <Label className="text-xs text-muted-foreground">Unit ID</Label>
            <Input
              type="number"
              value={unitId}
              onChange={e => setUnitId(Number(e.target.value))}
              min={0}
              max={247}
              className="bg-background font-mono text-sm"
            />
          </div>
        </div>

        <div>
          <Label className="text-xs text-muted-foreground">Register Types</Label>
          <div className="flex gap-2 mt-1">
            {(['holding', 'input', 'coil', 'discrete'] as const).map(type => (
              <button
                key={type}
                onClick={() => toggleType(type)}
                className={`px-3 py-1 rounded text-xs font-mono transition-colors ${
                  scanTypes[type]
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-secondary text-muted-foreground hover:text-foreground'
                }`}
              >
                {type}
              </button>
            ))}
          </div>
        </div>

        <div className="grid grid-cols-3 gap-3">
          <div>
            <Label className="text-xs text-muted-foreground">Start Address</Label>
            <Input
              type="number"
              value={addrStart}
              onChange={e => setAddrStart(Number(e.target.value))}
              min={0}
              className="bg-background font-mono text-sm"
            />
          </div>
          <div>
            <Label className="text-xs text-muted-foreground">End Address</Label>
            <Input
              type="number"
              value={addrEnd}
              onChange={e => setAddrEnd(Number(e.target.value))}
              max={65535}
              className="bg-background font-mono text-sm"
            />
          </div>
          <div>
            <Label className="text-xs text-muted-foreground">Samples</Label>
            <Input
              type="number"
              value={samples}
              onChange={e => setSamples(Number(e.target.value))}
              min={1}
              max={20}
              className="bg-background font-mono text-sm"
            />
          </div>
        </div>

        {error && (
          <div className="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded">
            {error}
          </div>
        )}

        <div className="flex gap-2">
          <Button
            onClick={() => doScan('quick')}
            disabled={!!loading || !canSubmit}
            variant="secondary"
            className="flex-1 font-mono text-xs"
          >
            {loading === 'quick' ? 'Scanning...' : 'Quick Scan'}
          </Button>
          <Button
            onClick={() => doScan('full')}
            disabled={!!loading || !canSubmit || selectedTypes.length === 0}
            className="flex-1 font-mono text-xs"
          >
            {loading === 'full' ? 'Scanning...' : 'Full Scan'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
