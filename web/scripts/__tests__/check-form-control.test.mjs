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
  new URL('../check-form-control.mjs', import.meta.url),
  'utf8'
)

function runGate(files) {
  const root = mkdtempSync(join(tmpdir(), 'metapi-form-control-'))
  const allFiles = {
    'scripts/check-form-control.mjs': gateSource,
    ...files,
  }
  try {
    mkdirSync(join(root, 'src'), { recursive: true })
    for (const [path, contents] of Object.entries(allFiles)) {
      const target = join(root, path)
      mkdirSync(dirname(target), { recursive: true })
      writeFileSync(target, contents)
    }
    const result = spawnSync(
      process.execPath,
      ['scripts/check-form-control.mjs'],
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

test('原生元素、ui 原语和透传 rest props 的复合组件通过', () => {
  const result = runGate({
    'src/components/ui/input.tsx':
      'export function Input(props: Record<string, unknown>) { return <input {...props} /> }\n',
    'src/features/demo/forwarding.tsx':
      'export function ForwardingInput({ label, ...props }: { label: string }) { return <input aria-label={label} {...props} /> }\n',
    'src/features/demo/form.tsx': [
      "import { Input } from '@/components/ui/input'",
      "import { ForwardingInput } from './forwarding'",
      'export function DemoForm() {',
      '  return (',
      '    <>',
      '      <FormControl><input /></FormControl>',
      '      <FormControl><Input /></FormControl>',
      "      <FormControl><ForwardingInput label='x' /></FormControl>",
      '    </>',
      '  )',
      '}',
    ].join('\n'),
  })
  assert.equal(result.status, 0, result.output)
  assert.match(result.output, /FormControl forwarding clean: 3 site\(s\)/)
})

test('未透传 rest props 的复合组件包裹裸 <input> 失败', () => {
  const result = runGate({
    'src/features/demo/leaky.tsx':
      'export function LeakyInput({ value }: { value?: string }) { return <input value={value} /> }\n',
    'src/features/demo/form.tsx': [
      "import { LeakyInput } from './leaky'",
      'export function DemoForm() {',
      '  return <FormControl><LeakyInput /></FormControl>',
      '}',
    ].join('\n'),
  })
  assert.equal(result.status, 1, result.output)
  assert.match(result.output, /FormControl child loses its accessible name/)
  assert.match(result.output, /<LeakyInput>/)
})

test('空目录 0-site 扫描不得通过', () => {
  const result = runGate({})
  assert.equal(result.status, 1, result.output)
  assert.match(result.output, /would pass vacuously: 0 <FormControl> sites/)
})
