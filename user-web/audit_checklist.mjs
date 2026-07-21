import fs from 'fs'
import path from 'path'
import { pathToFileURL } from 'url'

const ROOT = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'
const MODULES_DIR = path.join(ROOT, 'router/modules')

function resolveView(raw) {
  if (!raw || !raw.includes('@/views/')) return raw || '(none)'
  let rel = raw.replace('@/views/', '').replace(/^\//, '')
  let abs = path.join(ROOT, 'views', rel)
  if (fs.existsSync(abs)) return path.relative(ROOT, abs)
  const cands = [abs + '.vue', path.join(abs, 'index.vue')]
  const f = cands.find(c => fs.existsSync(c))
  return f ? path.relative(ROOT, f) : 'MISSING:' + raw
}

function combine(parent, child) {
  if (child.startsWith('/')) return child
  if (parent === '/' || parent === '') return '/' + child
  return parent + '/' + child
}

const results = []
function traverse(route, parentPath) {
  const resolved = combine(parentPath, route.path || '')
  if (route.component) {
    // lazy component () => import(...)
    const m = String(route.component).match(/import\(\s*['"]([^'"]+)['"]\s*\)/)
    results.push({ path: resolved, view: m ? resolveView(m[1]) : '(inline)', title: route.meta?.title, group: route.meta?.group })
  } else if (route.children) {
    for (const c of route.children) traverse(c, resolved)
  } else if (route.redirect) {
    results.push({ path: resolved, view: '(redirect -> ' + route.redirect + ')', title: route.meta?.title, group: route.meta?.group })
  }
}

const files = fs.readdirSync(MODULES_DIR).filter(f => f.endsWith('.js'))
for (const f of files) {
  try {
    const mod = await import(pathToFileURL(path.join(MODULES_DIR, f)).href)
    const arr = mod.default || mod
    const list = Array.isArray(arr) ? arr : [arr]
    for (const r of list) traverse(r, '/')
  } catch (e) { /* ignore */ }
}

// Group by meta.group
const groups = {}
for (const r of results) {
  const g = r.group || 'ungrouped'
  if (!groups[g]) groups[g] = []
  groups[g].push(r)
}
console.log('TOTAL PAGES:', results.length)
for (const g of Object.keys(groups).sort()) {
  console.log('\n### GROUP: ' + g + ' (' + groups[g].length + ')')
  for (const r of groups[g].sort((a,b)=>a.path.localeCompare(b.path))) {
    console.log('  ' + r.path.padEnd(42) + '  ' + r.view)
  }
}
