const BASE = '/api'

export interface ScanRequest {
  mode: 'tcp' | 'rtu'
  // TCP
  host: string
  port: number
  // RTU
  serial_port: string
  baud_rate: number
  data_bits: number
  stop_bits: number
  parity: string
  // Common
  unit_id: number
  scan_types: string[]
  address_start: number
  address_end: number
  batch_size: number
  samples: number
  sample_interval_ms: number
  timeout_ms: number
}

export interface RegisterGuess {
  category: string
  reason: string
}

export interface AnalyzedRegister {
  address: number
  type: string
  raw_values: number[]
  inferred_type: string
  is_dynamic: boolean
  value_range: { min: number; max: number }
  float32_value?: number
  paired_address?: number
  guess?: RegisterGuess
}

export interface ScanSummary {
  total_scanned: number
  responsive: number
  dynamic: number
  static: number
}

export interface ScanResult {
  device: string
  unit_id: number
  scan_duration_ms: number
  summary: ScanSummary
  registers: AnalyzedRegister[]
}

export interface Job {
  job_id: string
  status: 'running' | 'completed' | 'failed'
  request: ScanRequest
  result?: ScanResult
  error?: string
  created_at: string
}

export interface JobSummary {
  job_id: string
  status: string
  device: string
  created_at: string
  summary?: ScanSummary
  duration_ms?: number
  error?: string
}

interface ApiResponse<T> {
  success: boolean
  data: T
  error?: string
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const json: ApiResponse<T> = await res.json()
  if (!json.success) throw new Error(json.error || 'Request failed')
  return json.data
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`)
  const json: ApiResponse<T> = await res.json()
  if (!json.success) throw new Error(json.error || 'Request failed')
  return json.data
}

export const api = {
  scan: (req: Partial<ScanRequest>) => post<{ job_id: string; status: string }>('/scan', req),
  quickScan: (req: Partial<ScanRequest>) => post<{ job_id: string; status: string }>('/scan/quick', req),
  read: (req: Record<string, unknown>) =>
    post<{ device: string; values: number[] }>('/read', req),
  listJobs: () => get<JobSummary[]>('/jobs'),
  getJob: (id: string) => get<Job>(`/jobs/${id}`),
  listSerialPorts: () => get<{ ports: string[] }>('/serial/ports'),
}
