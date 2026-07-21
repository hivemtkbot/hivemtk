import fs from 'fs'
import path from 'path'

const root = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src'
function walk(dir, out) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name)
    if (e.isDirectory()) walk(p, out)
    else if (e.name.endsWith('.vue')) out.push(p)
  }
}
const files = []
walk(root, files)

let hits = 0
for (const fp of files) {
  const lines = fs.readFileSync(fp, 'utf8').split('\n')
  lines.forEach((line, i) => {
    const hasFor = /\bv-for\b/.test(line)
    const hasIf = /\bv-if\b/.test(line)
    // v-if + v-for same element (Vue3: v-if lower priority -> bug)
    if (hasFor && hasIf) {
      console.log(`v-if+v-for same el => ${fp}:${i + 1}: ${line.trim().slice(0, 90)}`)
      hits++
    }
    // v-for without :key (loose: same line must contain key= or :key)
    if (hasFor && !/\b:key\b/.test(line) && !/\bkey\s*=/.test(line)) {
      // might be multi-line tag; check next 2 lines
      const window = lines.slice(i, i + 3).join('\n')
      if (!/\b:key\b/.test(window)) {
        console.log(`v-for missing :key => ${fp}:${i + 1}: ${line.trim().slice(0, 90)}`)
        hits++
      }
    }
  })
}
console.log('TOTAL template-issue hits:', hits)
