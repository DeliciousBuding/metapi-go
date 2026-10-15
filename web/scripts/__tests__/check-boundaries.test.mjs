import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'

const gateSource = readFileSync(
  new URL('../check-boundaries.mjs', import.meta.url),
  'utf8'
)

// 每次运行真实 CLI，只改变输入目录；不在测试中复刻检测逻辑。
function runGate(changes = {}, source = gateSource) {
  const root = mkdtempSync(join(tmpdir(), 'metapi-boundaries-'))
  const files = {
    'scripts/check-boundaries.mjs': source,
    'src/features/alpha/index.ts':
      "// import { Alpha } from '@/features/alpha'\nexport const Alpha = 1\n",
    'src/features/alpha/consumer.ts':
      "import { PublicValue } from '@/features/beta'\nimport { Table } from '@/components/data-table'\n",
    'src/features/beta/index.ts':
      'export { Internal as PublicValue } from "./value"\n',
    'src/features/beta/value.ts': 'export const Internal = 1\n',
    'src/components/data-table/index.ts': 'export const Table = 1\n',
    ...changes,
  }
  try {
    for (const [path, contents] of Object.entries(files)) {
      if (contents === null) continue
      const target = join(root, path)
      mkdirSync(dirname(target), { recursive: true })
      writeFileSync(target, contents)
    }
    const result = spawnSync(
      process.execPath,
      ['scripts/check-boundaries.mjs'],
      {
        cwd: root,
        encoding: 'utf8',
        timeout: 10_000,
      }
    )
    assert.ifError(result.error)
    return { status: result.status, output: result.stdout + result.stderr }
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
}

function failsWith(changes, message, source) {
  const result = runGate(changes, source)
  assert.equal(result.status, 1, result.output)
  assert.match(result.output, message)
}

test('公共入口、导出别名和 feature 内部相对导入通过', () => {
  const result = runGate({
    'src/features/beta/consumer.ts': "import { Internal } from './value'\n",
    'src/routes/beta.ts': "import { Internal } from '@/features/beta/value'\n",
    'src/components/data-table/consumer.ts':
      "import { Cell } from './core/cell'\n",
    'src/components/data-table/core/cell.ts': 'export const Cell = 1\n',
  })
  assert.equal(result.status, 0, result.output)
  assert.match(result.output, /package boundaries clean/)
})

test('注释导入别名按原始导出名校验，支持多行与 type', () => {
  const result = runGate({
    'src/features/alpha/index.ts': [
      '// import type {',
      '//   PublicValue as LocalValue,',
      "// } from '@/features/beta'",
      'export const Alpha = 1',
    ].join('\n'),
  })
  assert.equal(result.status, 0, result.output)
})

test('本地别名即使存在，也不能掩盖不存在的原始导出', () => {
  failsWith(
    {
      'src/features/alpha/index.ts':
        "// import { Missing as PublicValue } from '@/features/beta'\nexport const Alpha = 1\n",
    },
    /documents 'Missing'.*does not export/
  )
})

for (const [name, statement] of [
  ['静态导入', "import { Internal } from '@/features/beta/value'"],
  ['动态导入', "const loaded = import('@/features/beta/value')"],
  ['副作用导入', "import '@/features/beta/value'"],
  ['相对路径导入', "import { Internal } from '../beta/value'"],
]) {
  test(`跨 feature 深路径${name}失败`, () => {
    failsWith(
      { 'src/features/alpha/deep.ts': statement },
      /feature barrel violation: src\/features\/alpha\/deep.ts:1/
    )
  })
}

for (const [path, target] of [
  ['lib/invalid.ts', '@/features/beta'],
  ['components/invalid.tsx', '@/routes/beta'],
]) {
  test(`${path} 的禁止层间导入失败`, () => {
    failsWith(
      { [`src/${path}`]: `import '${target}'\n` },
      /boundary violation:/
    )
  })
}

test('包外深入 data-table 失败', () => {
  failsWith(
    {
      'src/features/alpha/deep.ts':
        "import { Cell } from '@/components/data-table/core/cell'\n",
    },
    /barrel violation: src\/features\/alpha\/deep.ts:1/
  )
})

test('没有匹配的注释示例时失败', () => {
  failsWith(
    { 'src/features/alpha/index.ts': 'export const Alpha = 1\n' },
    /barrel doc-example check would pass vacuously/
  )
})

test('没有跨 feature 公共入口消费者时失败', () => {
  failsWith(
    {
      'src/features/alpha/consumer.ts':
        "import { Table } from '@/components/data-table'\n",
    },
    /feature barrel gate would pass vacuously/
  )
})

test('feature 空扫保护不依赖子系统登记项', () => {
  // 仅移除 fixture 脚本的登记项，证明独立门禁不会随子系统循环消失。
  const source = gateSource.replace(
    /const BARRELS = \[[\s\S]*?\n\]/,
    'const BARRELS = []'
  )
  assert.notEqual(source, gateSource)
  failsWith(
    { 'src/features/alpha/consumer.ts': 'export const unrelated = 1\n' },
    /feature barrel gate would pass vacuously/,
    source
  )
})

test('子系统公共入口不存在时失败', () => {
  failsWith(
    { 'src/components/data-table/index.ts': null },
    /barrel gate would pass vacuously:[\s\S]*missing/
  )
})

test('子系统没有包外消费者时失败', () => {
  failsWith(
    {
      'src/features/alpha/consumer.ts':
        "import { PublicValue } from '@/features/beta'\n",
    },
    /barrel gate would pass vacuously: no file outside/
  )
})
