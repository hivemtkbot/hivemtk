#!/usr/bin/env node
/**
 * USR-PF-01: 老风格 API → http 风格自动迁移
 * 转换规则：
 *   import request from '@/utils/request'  →  import { http } from '@/utils/request'
 *   request({ url: '...', method: 'get', params })  →  http.get('...', params)
 *   request({ url: '...', method: 'get', data })     →  http.get('...', data) (但语义可能不同)
 *   request({ url: '...', method: 'post', data })    →  http.post('...', data)
 *   request({ url: '...', method: 'put', data })     →  http.put('...', data)
 *   request({ url: '...', method: 'delete', params }) →  http.delete('...', params)
 *   request({ url: '...', method: 'delete', data })   →  http.delete('...', data)
 *   request({ url: '...', method: 'post', params })   →  http.post('...', params)
 *
 * 复杂情况（headers / custom config）会保持为 request() 形式并打印 WARN 提示。
 *
 * 用法:
 *   node scripts/migrate-api-to-http.cjs              # 干跑 + 显示将变更
 *   node scripts/migrate-api-to-http.cjs --apply     # 实际写文件
 *   node scripts/migrate-api-to-http.cjs --apply --only=abExperiment.js,oneid.js
 */
const fs = require('fs');
const path = require('path');

const API_DIR = path.resolve(__dirname, '..', 'src', 'api');
const args = process.argv.slice(2);
const apply = args.includes('--apply');
const onlyIdx = args.indexOf('--only');
const onlyFiles = onlyIdx > -1 ? args[onlyIdx + 1].split(',') : null;

function listApiFiles() {
  return fs.readdirSync(API_DIR).filter((f) => f.endsWith('.js'));
}

function transform(src) {
  let out = src;
  let warnings = [];

  // 1. 替换 import
  out = out.replace(
    /^import\s+request\s+from\s+['"]@\/utils\/request['"];?$/gm,
    "import { http } from '@/utils/request';"
  );

  // 2. 简单 request() 调用转换
  // 模式 1：单行
  //   request({ url: '...', method: 'get', params: x })
  //   request({ url: '...', method: 'get', params: x, _silent: true })
  //   request({ url: '...', method: 'get' })
  // 模式 2：多行
  //   request({
  //     url: '...',
  //     method: 'get',
  //     params: x
  //   })
  // ...
  // 通用转换（用 AST-like 文本替换）

  // 简单情况：单行 + method
  const singleLineRe = /request\(\s*\{\s*url:\s*(\S+?)\s*,\s*method:\s*['"](\w+)['"]\s*\}\s*\)/g;
  out = out.replace(singleLineRe, (m, url, method) => {
    return `http.${method.toLowerCase()}(${url})`;
  });

  const singleLineWithParamsRe =
    /request\(\s*\{\s*url:\s*(\S+?)\s*,\s*method:\s*['"](\w+)['"]\s*,\s*params:\s*([^}]+?)\s*\}\s*\)/g;
  out = out.replace(singleLineWithParamsRe, (m, url, method, params) => {
    return `http.${method.toLowerCase()}(${url}, ${params})`;
  });

  const singleLineWithDataRe =
    /request\(\s*\{\s*url:\s*(\S+?)\s*,\s*method:\s*['"](\w+)['"]\s*,\s*data:\s*([^}]+?)\s*\}\s*\)/g;
  out = out.replace(singleLineWithDataRe, (m, url, method, data) => {
    return `http.${method.toLowerCase()}(${url}, undefined, ${data})`;
  });

  // 3. 含复杂 option（headers / timeout / _silent 等）— 标记
  if (/\brequest\(/.test(out)) {
    const complexRe = /request\(\s*\{[^}]*?(?:headers|timeout|responseType|_silent|onUploadProgress|signal|cancelToken)[^}]*?\}\s*\)/g;
    let m;
    while ((m = complexRe.exec(out))) {
      warnings.push(`复杂配置需要人工审核: ${m[0].substring(0, 100)}...`);
    }
  }

  return { out, warnings };
}

function main() {
  const files = listApiFiles().filter((f) => {
    if (onlyFiles) return onlyFiles.includes(f);
    return true;
  });
  let totalChanged = 0;
  let totalWarnings = 0;

  for (const file of files) {
    const filePath = path.join(API_DIR, file);
    const src = fs.readFileSync(filePath, 'utf8');
    if (!/^import\s+request\s+from\s+['"]@\/utils\/request['"]/m.test(src)) {
      // 已迁移
      continue;
    }
    const { out, warnings } = transform(src);
    if (out !== src) {
      totalChanged++;
      totalWarnings += warnings.length;
      console.log(`\n📝 ${file}`);
      if (warnings.length > 0) {
        warnings.forEach((w) => console.log(`   ⚠ ${w}`));
      }
      if (apply) {
        fs.writeFileSync(filePath, out, 'utf8');
        console.log(`   ✅ 已写入`);
      } else {
        console.log(`   🔍 干跑模式，加 --apply 实际写入`);
      }
    }
  }

  console.log(`\n=== 汇总 ===`);
  console.log(`变更文件数: ${totalChanged}`);
  console.log(`需要人工审核: ${totalWarnings}`);
  if (!apply) {
    console.log(`\n加 --apply 实际写入文件`);
  }
}

main();
