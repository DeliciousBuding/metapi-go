class StaticParseError extends Error {}

const MAX_STATIC_NESTING_DEPTH = 32

/**
 * Pure static parser for JS literal values (`{...}`, `[...]`, strings, numbers,
 * booleans, null) without ever invoking eval, Function, or executing user code.
 */
class StaticJsLiteralParser {
  private readonly source: string
  private pos: number
  private depth = 0

  constructor(source: string, startPos: number) {
    this.source = source
    this.pos = startPos
  }

  private skipTrivia(): void {
    while (this.pos < this.source.length) {
      const ch = this.source[this.pos]
      if (ch === ' ' || ch === '\t' || ch === '\n' || ch === '\r') {
        this.pos += 1
        continue
      }
      if (ch === '/' && this.source[this.pos + 1] === '/') {
        this.pos += 2
        while (
          this.pos < this.source.length &&
          this.source[this.pos] !== '\n'
        ) {
          this.pos += 1
        }
        continue
      }
      if (ch === '/' && this.source[this.pos + 1] === '*') {
        const end = this.source.indexOf('*/', this.pos + 2)
        if (end === -1) {
          throw new StaticParseError('Unterminated block comment')
        }
        this.pos = end + 2
        continue
      }
      break
    }
  }

  parseRootObject(): Record<string, unknown> {
    this.skipTrivia()
    if (this.source[this.pos] !== '{') {
      throw new StaticParseError('Expected object literal')
    }
    const obj = this.parseObject()
    return obj
  }

  private parseValue(): unknown {
    this.skipTrivia()
    if (this.pos >= this.source.length) {
      throw new StaticParseError('Unexpected end of input')
    }
    const ch = this.source[this.pos]
    if (ch === '{') return this.parseObject()
    if (ch === '[') return this.parseArray()
    if (ch === '"' || ch === "'" || ch === '`') return this.parseString(ch)
    if (ch === '-' || (ch >= '0' && ch <= '9')) return this.parseNumber()

    const ident = this.readIdentifier()
    if (ident === 'true') return true
    if (ident === 'false') return false
    if (ident === 'null') return null
    throw new StaticParseError(
      ident
        ? `Dynamic identifier "${ident}" is not allowed in static preview`
        : `Unexpected token "${ch}"`
    )
  }

  private parseObject(): Record<string, unknown> {
    if (this.depth >= MAX_STATIC_NESTING_DEPTH) {
      throw new StaticParseError('Nesting depth limit exceeded')
    }
    this.depth += 1
    try {
      this.pos += 1 // '{'
      const result: Record<string, unknown> = {}
      while (true) {
        this.skipTrivia()
        if (this.pos >= this.source.length) {
          throw new StaticParseError('Unterminated object literal')
        }
        if (this.source[this.pos] === '}') {
          this.pos += 1
          return result
        }
        if (
          this.source[this.pos] === '.' &&
          this.source.slice(this.pos, this.pos + 3) === '...'
        ) {
          throw new StaticParseError(
            'Spread properties cannot be statically resolved'
          )
        }
        if (this.source[this.pos] === '[') {
          throw new StaticParseError('Computed property keys are not supported')
        }

        let key: string
        const ch = this.source[this.pos]
        if (ch === '"' || ch === "'" || ch === '`') {
          key = this.parseString(ch)
        } else {
          key = this.readIdentifier()
          if (!key) {
            throw new StaticParseError(`Invalid object key at ${this.pos}`)
          }
        }

        this.skipTrivia()
        if (this.source[this.pos] !== ':') {
          throw new StaticParseError(`Expected ":" after key "${key}"`)
        }
        this.pos += 1

        const value = this.parseValue()
        if (
          key !== '__proto__' &&
          key !== 'constructor' &&
          key !== 'prototype'
        ) {
          result[key] = value
        }

        this.skipTrivia()
        if (this.source[this.pos] === ',') {
          this.pos += 1
          continue
        }
        if (this.source[this.pos] === '}') {
          this.pos += 1
          return result
        }
        throw new StaticParseError(`Expected "," or "}" in object literal`)
      }
    } finally {
      this.depth -= 1
    }
  }

