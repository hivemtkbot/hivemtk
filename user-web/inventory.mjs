import fs from 'fs'
import path from 'path'
import url from 'url'

const root = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'
const routerDir = path.join(root, 'router/modules')

// 1. collect route modules
const files = fs.readdirSync(routerDir).filter(f => f.endsWith('.js'))
const routes = []
for (const f of files) {
  const src = fs.readFileSync(path.join(routerDir, f), 'utf8')
  // find default export array
  const m = src.match(/export\s+default\s*(\[[\s\S]*\])\s*$/m)
  if (!m) continue
  let arr
  try { arr = eval('(' + m[1] + ')') } catch (e) { continue }
  function walk(list, prefix) {
    for (const r of list) {
      if (!r || !r.path) continue
      const full = r.path.startsWith('/') ? r.path : (prefix ? prefix + '/' + r.path : r.path)
      routes.push({
        module: f,
        path: full,
        name: r.name,
        component: r.component ? (r.component.name || r.component._c || String(r.component)) : null,
        group: r.meta && r.meta.group,
        title: r.meta && r.meta.title,
        file: r.meta && r.meta.file,
      })
      if (r.children) walk(r.children, full)
    }
  }
  walk(arr, '')
}
// resolve component file
for (const r of routes) {
  let comp = r.component
  if (comp && comp.__file) r.view = comp.__file
  else if (r.file) r.view = r.file
}
// Also capture lazy components referenced as () => import('...')
const lazyRe = /import\(\s*['"]([^'"]+)['"]\s*\)/g
for (const f of files) {
  const src = fs.readFileSync(path.join(routerDir, f), 'utf8')
  let m
  while ((m = lazyRe.exec(src))) {
    const p = m[1]
    // find route whose title precedes; attach later by matching path
  }
}

console.log(JSON.stringify(routes, null, 1))
