import i18n from '@/i18n/config'

export async function extractResponseErrorMessage(
  res: Response
): Promise<string> {
  let message = `HTTP ${res.status}`
  try {
    const text = await res.text()
    if (text) {
      try {
        const json = JSON.parse(text)
        if (json?.message && typeof json.message === 'string') {
          message = json.message
        } else if (json?.error && typeof json.error === 'string') {
          message = json.error
        } else if (
          json?.error?.message &&
          typeof json.error.message === 'string'
        ) {
          message = json.error.message
        } else {
          message = `${message}: ${text.slice(0, 120)}`
        }
      } catch {
        message = `${message}: ${text.slice(0, 120)}`
      }
    }
  } catch {
    // ignore body read errors
  }
  return message
}

type Classified403 =
  | { kind: 'invalid-token' }
  | { kind: 'reauth-required'; message: string }
  | { kind: 'other'; message: string }

export async function classify403(res: Response): Promise<Classified403> {
  try {
    // Clone so classification never consumes the body the caller may need.
    const text = await res.clone().text()
    const lower = text.toLowerCase()
    if (lower.includes('"reauthrequired":true')) {
      let message = i18n.t('common.requestFailed')
      try {
        const json = JSON.parse(text)
        if (typeof json?.error === 'string' && json.error) message = json.error
      } catch {
        // fall back to the generic message
      }
      return { kind: 'reauth-required', message }
    }
    if (lower.includes('invalid token')) {
      return { kind: 'invalid-token' }
    }
    return { kind: 'other', message: await extractResponseErrorMessage(res) }
  } catch {
    return { kind: 'other', message: `HTTP ${res.status}` }
  }
}
