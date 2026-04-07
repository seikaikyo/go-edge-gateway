import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { JobSummary } from '@/lib/api'

interface JobListProps {
  jobs: JobSummary[]
  selectedId: string | null
  onSelect: (id: string) => void
}

export function JobList({ jobs, selectedId, onSelect }: JobListProps) {
  if (jobs.length === 0) {
    return (
      <Card className="bg-card border-border">
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
            Jobs
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">No scan jobs yet</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className="bg-card border-border">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground uppercase tracking-wider">
          Jobs
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        {jobs.map(job => (
          <button
            key={job.job_id}
            onClick={() => onSelect(job.job_id)}
            className={`w-full text-left px-3 py-2 rounded text-xs font-mono flex items-center justify-between transition-colors ${
              selectedId === job.job_id
                ? 'bg-primary/10 border border-primary/30'
                : 'hover:bg-secondary'
            }`}
          >
            <div className="flex items-center gap-3">
              <StatusBadge status={job.status} />
              <span className="text-foreground">{job.device}</span>
            </div>
            <div className="flex items-center gap-3 text-muted-foreground">
              {job.summary && (
                <span>{job.summary.responsive} regs</span>
              )}
              {job.duration_ms && (
                <span>{(job.duration_ms / 1000).toFixed(1)}s</span>
              )}
              {job.error && (
                <span className="text-destructive truncate max-w-[200px]">{job.error}</span>
              )}
            </div>
          </button>
        ))}
      </CardContent>
    </Card>
  )
}

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case 'completed':
      return <Badge className="bg-success/20 text-success border-0 text-[10px]">done</Badge>
    case 'running':
      return <Badge className="bg-primary/20 text-primary border-0 text-[10px] animate-pulse">scan</Badge>
    case 'failed':
      return <Badge className="bg-destructive/20 text-destructive border-0 text-[10px]">fail</Badge>
    default:
      return <Badge variant="secondary" className="text-[10px]">{status}</Badge>
  }
}
