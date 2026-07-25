// 提取 user-web 全部路由，生成 ui-walk/manifest.json（持久化任务清单，避免任务丢失）
import { readFileSync, writeFileSync, readdirSync, existsSync, mkdirSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const SRC = join(__dirname, '..', 'src')
const ROUTER_DIR = join(SRC, 'router')
const MODULES_DIR = join(ROUTER_DIR, 'modules')
const OUT = join(__dirname, 'manifest.json')
const REPORTS = join(__dirname, 'reports')
if (!existsSync(REPORTS)) mkdirSync(REPORTS, { recursive: true })

function parseRoutes(content) {
  const paths = [...content.matchAll(/path:\s*'([^']+)'/g)].map((m) => m[1])
  const names = [...content.matchAll(/name:\s*'([^']+)'/g)].map((m) => m[1])
  const titles = [...content.matchAll(/title:\s*'([^']*)'/g)].map((m) => m[1])
  const entries = []
  for (let i = 0; i < paths.length; i++) {
    const raw = paths[i]
    if (raw.includes(':pathMatch')) continue // catch-all
    if (raw === '/' ) continue
    const full = raw.startsWith('/') ? raw : '/' + raw
    entries.push({
      path: full,
      name: names[i] || '',
      title: titles[i] || '',
      hasParams: raw.includes(':'),
    })
  }
  return entries
}

const all = []
// initRoutes (index.js)
const idxContent = readFileSync(join(ROUTER_DIR, 'index.js'), 'utf8')
all.push(...parseRoutes(idxContent))
// modules
for (const f of readdirSync(MODULES_DIR)) {
  if (!f.endsWith('.js')) continue
  const c = readFileSync(join(MODULES_DIR, f), 'utf8')
  const mod = f.replace(/\.js$/, '')
  for (const e of parseRoutes(c)) {
    e.module = mod
    all.push(e)
  }
}

// 去重
const seen = new Set()
const routes = []
const REDIRECTS = new Set(['/telegram/group', '/oneid', '/chat/embed', '/chat/embed/'])
for (const e of all) {
  if (REDIRECTS.has(e.path)) continue
  if (seen.has(e.path)) continue
  seen.add(e.path)
  const isPublic = e.path === '/login' || e.path === '/setup' || e.path.startsWith('/chat/embed')
  routes.push({
    path: e.path,
    name: e.name,
    title: e.title,
    module: e.module || 'init',
    needsAuth: !isPublic,
    hasParams: e.hasParams,
    status: 'pending', // pending | running | done | issue | fixed | skip
    note: '',
    report: '',
    updatedAt: '',
  })
}

// 保留已有权限状态（若 manifest 已存在）
let existing = {}
if (existsSync(OUT)) {
  try {
    const prev = JSON.parse(readFileSync(OUT, 'utf8'))
    for (const r of prev.routes || []) existing[r.path] = r
  } catch {}
}
for (const r of routes) {
  const ex = existing[r.path]
  if (ex && ex.status && ex.status !== 'pending') {
    r.status = ex.status
    r.note = ex.note || ''
    r.report = ex.report || ''
    r.updatedAt = ex.updatedAt || ''
  }
}

writeFileSync(OUT, JSON.stringify({ generatedAt: new Date().toISOString(), base: 'http://127.0.0.1:8211', routes }, null, 2))
console.log(`提取路由 ${routes.length} 条，写入 ${OUT}`)
const counts = {}
for (const r of routes) counts[r.status] = (counts[r.status] || 0) + 1
console.log('状态分布:', JSON.stringify(counts))
