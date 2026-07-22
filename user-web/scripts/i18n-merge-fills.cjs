#!/usr/bin/env node
/**
 * i18n 合并脚本（M4）：将 phrases_fill1/2/3.js 合并到 phrases.js 中，
 * 自动去重（zh 段），并保留 phrases.js 已有的 en/ja/ar 翻译不被覆盖。
 *
 * 用法：node scripts/i18n-merge-fills.cjs
 */
const fs = require('fs')
const path = require('path')

const root = path.resolve(__dirname, '..')
const modDir = path.join(root, 'src/i18n/modules')

function loadModule(file) {
  let src = fs.readFileSync(path.join(modDir, file), 'utf8')
  // ESM → CJS 兼容：去掉 export default
  src = src.replace(/export\s+default\s+/g, 'module.exports = ')
  // eslint-disable-next-line no-new-func
  const mod = { exports: {} }
  const fn = new Function('module', 'exports', src)
  fn(mod, mod.exports)
  return mod.exports
}

const fillModules = ['phrases_fill1.js', 'phrases_fill2.js', 'phrases_fill3.js']
  .map(loadModule)
  .filter(Boolean)

const phrases = loadModule('phrases.js')

// 合并 zh：以 phrases.js 为准，fill 中的新增 key 追加进去
const mergedZh = { ...phrases.zh }
for (const m of fillModules) {
  if (!m || !m.zh) continue
  for (const [k, v] of Object.entries(m.zh)) {
    if (!(k in mergedZh)) mergedZh[k] = v
  }
}

// 合并 en/ja/ar：phrases.js 已有则保留，否则从 fill 中取
function mergeLang(phrases, fillModules, lang) {
  const out = { ...phrases[lang] }
  for (const m of fillModules) {
    if (!m || !m[lang]) continue
    for (const [k, v] of Object.entries(m[lang])) {
      if (!(k in out)) out[k] = v
    }
  }
  return out
}

const merged = {
  zh: mergedZh,
  en: mergeLang(phrases, fillModules, 'en'),
  ja: mergeLang(phrases, fillModules, 'ja'),
  ar: mergeLang(phrases, fillModules, 'ar'),
}

// 统计
const fillZhKeys = new Set()
for (const m of fillModules) {
  if (m && m.zh) for (const k of Object.keys(m.zh)) fillZhKeys.add(k)
}
const newKeys = Object.keys(mergedZh).filter((k) => !Object.keys(phrases.zh).includes(k))
const overlap = fillZhKeys.size - newKeys.length

console.log(`[i18n-merge] phrases.js 已有 ${Object.keys(phrases.zh).length} 条`)
console.log(`[i18n-merge] fill 合并去重后净增 ${newKeys.length} 条（重复 ${overlap} 条）`)
console.log(`[i18n-merge] 合并后总条数 ${Object.keys(merged.zh).length}`)

// 写出 phrases.js
const header = `// 业务/视图长尾短语词典：以「中文原文」作为 i18n key。
// 设计：zh 中 key === value（源语言即中文）；en/ja/ar 提供翻译。
// vue-i18n 已配置 fallbackLocale:'zh'，未翻译的短语会自动回退显示中文。
//
// 本文件由 scripts/i18n-merge-fills.cjs 自动合并 phrases.js + phrases_fill*.js 生成。
// phrases_fill*.js 已被合并并删除；新增翻译时直接编辑本文件。
`

function dumpLang(name, obj) {
  const lines = Object.entries(obj).map(([k, v]) => {
    const e = (s) => s.replace(/\\/g, '\\\\').replace(/'/g, "\\'")
    return `  ${JSON.stringify(k)}: '${e(v)}',`
  })
  return `const ${name} = {\n${lines.join('\n')}\n}\n`
}

const out = header + '\n' + dumpLang('zh', merged.zh) + '\n' +
  dumpLang('en', merged.en) + '\n' +
  dumpLang('ja', merged.ja) + '\n' +
  dumpLang('ar', merged.ar) + '\n' +
  'export default { zh, en, ja, ar }\n'

fs.writeFileSync(path.join(modDir, 'phrases.js'), out, 'utf8')
console.log(`[i18n-merge] 已写入 phrases.js`)

// 删除 fill 文件
for (const f of ['phrases_fill1.js', 'phrases_fill2.js', 'phrases_fill3.js']) {
  const p = path.join(modDir, f)
  if (fs.existsSync(p)) {
    fs.unlinkSync(p)
    console.log(`[i18n-merge] 已删除 ${f}`)
  }
}
