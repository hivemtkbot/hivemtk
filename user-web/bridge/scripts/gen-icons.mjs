// 零依赖生成蜂桥图标 PNG（16/32/48/128）。
// 设计：琥珀色圆角渐变底 + 白色圆角聊天气泡 + 三个橙色圆点。
// 用途：manifest 的 icons / action.default_icon 需要真实栅格图标（Chrome 不支持 SVG 作扩展图标）。
// 运行：node scripts/gen-icons.mjs
import { writeFileSync, mkdirSync } from 'fs';
import { resolve, dirname } from 'path';
import { deflateSync } from 'zlib';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const outDir = resolve(__dirname, '..', 'assets', 'icons');
mkdirSync(outDir, { recursive: true });

// ---- PNG 编码（RGBA, 8bit）----
const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();
function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}
function chunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length, 0);
  const typeBuf = Buffer.from(type, 'ascii');
  const body = Buffer.concat([typeBuf, data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body), 0);
  return Buffer.concat([len, body, crc]);
}
function encodePNG(width, height, rgba) {
  const sig = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 6; // color type RGBA
  ihdr[10] = 0;
  ihdr[11] = 0;
  ihdr[12] = 0;
  // 每行前置 filter byte 0
  const stride = width * 4;
  const raw = Buffer.alloc((stride + 1) * height);
  for (let y = 0; y < height; y++) {
    raw[y * (stride + 1)] = 0;
    rgba.copy(raw, y * (stride + 1) + 1, y * stride, y * stride + stride);
  }
  const idat = deflateSync(raw);
  return Buffer.concat([sig, chunk('IHDR', ihdr), chunk('IDAT', idat), chunk('IEND', Buffer.alloc(0))]);
}

// ---- 绘制工具 ----
function makeCanvas(s) {
  return { s, buf: Buffer.alloc(s * s * 4) }; // RGBA, 初始全透明
}
function blend(cv, x, y, r, g, b, a) {
  if (x < 0 || y < 0 || x >= cv.s || y >= cv.s) return;
  const i = (y * cv.s + x) * 4;
  const sa = a / 255;
  const da = cv.buf[i + 3] / 255;
  const oa = sa + da * (1 - sa);
  if (oa <= 0) return;
  cv.buf[i] = Math.round((r * sa + cv.buf[i] * da * (1 - sa)) / oa);
  cv.buf[i + 1] = Math.round((g * sa + cv.buf[i + 1] * da * (1 - sa)) / oa);
  cv.buf[i + 2] = Math.round((b * sa + cv.buf[i + 2] * da * (1 - sa)) / oa);
  cv.buf[i + 3] = Math.round(oa * 255);
}
function fillRoundRect(cv, x0, y0, x1, y1, rad, col) {
  for (let y = Math.floor(y0); y <= Math.ceil(y1); y++) {
    for (let x = Math.floor(x0); x <= Math.ceil(x1); x++) {
      // 圆角裁剪
      let inside = true;
      const corners = [
        [x0 + rad, y0 + rad, x0, y0],
        [x1 - rad, y0 + rad, x1, y0],
        [x0 + rad, y1 - rad, x0, y1],
        [x1 - rad, y1 - rad, x1, y1],
      ];
      for (const [cx, cy, ox, oy] of corners) {
        if (x < cx && y < cy) {
          const dx = x - ox, dy = y - oy;
          if (dx * dx + dy * dy > rad * rad) { inside = false; break; }
        }
      }
      if (inside) blend(cv, x, y, col[0], col[1], col[2], col[3]);
    }
  }
}
function fillCircle(cv, cx, cy, r, col) {
  for (let y = Math.floor(cy - r); y <= Math.ceil(cy + r); y++) {
    for (let x = Math.floor(cx - r); x <= Math.ceil(cx + r); x++) {
      const dx = x - cx, dy = y - cy;
      if (dx * dx + dy * dy <= r * r) blend(cv, x, y, col[0], col[1], col[2], col[3]);
    }
  }
}

function drawIcon(s) {
  const cv = makeCanvas(s);
  const top = [0xff, 0xc0, 0x46, 255];
  const bot = [0xfb, 0x8c, 0x00, 255];
  // 背景圆角渐变（每像素线性插值上下色）
  const m = Math.max(1, Math.round(s * 0.06));
  for (let y = 0; y < s; y++) {
    const t = s <= 1 ? 0 : y / (s - 1);
    const r = Math.round(top[0] + (bot[0] - top[0]) * t);
    const g = Math.round(top[1] + (bot[1] - top[1]) * t);
    const b = Math.round(top[2] + (bot[2] - top[2]) * t);
    for (let x = 0; x < s; x++) blend(cv, x, y, r, g, b, 255);
  }
  fillRoundRect(cv, m, m, s - m, s - m, Math.round(s * 0.20), [top[0], top[1], top[2], 255]);
  // 白色气泡
  const bx0 = s * 0.24, by0 = s * 0.20, bx1 = s * 0.80, by1 = s * 0.70, br = s * 0.16;
  fillRoundRect(cv, bx0, by0, bx1, by1, br, [255, 255, 255, 255]);
  // 三个橙色点
  const dotY = (by0 + by1) / 2;
  const dotR = s * 0.055;
  const offs = [-s * 0.11, 0, s * 0.11];
  for (const o of offs) fillCircle(cv, (bx0 + bx1) / 2 + o, dotY, dotR, [0xfb, 0x8c, 0x00, 255]);
  return cv.buf;
}

for (const s of [16, 32, 48, 128]) {
  const png = encodePNG(s, s, drawIcon(s));
  const f = resolve(outDir, `icon-${s}.png`);
  writeFileSync(f, png);
  console.log('wrote', f, png.length, 'bytes');
}
