import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import test from 'node:test'

const web = resolve(import.meta.dirname, '../..')
const script = join(web, 'scripts/test-related.mjs')

test('actual frontend entry selects consumers/contracts and rejects failures', () => {
  const fixture = mkdtempSync(join(tmpdir(), 'metapi-related-'))
  try {
    const write = (file, data) => {
      const target = join(fixture, file)
      mkdirSync(resolve(target, '..'), { recursive: true })
      writeFileSync(target, data)
    }
    write(
      'vitest.config.mjs',
      `export default { test: { environment: 'node', include: ['src/**/*.test.js'], maxWorkers: 1 } }`
    )
    write('src/features/a/value.js', 'export const value = 1\n')
    write(
      'src/features/a/local.test.js',
      'import { test, expect } from "vitest"; test("owner", () => expect(1).toBe(1))\n'
    )
    write(
      'src/features/b/consumer.test.js',
      'import { test, expect } from "vitest"; import {value} from "../a/value.js"; test("consumer", () => expect(value).toBe(1))\n'
    )
    write(
      'src/features/c/unrelated.test.js',
      'import { test, expect } from "vitest"; test("other", () => expect(1).toBe(1))\n'
    )
    write(
      'src/contracts.test.js',
      'import { test, expect } from "vitest"; import {readFileSync} from "node:fs"; test("fs", () => expect(typeof readFileSync).toBe("function"))\n'
    )
    // Fixture resolves the repo-pinned packages, not another installed version.
    write('package.json', '{"type":"module"}')
    const run = (files, planOnly = true) =>
      spawnSync(process.execPath, [script], {
        cwd: fixture,
        encoding: 'utf8',
        env: {
          ...process.env,
          SELECTED_WEB_FILES: files,
          METAPI_WEB_PLAN_ONLY: planOnly ? '1' : '0',
          NODE_PATH: join(web, 'node_modules'),
        },
      })
    let out = run('["src/features/a/value.js"]')
    assert.equal(out.status, 0, out.stdout + out.stderr)
    assert.match(out.stdout, /3\/4 test files/)
    assert.match(out.stdout, /consumer\.test\.js/)
    assert.match(out.stdout, /contracts\.test\.js/)
    assert.doesNotMatch(out.stdout, /unrelated\.test\.js/)
    out = run('["src/features/a/value.js"]', false)
    assert.equal(
      out.status,
      0,
      'normal selected behavior must pass: ' + out.stdout + out.stderr
    )
    out = run('[]')
    assert.equal(out.status, 0, out.stdout + out.stderr)
    assert.match(out.stdout, /4\/4 test files/)
    out = run('["src/features/a/missing.js"]')
    assert.equal(out.status, 0, out.stdout + out.stderr)
    assert.match(out.stdout, /full fallback/)
    assert.notEqual(run('{').status, 0)
    assert.notEqual(run('["../outside.ts"]').status, 0)
    write(
      'src/features/a/local.test.js',
      'import { test, expect } from "vitest"; test("bad", () => expect(1).toBe(2))\n'
    )
    out = run('["src/features/a/value.js"]', false)
    assert.notEqual(
      out.status,
      0,
      'failing selected behavior must reject the gate: ' +
        out.stdout +
        out.stderr
    )
  } finally {
    rmSync(fixture, { recursive: true, force: true })
  }
})
