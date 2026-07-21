const fs = require('fs');
const path = require('path');

const ROOT = '/Users/xiaofang/Documents/www/go/marketing-tools-kit/user/user-web/src';
const MODULES_DIR = path.join(ROOT, 'router/modules');
const LAYOUT = path.join(ROOT, 'layout/Layout.vue');

// 1) Collect all route paths
const routePaths = new Set();
function walk(dir) {
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f);
    const st = fs.statSync(p);
    if (st.isDirectory()) walk(p);
    else if (f.endsWith('.js')) {
      fs.readFileSync(p, 'utf8').split('\n').forEach(line => {
        const m = line.match(/path:\s*['"]([^'"]+)['"]/);
        if (m) routePaths.add(m[1]);
      });
    }
  }
}
walk(MODULES_DIR);
// also init routes from router/index.js (profile, notifications, etc.)
fs.readFileSync(path.join(ROOT, 'router/index.js'), 'utf8').split('\n').forEach(line => {
  const m = line.match(/path:\s*['"]([^'"]+)['"]/);
  if (m) routePaths.add(m[1]);
});
// normalize: many module paths are relative (no leading /); menu paths are relative too.
// Build a set of both normalized forms (with and without leading /)
const norm = (p) => (p.startsWith('/') ? p : '/' + p);
const routeSetNorm = new Set([...routePaths].map(norm));

// 2) Extract menu leaf paths from Layout.vue
const layoutSrc = fs.readFileSync(LAYOUT, 'utf8');
const menuPaths = [];
// match :path="'xxx'" and path="xxx" within menu items; also capture sibling title for context
const re = /:path="'([^']+)'"|path="([^"]+)"/g;
let m;
while ((m = re.exec(layoutSrc))) {
  const p = m[1] || m[2];
  if (p) menuPaths.push(p);
}

console.log('=== ALL MENU LEAF PATHS (from Layout.vue) ===');
console.log(menuPaths.length, 'paths');
console.log('=== BROKEN MENU LINKS (no matching route) ===');
let broken = 0;
for (const p of menuPaths) {
  if (!routeSetNorm.has(norm(p))) {
    broken++;
    console.log('  BROKEN:', JSON.stringify(p));
  }
}
console.log('=== TOTAL BROKEN ===', broken);

// also list all route paths for reference
console.log('=== ROUTE PATHS (normalized) ===');
[...routeSetNorm].sort().forEach(p => console.log('  ' + p));
