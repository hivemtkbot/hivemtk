import fs from 'fs'
import path from 'path'
import { pathToFileURL } from 'url'

const ROOT = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'
const MODULES_DIR = path.join(ROOT, 'router/modules')
const LAYOUT = path.join(ROOT, 'layout/Layout.vue')

const routeSet = new Set()

function combine(parent, child) {
  if (child.startsWith('/')) return child
  if (parent === '/' || parent === '') return '/' + child
  return parent + '/' + child
}

function traverse(route, parentPath) {
  const resolved = combine(parentPath, route.path || '')
  routeSet.add(resolved)
  if (route.children) {
    for (const c of route.children) traverse(c, resolved)
  }
}

const files = fs.readdirSync(MODULES_DIR).filter(f => f.endsWith('.js'))
for (const f of files) {
  const fp = path.join(MODULES_DIR, f)
  try {
    const mod = await import(pathToFileURL(fp).href)
    const arr = mod.default || mod
    if (Array.isArray(arr)) {
      for (const r of arr) traverse(r, '/')
    } else if (arr && typeof arr === 'object') {
      traverse(arr, '/')
    }
  } catch (e) {
    console.error('IMPORT FAIL', f, e.message)
  }
}

const norm = (p) => (p.startsWith('/') ? p : '/' + p)
const rsNorm = new Set(Array.from(routeSet).map(norm))

const layoutSrc = fs.readFileSync(LAYOUT, 'utf8')
const menuPaths = []
const re = /path:\s*['"]([^'"]+)['"]/g
let m
while ((m = re.exec(layoutSrc))) menuPaths.push(m[1])

console.log('route count:', rsNorm.size, 'menu leaf paths:', menuPaths.length)
console.log('=== BROKEN MENU LINKS ===')
let broken = 0
for (const p of Array.from(new Set(menuPaths))) {
  if (!rsNorm.has(norm(p))) {
    broken++
    console.log('  BROKEN:', p)
  }
}
console.log('total broken:', broken)

console.log('=== ALL RESOLVED ROUTES ===')
Array.from(rsNorm).sort().forEach(p => console.log('  ' + p))
