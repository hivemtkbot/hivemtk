// 用 esbuild 把每个入口独立打包为自包含 IIFE（MV3 content/background 多入口场景最稳）。
import { build } from 'esbuild';
import { copyFileSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = resolve(__dirname, '..');
const dist = resolve(root, 'dist');

rmSync(dist, { recursive: true, force: true });
mkdirSync(dist, { recursive: true });

const entries = {
  background: 'src/background/index.js',
  'content-douyin': 'src/content/douyin.js',
  'content-xhs': 'src/content/xhs.js',
  'content-tiktok': 'src/content/tiktok.js',
  popup: 'src/popup/index.js',
};

const watch = process.argv.includes('--watch');

for (const [name, entry] of Object.entries(entries)) {
  const opts = {
    entryPoints: [resolve(root, entry)],
    bundle: true,
    format: 'iife',
    outfile: resolve(dist, `${name}.js`),
    platform: 'browser',
    target: 'es2020',
    minify: !watch,
    sourcemap: watch,
    logLevel: 'info',
  };
  if (watch) {
    const ctx = await build.context(opts);
    await ctx.watch();
  } else {
    await build(opts);
  }
}

// popup.html：复制并把脚本指向打包后的 popup.js（classic script，符合 MV3）
let html = readFileSync(resolve(root, 'src/popup/index.html'), 'utf8');
html = html.replace('./index.js', './popup.js');
writeFileSync(resolve(dist, 'popup.html'), html);
copyFileSync(resolve(root, 'manifest.json'), resolve(dist, 'manifest.json'));

console.log('build done ->', dist);
