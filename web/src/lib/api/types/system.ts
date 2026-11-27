// C1: unified recurring-scheduler run history.
export type SchedulerRunStatus = {
  job: string
  enabled: boolean
  lastRunAt?: string
  lastStatus?: string
  runs24h: number
  success24h: number
  note?: string
  /** Populated only for model-probe (in-memory ring buffer of recent passes). */
  recentRuns?: SchedulerProbeRunSummary[]
}

/** One completed model-probe scheduler pass (in-memory ring buffer entry). */
export type SchedulerProbeRunSummary = {
  startedAt?: string
  completedAt?: string
  accountsConsidered: number
  accountsProbed: number
  targetsScanned: number
  success: number
  failed: number
  inconclusive: number
  skipped: number
}

// H1: product risk banners.
export type Announcement = {
  id: number
  title: string
  message: string
  severity: 'info' | 'warning' | 'critical'
  link?: string | null
  enabled: boolean
  dismissed?: boolean
  dismissedAt?: string | null
  createdAt: string
  updatedAt: string
}

export type AnnouncementsResponse = { items: Announcement[] }

/**
 * GET /api/about — build provenance of the running Go binary.
 *
 * `commit` and `buildTime` are only populated when the binary was linked with
 * ldflags injection (release/Docker builds); a plain local `go build` reports
 * them as empty strings, which the About page renders as an em-dash. Every
 * field is optional here so a trimmed or older backend cannot crash the page.
 */
export type AboutBuildInfoResponse = {
  version?: string
  commit?: string
  buildTime?: string
  goVersion?: string
}
