import type { ImportCandidate, ImportSiteItem } from '../types'

export type CandidateStepProps = {
  candidates: ImportCandidate[]
  updateCandidate: (id: string, patch: Partial<ImportCandidate>) => void
}

export function makeCandidate(url: string, index: number): ImportCandidate {
  return {
    id: `candidate-${index}-${url.slice(0, 32)}`,
    url,
    name: defaultSiteName(url),
    platform: '',
    confidence: null,
    detected: false,
    detecting: false,
    duplicateStrategy: 'skip',
    includeAccount: false,
    username: '',
    accessToken: '',
    apiToken: '',
    weight: 1,
  }
}

function defaultSiteName(url: string): string {
  try {
    const withScheme = url.includes('://') ? url : `https://${url}`
    const parsed = new URL(withScheme)
    return parsed.hostname
  } catch {
    return url
  }
}

export function toImportSiteItem(candidate: ImportCandidate): ImportSiteItem {
  const accessToken = candidate.accessToken.trim()
  const apiToken = candidate.apiToken.trim()
  const hasAccount =
    candidate.includeAccount && (accessToken !== '' || apiToken !== '')
  return {
    name: candidate.name.trim() || candidate.url,
    url: candidate.url,
    platform: candidate.platform || undefined,
    globalWeight: candidate.weight,
    duplicateStrategy: candidate.duplicateStrategy,
    accounts: hasAccount
      ? [
          {
            username: candidate.username.trim() || undefined,
            accessToken,
            apiToken,
          },
        ]
      : [],
  }
}