  private parseArray(): unknown[] {
    if (this.depth >= MAX_STATIC_NESTING_DEPTH) {
      throw new StaticParseError('Nesting depth limit exceeded')
    }
    this.depth += 1
    try {
      this.pos += 1 // '['
      const result: unknown[] = []
      while (true) {
        this.skipTrivia()
        if (this.pos >= this.source.length) {
          throw new StaticParseError('Unterminated array literal')
        }
        if (this.source[this.pos] === ']') {
          this.pos += 1
          return result
        }
        if (
          this.source[this.pos] === '.' &&
          this.source.slice(this.pos, this.pos + 3) === '...'
        ) {
          throw new StaticParseError(
            'Spread elements cannot be statically resolved'
          )
        }
        result.push(this.parseValue())
        this.skipTrivia()
        if (this.source[this.pos] === ',') {
          this.pos += 1
          continue
        }
        if (this.source[this.pos] === ']') {
          this.pos += 1
          return result
        }
        throw new StaticParseError('Expected "," or "]" in array literal')
      }
    } finally {
      this.depth -= 1
    }
  }

  private parseString(quote: '"' | "'" | '`'): string {
    this.pos += 1 // opening quote
    let out = ''
    while (this.pos < this.source.length) {
      const ch = this.source[this.pos]
      if (ch === quote) {
        this.pos += 1
        return out
      }
      if (quote === '`' && ch === '$' && this.source[this.pos + 1] === '{') {
        throw new StaticParseError(
          'Template interpolation is not allowed in static preview'
        )
      }
      if (ch === '\\') {
        this.pos += 1
        if (this.pos >= this.source.length) {
          throw new StaticParseError('Unterminated escape sequence')
        }
        const esc = this.source[this.pos]
        this.pos += 1
        switch (esc) {
          case 'n':
            out += '\n'
            break
          case 'r':
            out += '\r'
            break
          case 't':
            out += '\t'
            break
          case '\\':
          case '"':
          case "'":
          case '`':
            out += esc
            break
          case 'u': {
            if (this.source[this.pos] === '{') {
              const close = this.source.indexOf('}', this.pos + 1)
              if (close === -1)
                throw new StaticParseError('Invalid unicode escape')
              const hex = this.source.slice(this.pos + 1, close)
              if (!/^[0-9a-fA-F]{1,6}$/.test(hex)) {
                throw new StaticParseError('Invalid unicode escape')
              }
              const code = Number.parseInt(hex, 16)
              if (!Number.isFinite(code) || code < 0 || code > 0x10ffff) {
                throw new StaticParseError('Invalid unicode escape')
              }
              out += String.fromCodePoint(code)
              this.pos = close + 1
            } else {
              const hex = this.source.slice(this.pos, this.pos + 4)
              if (!/^[0-9a-fA-F]{4}$/.test(hex)) {
                throw new StaticParseError('Invalid unicode escape')
              }
              out += String.fromCharCode(Number.parseInt(hex, 16))
              this.pos += 4
            }
            break
          }
          case 'x': {
            const hex = this.source.slice(this.pos, this.pos + 2)
            if (!/^[0-9a-fA-F]{2}$/.test(hex)) {
              throw new StaticParseError('Invalid hex escape')
            }
            out += String.fromCharCode(Number.parseInt(hex, 16))
            this.pos += 2
            break
          }
          default:
            out += esc
        }
        continue
      }
      if (quote !== '`' && (ch === '\n' || ch === '\r')) {
        throw new StaticParseError('Unterminated string literal')
      }
      out += ch
      this.pos += 1
    }
    throw new StaticParseError('Unterminated string literal')
  }

  private parseNumber(): number {
    const slice = this.source.slice(this.pos)
    const match = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/.exec(slice)
    if (!match) {
      throw new StaticParseError(`Invalid number at ${this.pos}`)
    }
    this.pos += match[0].length
    const num = Number(match[0])
    if (!Number.isFinite(num)) {
      throw new StaticParseError('Non-finite number literal')
    }
    return num
  }

  private readIdentifier(): string {
    const slice = this.source.slice(this.pos)
    const match = /^[A-Za-z_$][A-Za-z0-9_$]*/.exec(slice)
    if (!match) return ''
    this.pos += match[0].length
    return match[0]
  }
}

export function parseStaticObject(source: string, startIndex: number) {
  return new StaticJsLiteralParser(source, startIndex).parseRootObject()
}
