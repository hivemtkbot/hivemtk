#!/usr/bin/env node
/**
 * 触达运营 i18n 静态完整性审计
 * - 扫描触达运营相关视图目录中所有 $t('key') / t('key') / i18n.global.t('key') 用法
 * - 比对 src/i18n/locales/{zh,en}.json，报告任一方缺失的 key
 * 仅做报告，不改变测试通过状态。
 */
const fs = require('fs')
const path = require('path')

const ROOT = path.resolve(__dirname, '..')
const LOCALES = path.join(ROOT, 'src', 'i18n', 'locales')
const zh = JSON.parse(fs.readFileSync(path.join(LOCALES, 'zh.json'), 'utf8'))
const en = JSON.parse(fs.readFileSync(path.join(LOCALES, 'en.json'), 'utf8'))

const REACH_DIRS = [
  'email', 'sms',
  'douyinCard', 'kuaishouCard', 'xiaohongshuCard', 'xianyuCard', 'tiktokCard',
  'whatsapp', 'whatsappBot', 'whatsappCloud', 'telegram', 'feishu',
  'community', 'shortLink', 'livecode',
]

const keyRe = /(?:\$t|t|i18n\.global\.t)\(\s*['"]([^'"]+)['"]\s*\)/g

function walk(dir) {
  const out = []
  if (!fs.existsSync(dir)) return out
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name)
    if (ent.isDirectory()) out.push(...walk(p))
    else if (ent.name.endsWith('.vue')) out.push(p)
  }
  return out
}

const files = []
for (const d of REACH_DIRS) files.push(...walk(path.join(ROOT, 'src', 'views', d)))

const missingZh = new Map() // key -> [files]
const missingEn = new Map()
const used = new Set()

for (const f of files) {
  const src = fs.readFileSync(f, 'utf8')
  let m
  while ((m = keyRe.exec(src))) {
    const key = m[1]
    used.add(key)
    const rel = path.relative(ROOT, f)
    if (!(key in zh)) {
      if (!missingZh.has(key)) missingZh.set(key, [])
      missingZh.get(key).push(rel)
    }
    if (!(key in en)) {
      if (!missingEn.has(key)) missingEn.set(key, [])
      missingEn.get(key).push(rel)
    }
  }
}

let problems = 0
console.log(`扫描文件: ${files.length} | 使用 key 数: ${used.size}`)
if (missingZh.size) {
  problems += missingZh.size
  console.log('\n[缺失于 zh.json]')
  for (const [k, fs2] of missingZh) console.log(`  - ${k}  <- ${[...new Set(fs2)].join(', ')}`)
}
if (missingEn.size) {
  problems += missingEn.size
  console.log('\n[缺失于 en.json]')
  for (const [k, fs2] of missingEn) console.log(`  - ${k}  <- ${[...new Set(fs2)].join(', ')}`)
}
if (!problems) {
  console.log('\n✅ i18n 完整：所有触达视图使用的 key 在 zh.json 与 en.json 均存在')
} else {
  console.log(`\n⚠️ 共 ${problems} 个 locale 缺口（需补全）`)
}
process.exit(0)
