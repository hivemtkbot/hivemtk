import fs from 'fs'
import path from 'path'

const root = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'
const DRY = process.argv[2] === 'dry'
function walk(dir, out) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (e.name.endsWith('.vue')) out.push(p)
  }
}
const files = []
walk(root, files)

const forRe = /v-for="\s*(?:\(\s*([A-Za-z_$][\w$]*)\s*,\s*([A-Za-z_$][\w$]*)\s*\)|([A-Za-z_$][\w$]*))\s+in\s+([^"]+)"/g
const changes = []
for (const fp of files) {
  const lines = fs.readFileSync(fp, 'utf8').split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (!/\bv-for\b/.test(line)) continue
    if (/\b:key\b/.test(line) || /\bkey\s*=/.test(line)) continue
    let hasKey = false
    for (let j = i; j <= Math.min(i + 8, lines.length - 1); j++) {
      if (/\b:key\b/.test(lines[j]) || /\bkey\s*=/.test(lines[j])) { hasKey = true; break }
      if (/>/.test(lines[j])) break
    }
    if (hasKey) continue
    forRe.lastIndex = 0
    const m = forRe.exec(line)
    if (!m) continue
    const idxVar = m[2]
    const singleVar = m[1] || m[3]
    const iterated = m[4].trim()
    let keyExpr
    if (idxVar) keyExpr = idxVar
    else if (/^\d+$/.test(iterated) || /^(count|total|length|n|num)$/i.test(iterated.replace(/\.\w+$/, ''))) keyExpr = singleVar
    else keyExpr = singleVar + '.id'
    const replaced = line.replace(/(v-for="[^"]*")/, `$1 :key="${keyExpr}"`)
    if (replaced === line) continue
    changes.push({ fp, line: i + 1, k: keyExpr })
    lines[i] = replaced
  }
  if (!DRY) fs.writeFileSync(fp, lines.join('\n'))
}
console.log('DRY=' + DRY + ' TOTAL changes:', changes.length)
for (const c of changes) console.log(`${c.fp}:${c.line}  +:key="${c.k}"`)
