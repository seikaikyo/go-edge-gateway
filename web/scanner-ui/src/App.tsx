import { useState, useEffect, useCallback } from 'react'
import { ScanForm } from '@/components/ScanForm'
import { JobList } from '@/components/JobList'
import { RegisterTable } from '@/components/RegisterTable'
import { ExportButton } from '@/components/ExportButton'
import { api } from '@/lib/api'
import type { JobSummary, Job } from '@/lib/api'

export default function App() {
  const [jobs, setJobs] = useState<JobSummary[]>([])
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null)
  const [selectedJob, setSelectedJob] = useState<Job | null>(null)
  const [connected, setConnected] = useState<boolean | null>(null)

  // Check API connectivity
  useEffect(() => {
    fetch('/api/jobs')
      .then(r => { setConnected(r.ok) })
      .catch(() => setConnected(false))
  }, [])

  // Poll jobs list
  const refreshJobs = useCallback(async () => {
    try {
      const data = await api.listJobs()
      setJobs(data || [])
    } catch {
      // silent
    }
  }, [])

  useEffect(() => {
    refreshJobs()
    const interval = setInterval(refreshJobs, 3000)
    return () => clearInterval(interval)
  }, [refreshJobs])

  // Load selected job detail
  useEffect(() => {
    if (!selectedJobId) {
      setSelectedJob(null)
      return
    }

    let active = true
    const load = async () => {
      try {
        const job = await api.getJob(selectedJobId)
        if (active) setSelectedJob(job)
      } catch {
        // silent
      }
    }
    load()

    // Poll if running
    const interval = setInterval(() => {
      if (selectedJob?.status === 'running') load()
    }, 2000)

    return () => {
      active = false
      clearInterval(interval)
    }
  }, [selectedJobId, selectedJob?.status])

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="border-b border-border px-6 py-3 flex items-center justify-between sticky top-0 bg-background/95 backdrop-blur z-10">
        <div className="flex items-center gap-3">
          <h1 className="text-sm font-semibold tracking-wider uppercase text-foreground">
            Modbus Scanner
          </h1>
          <span className="text-xs text-muted-foreground">v0.1.0</span>
        </div>
        <div className="flex items-center gap-3">
          {selectedJob?.result && <ExportButton result={selectedJob.result} />}
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span
              className={`w-2 h-2 rounded-full ${
                connected === true
                  ? 'bg-success'
                  : connected === false
                  ? 'bg-destructive'
                  : 'bg-warning animate-pulse'
              }`}
            />
            {connected === true ? 'API connected' : connected === false ? 'API offline' : 'Checking...'}
          </div>
        </div>
      </header>

      {/* Main */}
      <main className="max-w-6xl mx-auto p-4 space-y-4">
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          {/* Left: Scan Config */}
          <div className="lg:col-span-1 space-y-4">
            <ScanForm onScanStarted={refreshJobs} />
            <JobList
              jobs={jobs}
              selectedId={selectedJobId}
              onSelect={id => setSelectedJobId(id === selectedJobId ? null : id)}
            />
          </div>

          {/* Right: Results */}
          <div className="lg:col-span-2">
            {selectedJob?.result ? (
              <RegisterTable result={selectedJob.result} />
            ) : selectedJob?.status === 'running' ? (
              <div className="flex items-center justify-center h-64 text-muted-foreground text-sm">
                <div className="text-center space-y-2">
                  <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin mx-auto" />
                  <p>Scanning {selectedJob.request.host}...</p>
                </div>
              </div>
            ) : selectedJob?.status === 'failed' ? (
              <div className="flex items-center justify-center h-64">
                <div className="text-center space-y-2">
                  <p className="text-destructive text-sm">Scan failed</p>
                  <p className="text-xs text-muted-foreground max-w-md">{selectedJob.error}</p>
                </div>
              </div>
            ) : (
              <div className="flex items-center justify-center h-64 text-muted-foreground text-sm">
                <div className="text-center space-y-2">
                  <p>Configure and run a scan to discover register maps</p>
                  <p className="text-xs">Read-only operations (FC01-04), safe for production PLCs</p>
                </div>
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  )
}
