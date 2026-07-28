// 本机构建发布脚本（无 CI 依赖，macOS / Linux 直接 `npm run release`）
// 流程：1) 生产构建(esbuild) 2) 打包 dist -> release/hivebridge-<version>.zip 3) 生成发布说明
import { execSync } from 'node:child_process';
import { readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pkg = JSON.parse(readFileSync(resolve(root, 'package.json'), 'utf8'));
const version = pkg.version;

console.log(`\n🐝 HiveBridge 蜂桥 发布构建 v${version}\n`);

// 1) 生产构建
console.log('==> [1/3] 生产构建 (esbuild)');
execSync('npm run build', { cwd: root, stdio: 'inherit' });

const dist = resolve(root, 'dist');
const releaseDir = resolve(root, 'release');
rmSync(releaseDir, { recursive: true, force: true });
mkdirSync(releaseDir, { recursive: true });

// 2) 打包（zip 根目录即 manifest.json，满足 Chrome 加载要求）
const zipName = `hivebridge-${version}.zip`;
const zipPath = resolve(releaseDir, zipName);
console.log(`==> [2/3] 打包 ${zipName}`);
execSync(`cd "${dist}" && zip -r -X "${zipPath}" . >/dev/null`, { stdio: 'inherit' });

// 3) 发布说明
const notes = `# HiveBridge 蜂桥 v${version} 发布包

- 产物：\`release/${zipName}\`（解压后含 manifest.json，可直接「加载已解压的扩展程序」）
- 构建时间：${new Date().toISOString()}

## 包含能力
- 三平台私信桥接：抖音 / 小红书 / TikTok（网页版）
- 实时上行：客户私信 -> user-server（触发 AI）
- 历史回填：页面加载/会话切换时存量消息仅落库，不触发 AI（防回环）
- 下行回写：user-server AI 回复 -> 回写网页私信输入框并发送
- 拟人限速风控：最小间隔 + 令牌桶 + 会话冷却 + 相同文案去重
- 多用户/多会话：按 (channel, account, conversation) 隔离路由与历史

## 安装（本机 Chrome）
1. 打开 chrome://extensions
2. 开启「开发者模式」
3. 「加载已解压的扩展程序」-> 选择解压后的 dist 目录
4. 点击工具栏图标 -> 填写 user-server 地址（如 http://localhost:8080）-> 保存
5. 打开任一平台私信页 -> 点「自检当前私信页」按 bridge.md §17.2 校准选择器

## 验证清单
- [ ] 客户发消息后 user-server 收到 inbound_message（AI 触发）
- [ ] AI 回复回写到网页并出现在对话框（outbound_reply）
- [ ] 刷新/切换会话后历史进入 user-server（history 帧，direction=inbound/outbound）
- [ ] 连续高频消息被限速拦截（popup / 后台日志可见 reason）
`;
writeFileSync(resolve(releaseDir, 'RELEASE_NOTES.md'), notes);
console.log(`==> [3/3] 发布说明 -> release/RELEASE_NOTES.md`);
console.log(`\n✅ 完成：release/${zipName}\n`);
