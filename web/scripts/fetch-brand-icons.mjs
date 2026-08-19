#!/usr/bin/env node
// Fetch lobehub brand icons, downscale with sharp, and vendor them into
// src/assets/brand-icons/icons/ so BrandIcon no longer depends on the
// registry.npmmirror.com CDN (which the production CSP img-src blocks).
import { execSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import sharp from 'sharp'

const ROOT = join(dirname(fileURLToPath(import.meta.url)), '..')
const VERSION = '1.83.0'
const CDN = `https://registry.npmmirror.com/@lobehub/icons-static-png/${VERSION}/files`
const OUT = join(ROOT, 'src/assets/brand-icons/icons')
const SIZE = 96

// Icon keys extracted from shared/modelBrand.ts (BRAND_DEFINITIONS icon
// fields) + brandRegistry.ts LEGACY_ICON_ALIASES values. Keep in sync when
// brands are added: re-run this script.
const ICON_KEYS = [
  'openai',
  'claude-color',
  'gemini-color',
  'deepseek-color',
  'qwen-color',
  'zhipu-color',
  'meta-color',
  'mistral-color',
  'moonshot',
  'yi-color',
  'wenxin-color',
  'spark-color',
  'hunyuan-color',
  'doubao-color',
  'minimax-color',
  'cohere-color',
  'microsoft-color',
  'xai',
  'stepfun-color',
  'baichuan-color',
  'ai21-brand-color',
  'ai2-color',
  'nova',
  'stability-color',
  'nvidia-color',
  'ibm',
  'baai',
  'bytedance-color',
  'internlm-color',
  'midjourney',
  'deepl-color',
  'jina',
  'relace',
  'arcee-color',
  'aionlabs-color',
  'deepcogito-color',
  'essentialai-color',
  'inception',
  'inflection',
  'liquid',
  'longcat-color',
  'morph-color',
  'nousresearch',
  'upstage-color',
  'xiaomimimo',
  'zai',
  'sensenova-brand-color',
  'openrouter',
  'groq',
  'deepinfra-color',
  'fireworks-color',
  'together-brand-color',
  'replicate-brand',
  'cerebras-brand-color',
  'bailian-color',
]

const results = { ok: 0, missing: [] }

for (const variant of ['dark', 'light']) {
  mkdirSync(join(OUT, variant), { recursive: true })
  for (const key of ICON_KEYS) {
    const url = `${CDN}/${variant}/${key}.png`
    const dest = join(OUT, variant, `${key}.png`)
    try {
      const body = execSync(`curl -fsS --max-time 30 "${url}"`, {
        maxBuffer: 4 * 1024 * 1024,
      })
      const resized = await sharp(body)
        .resize(SIZE, SIZE)
        .png({ quality: 90 })
        .toBuffer()
      writeFileSync(dest, resized)
      results.ok++
    } catch (error) {
      results.missing.push(`${variant}/${key}`)
      console.error(`MISSING ${url}: ${String(error).split('\n')[0]}`)
    }
  }
}

console.log(`fetched ${results.ok} icons (dark+light)`)
if (results.missing.length) {
  console.error(
    `missing ${results.missing.length}: ${results.missing.join(', ')}`
  )
  process.exit(1)
}
