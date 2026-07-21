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

// Find single-pipe usages inside {{ }} mustaches (Vue2 filter syntax, removed in Vue3)
// Exclude logical || and bitwise | 0 / | '' (those are valid JS)
const found = []
for (const vf of vueFiles) {
  const src = fs.readFileSync(vf, 'utf8')
  const rel = path.relative(ROOT, vf)
  const re = /\{\{([\s\S]*?)\}\}/g
  let m
  while ((m = re.exec(src))) {
    const expr = m[1]
    // find single | not followed by | and not part of ||; crude: split on |
    // ignore if the expression contains '||'
    if (expr.includes('||')) continue
    if (/\|\s*[A-Za-z_$]/.test(expr)) {
      // candidate filter: single pipe followed by identifier
      found.push({ rel, expr: expr.trim() })
    }
  }
}
console.log('=== SINGLE-PIPE (Vue2 filter?) usages in templates ===', found.length)
for (const x of found) console.log('  ' + x.rel + '  ::  ' + x.expr)
