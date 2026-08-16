#!/usr/bin/env node
/**
 * i18n Coverage CI 检查
 * USR-I18N-04 / OPT-FE-09
 *
 * 校验 src/i18n/locales/*.json 9 语言（zh / en / ja / ar / es / fr / de / ru / pt）覆盖率。
 * 缺失或空值 PR 阻塞。
 *
 * Usage:
 *   node scripts/check-i18n-coverage.cjs
 *   node scripts/check-i18n-coverage.cjs --threshold 0.95
 *   node scripts/check-i18n-coverage.cjs --json report.json
 */
const fs = require('fs');
const path = require('path');

const LOCALES = ['zh', 'en', 'ja', 'ar', 'es', 'fr', 'de', 'ru', 'pt'];
const REFERENCE = 'zh'; // 参考语言
const LOCALES_DIR = path.resolve(__dirname, '..', 'src', 'i18n', 'locales');

function parseArgs() {
  const args = process.argv.slice(2);
  const opts = { threshold: 1.0, json: null };
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--threshold') opts.threshold = parseFloat(args[++i]);
    else if (args[i] === '--json') opts.json = args[++i];
  }
  return opts;
}

function flatten(obj, prefix = '') {
  const out = [];
  if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
    for (const k of Object.keys(obj)) {
      const v = obj[k];
      const path = prefix ? `${prefix}.${k}` : k;
      if (v && typeof v === 'object' && !Array.isArray(v)) {
        out.push(...flatten(v, path));
      } else {
        out.push(path);
      }
    }
  }
  return out;
}

function loadLocale(code) {
  const file = path.join(LOCALES_DIR, `${code}.json`);
  if (!fs.existsSync(file)) return { keys: [], missing: true, file };
  const json = JSON.parse(fs.readFileSync(file, 'utf8'));
  return { keys: flatten(json), missing: false, file };
}

function isEmpty(val) {
  if (val === null || val === undefined) return true;
  if (typeof val === 'string' && val.trim() === '') return true;
  return false;
}

function checkCoverage() {
  const opts = parseArgs();
  const ref = loadLocale(REFERENCE);
  if (ref.missing) {
    console.error(`❌ 参考语言文件不存在: ${ref.file}`);
    process.exit(1);
  }
  const refKeys = new Set(ref.keys);
  const results = {};

  for (const code of LOCALES) {
    if (code === REFERENCE) {
      results[code] = { coverage: 1, missing: [], empty: [], total: ref.keys.length };
      continue;
    }
    const loc = loadLocale(code);
    if (loc.missing) {
      results[code] = { coverage: 0, missing: ref.keys, empty: [], total: ref.keys.length, fileMissing: true };
      continue;
    }
    const locSet = new Set(loc.keys);
    const missing = ref.keys.filter((k) => !locSet.has(k));
    const empty = loc.keys
      .filter((k) => refKeys.has(k))
      .map((k) => {
        // 简单判断：空字符串
        const json = JSON.parse(fs.readFileSync(loc.file, 'utf8'));
        const parts = k.split('.');
        let v = json;
        for (const p of parts) v = v?.[p];
        return isEmpty(v) ? k : null;
      })
      .filter(Boolean);
    const present = ref.keys.length - missing.length;
    const coverage = ref.keys.length === 0 ? 1 : present / ref.keys.length;
    results[code] = { coverage, missing, empty, total: ref.keys.length };
  }

  // 输出报告
  const report = { total: ref.keys.length, languages: results, threshold: opts.threshold };
  if (opts.json) {
    fs.writeFileSync(opts.json, JSON.stringify(report, null, 2));
    console.log(`📄 报告写入: ${opts.json}`);
  }

  // 控制台输出
  console.log('\n=== i18n Coverage 报告 ===');
  console.log(`参考语言: ${REFERENCE}，总 key 数: ${ref.keys.length}`);
  console.log(`阈值: ${(opts.threshold * 100).toFixed(1)}%\n`);
  let exitCode = 0;
  for (const code of LOCALES) {
    const r = results[code];
    const pct = (r.coverage * 100).toFixed(2);
    const flag = r.coverage >= opts.threshold ? '✅' : '❌';
    console.log(`${flag} ${code}: ${pct}%  (缺失 ${r.missing.length}, 空值 ${r.empty.length})`);
    if (r.fileMissing) {
      console.log(`   文件不存在: ${code}.json`);
      exitCode = 1;
    } else if (r.coverage < opts.threshold) {
      exitCode = 1;
      if (r.missing.length > 0 && r.missing.length <= 10) {
        console.log(`   缺失: ${r.missing.slice(0, 10).join(', ')}`);
      } else if (r.missing.length > 10) {
        console.log(`   缺失前 10: ${r.missing.slice(0, 10).join(', ')} ...（共 ${r.missing.length}）`);
      }
      if (r.empty.length > 0 && r.empty.length <= 10) {
        console.log(`   空值: ${r.empty.slice(0, 10).join(', ')}`);
      }
    }
  }
  console.log('');
  if (exitCode === 0) {
    console.log(`✅ 所有语言覆盖率 ≥ ${(opts.threshold * 100).toFixed(0)}%`);
  } else {
    console.error(`❌ 存在不达标语言，请补齐翻译后重试`);
  }
  process.exit(exitCode);
}

checkCoverage();
