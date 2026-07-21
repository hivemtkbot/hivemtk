import fs from 'fs'
import path from 'path'

const root = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'

// 1. Build api method index: for each api file, top-level keys of `export default {`
function braceMatch(src, openIdx) {
  let depth = 0, i = openIdx
  for (; i < src.length; i++) {
    const c = src[i]
    if (c === '{') depth++
    else if (c === '}') { depth--; if (depth === 0) return i }
  }
  return -1
}
const apiDir = path.join(root, 'api')
const apiMethods = {} // fileBase -> Set(methods)
for (const f of fs.readdirSync(apiDir).filter(f => f.endsWith('.js'))) {
  const src = fs.readFileSync(path.join(apiDir, f), 'utf8')
  const re = /export\s+default\s*\{/g
  let m, keys = new Set()
  while ((m = re.exec(src))) {
    const start = m.index + m[0].length - 1
    const end = braceMatch(src, start)
    if (end < 0) continue
    const body = src.slice(start, end).replace(/\/\/[^\n]*/g, '')
    // top-level keys: method shorthand `name(` OR `name:`
    const kr = /(?:^|[{,]\s*)([A-Za-z_$][\w$]*)\s*[\(:,}\s]/g
    let km
    while ((km = kr.exec(body))) keys.add(km[1])
  }
  apiMethods[f] = keys
}

// 2. Walk all .vue, find default imports from '@/api/X', then method calls
function walk(dir, out) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (e.name.endsWith('.vue')) out.push(p)
  }
}
const vues = []
walk(path.join(root, 'views'), vues)
walk(path.join(root, 'layout'), vues)
walk(path.join(root, 'components'), vues)

let problems = 0
for (const vp of vues) {
  const src = fs.readFileSync(vp, 'utf8')
  // import X from '@/api/Y'
  const ir = /import\s+([A-Za-z_$][\w$]*)\s+from\s+['"]@\/api\/([\w$]+)['"]/g
  let im
  while ((im = ir.exec(src))) {
    const alias = im[1]
    const apiFile = im[2] + '.js'
    const methods = apiMethods[apiFile]
    if (!methods) { console.log(`MISSING API FILE: ${vp} -> @/api/${im[2]}`); problems++; continue }
    // find alias.method( calls
    const cr = new RegExp('\\b' + alias + '\\.([A-Za-z_$][\\w$]*)\\s*\\(', 'g')
    let cm
    while ((cm = cr.exec(src))) {
      const method = cm[1]
      if (!methods.has(method)) {
        console.log(`MISSING METHOD: ${vp}:${alias}.${method}() not in @/api/${im[2]}`)
        problems++
      }
    }
  }
}
console.log('PROBLEMS:', problems)
