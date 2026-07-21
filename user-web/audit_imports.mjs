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

const exts = ['.js', '.vue', '.ts', '/index.js', '/index.vue']
function resolveImport(fromFile, spec) {
  if (spec.startsWith('@/')) {
    let rel = spec.slice(2)
    let abs = path.join(ROOT, rel)
    for (const e of exts) {
      if (fs.existsSync(abs + e) || fs.existsSync(abs + e.replace('/index', ''))) {
        // crude
      }
    }
    // try direct + extensions
    const candidates = [abs, abs + '.js', abs + '.vue', abs + '.ts', path.join(abs, 'index.js'), path.join(abs, 'index.vue')]
    return candidates.find(c => fs.existsSync(c)) || null
  }
  if (spec.startsWith('.') || spec.startsWith('/')) {
    let abs = path.resolve(path.dirname(fromFile), spec)
    const candidates = [abs, abs + '.js', abs + '.vue', abs + '.ts', path.join(abs, 'index.js'), path.join(abs, 'index.vue')]
    return candidates.find(c => fs.existsSync(c)) || null
  }
  return 'EXTERNAL' // node_module, skip
}

let missing = []
let count = 0
for (const vf of vueFiles) {
  const src = fs.readFileSync(vf, 'utf8')
  // grab <script> blocks (including setup)
  const scriptMatches = src.match(/<script[^>]*>([\s\S]*?)<\/script>/g) || []
  for (const block of scriptMatches) {
    const inner = block.replace(/<\/?script[^>]*>/g, '')
    const re = /import\s+(?:[\s\S]*?\s+from\s+)?['"]([^'"]+)['"]/g
    let m
    while ((m = re.exec(inner))) {
      const spec = m[1]
      if (spec.startsWith('@/') || spec.startsWith('.') || spec.startsWith('/')) {
        count++
        const resolved = resolveImport(vf, spec)
        if (resolved === null) {
          missing.push({ file: vf, spec })
        }
      }
    }
  }
}

console.log('total local imports checked:', count)
console.log('=== MISSING IMPORT TARGETS ===', missing.length)
const seen = new Set()
for (const x of missing) {
  const key = x.file + ' :: ' + x.spec
  if (seen.has(key)) continue
  seen.add(key)
  console.log('  ' + path.relative(ROOT, x.file) + '  ->  ' + x.spec)
}
