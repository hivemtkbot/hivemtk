import fs from 'fs'
import path from 'path'

const ROOT = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'
const VIEWS = path.join(ROOT, 'views')

let vueFiles = []
function walk(dir) {
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f)
    const st = fs.statSync(p)
    if (st.isDirectory()) walk(p)
    else if (f.endsWith('.vue')) vueFiles.push(p)
  }
}
walk(VIEWS)

// Scan 1: default import from @/api used as object with method call
const defaultApiUsed = []
// Scan 2: `this.` inside <script setup>
const thisInSetup = []
// Scan 3: TODO/FIXME/XXX markers
const todos = []
// Scan 4: hardcoded localhost/127.0.0.1 in api/views (excluding request base)
const hardcoded = []

for (const vf of vueFiles) {
  const src = fs.readFileSync(vf, 'utf8')
  const rel = path.relative(ROOT, vf)
  const scripts = src.match(/<script[^>]*>([\s\S]*?)<\/script>/g) || []
  const isSetup = scripts.some(s => /<script[^>]*setup/.test(s))
  const scriptText = scripts.map(s => s.replace(/<\/?script[^>]*>/g, '')).join('\n')

  // default import from @/api
  const defApi = scriptText.match(/import\s+(\w+)\s+from\s+['"]@\/api\/[\w-]+['"]/g)
  if (defApi) {
    const names = defApi.map(d => d.match(/import\s+(\w+)/)[1])
    for (const n of names) {
      const usage = new RegExp('\\b' + n + '\\.\\w+\\s*\\(').test(scriptText)
      if (usage) defaultApiUsed.push({ rel, name: n })
    }
  }
  // this. in setup
  if (isSetup && /\bthis\./.test(scriptText)) {
    thisInSetup.push({ rel })
  }
  // TODO/FIXME/XXX
  const tm = scriptText.match(/\b(TODO|FIXME|XXX)\b/g)
  if (tm) todos.push({ rel, count: tm.length })
  // hardcoded localhost/127.0.0.1
  if (/(localhost|127\.0\.0\.1)/.test(src)) hardcoded.push({ rel })
}

console.log('=== DEFAULT-IMPORTED API USED AS OBJECT (may be undefined) ===', defaultApiUsed.length)
for (const x of defaultApiUsed) console.log('  ' + x.rel + '  -> ' + x.name)
console.log('=== `this.` INSIDE <script setup> (breaks in Composition API) ===', thisInSetup.length)
for (const x of thisInSetup) console.log('  ' + x.rel)
console.log('=== TODO/FIXME/XXX MARKERS ===', todos.length)
for (const x of todos) console.log('  ' + x.rel + '  (' + x.count + ')')
console.log('=== HARDCODED localhost/127.0.0.1 ===', hardcoded.length)
for (const x of hardcoded) console.log('  ' + x.rel)
