// Introspect schema for B-domain seeding.
import { Database } from 'bun:sqlite'

const dbPath = process.argv[2] ?? '.tmp-ab/data/hub.db'
const db = new Database(dbPath, { readonly: true })
const tables = db
  .query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
  .all()
for (const { name } of tables) {
  const cols = db.query(`PRAGMA table_info(${name})`).all()
  console.log(
    `== ${name}: ${cols.map((c) => `${c.name}:${c.type}`).join(', ')}`
  )
}
db.close()
