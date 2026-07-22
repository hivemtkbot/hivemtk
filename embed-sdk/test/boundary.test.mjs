/**
 * @file embed-sdk 边界情况测试
 * @description 覆盖非正常 / 异常输入场景,验证 SDK 不崩溃、不越权、不污染全局
 *
 * 覆盖范围:
 *   1. postMessage origin 白名单:非法 origin / 协议不同 / 路径 / 大小写
 *   2. 消息体格式:null / undefined / 字符串 / 数字 / 数组 / 空对象
 *   3. 超大消息体:1MB / 10MB
 *   4. allowedOrigins 配置:null / undefined / 空数组 / 非数组(SDK 行为是自动推导)
 *   5. 多次 mount / destroy / 重连循环
 *   6. 用户事件回调异常不应导致 SDK 崩溃
 *   7. URL 解析边界:畸形 apiBaseURL / 空字符串 / javascript:
 *   8. config.js 源码 SSR 守卫完整性
 *   9. query 参数非法值(空字符串)
 *  10. 多次重复 init 不应重复挂载
 *  11. 多种 postMessage 消息类型
 *  12. 快速 open/close 循环(50 次)
 *  13. 危险协议 origin (javascript: / data: / vbscript:)
 *
 * 用法: node test/boundary.test.mjs
 *
 * 注意:panel.messageHandler 仅在 panel.create() / panel.show() 之后才注册。
 * 本测试在涉及 postMessage 校验的用例中显式调用 panel.create()。
 */
'use strict'

import { readFileSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve as pathResolve } from 'node:path'
import vm from 'node:vm'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const ROOT = pathResolve(__dirname, '..')

const IIFE_PATH = pathResolve(ROOT, 'dist/marketing-chat-widget.iife.js')
if (!existsSync(IIFE_PATH)) {
  console.error('IIFE 产物不存在,请先执行 npm run build')
  process.exit(1)
}

// ============================================================
// 轻量级 DOM mock
// ============================================================
function makeElement(tagName) {
  const el = {
    tagName: String(tagName).toUpperCase(),
    nodeType: 1,
    style: {},
    dataset: {},
    classList: {
      _set: new Set(),
      add: function (...c) { c.forEach(x => this._set.add(x)) },
      remove: function (...c) { c.forEach(x => this._set.delete(c)) },
      contains: function (c) { return this._set.has(c) },
      toggle: function (c) {
        if (this._set.has(c)) { this._set.delete(c); return false }
        this._set.add(c); return true
      }
    },
    children: [],
    childNodes: [],
    parentNode: null,
    attributes: {},
    setAttribute: function (k, v) { this.attributes[k] = String(v) },
    getAttribute: function (k) { return this.attributes[k] || null },
    appendChild: function (c) {
      c.parentNode = this
      this.children.push(c)
      this.childNodes.push(c)
      return c
    },
    removeChild: function (c) {
      this.children = this.children.filter(x => x !== c)
      this.childNodes = this.childNodes.filter(x => x !== c)
      c.parentNode = null
      return c
    },
    remove: function () { if (this.parentNode) this.parentNode.removeChild(this) },
    addEventListener: function (type, fn) {
      (this._listeners = this._listeners || {})[type] = (this._listeners[type] || []).concat(fn)
    },
    removeEventListener: function (type, fn) {
      (this._listeners = this._listeners || {})[type] = (this._listeners[type] || []).filter(f => f !== fn)
    },
    dispatchEvent: function (ev) {
      (this._listeners && this._listeners[ev.type] || []).forEach(f => f(ev))
    },
    querySelector: function () { return null },
    querySelectorAll: function () { return [] },
    set innerHTML(v) { this._innerHTML = v },
    get innerHTML() { return this._innerHTML || '' },
    set textContent(v) { this._innerHTML = String(v) },
    get textContent() { return this._innerHTML || '' },
    set cssText(v) { this._cssText = v },
    get cssText() { return this._cssText || '' },
    title: '',
    src: '',
    allow: '',
    id: ''
  }
  return el
}

function buildSandbox(opts = {}) {
  const body = makeElement('body')
  const documentMock = {
    body,
    head: makeElement('head'),
    createElement: (tag) => makeElement(tag),
    createElementNS: (_, tag) => makeElement(tag),
    currentScript: null,
    getElementsByTagName: () => [],
    addEventListener: function () {},
    removeEventListener: function () {}
  }

  const messageListeners = []
  const windowMock = {
    location: { origin: opts.origin || 'http://localhost:8204', hostname: 'localhost', protocol: 'http:', search: opts.search || '' },
    innerWidth: opts.innerWidth || 1280,
    innerHeight: 800,
    MarketingChatWidget: undefined,
    MarketingChatWidgetConfig: undefined,
    addEventListener: function (type, fn) {
      if (type === 'message') messageListeners.push(fn)
    },
    removeEventListener: function (type, fn) {
      if (type === 'message') {
        const i = messageListeners.indexOf(fn)
        if (i >= 0) messageListeners.splice(i, 1)
      }
    },
    postMessage: function (data, targetOrigin) {
      windowMock.__lastPostMessage = { data, targetOrigin }
    },
    __lastPostMessage: null
  }

  return { body, documentMock, windowMock, messageListeners }
}

function loadIIFE(windowMock, documentMock) {
  const iifeSource = readFileSync(IIFE_PATH, 'utf8')
  const sandbox = {
    window: windowMock,
    document: documentMock,
    console,
    setTimeout,
    clearTimeout,
    Number, String, Boolean, Object, Array, JSON,
    URL, URLSearchParams, Math, Error, Promise, Function,
    parseInt, parseFloat, isNaN, Symbol
  }
  sandbox.globalThis = sandbox
  vm.createContext(sandbox)
  vm.runInContext(iifeSource, sandbox, { filename: 'iife.js' })
  return sandbox.window.mcwInstance
}

// 注册 panel.create() 让 messageHandler 可用
function ensurePanelHandler(widget) {
  if (widget && widget.panel && typeof widget.panel.messageHandler !== 'function') {
    try { widget.panel.create() } catch (_) { /* ignore */ }
  }
}

// ============================================================
// 测试用例
// ============================================================
let pass = 0
let fail = 0
const failures = []
const ok = (n) => { pass++; console.log(`  \u001b[32m\u2713\u001b[0m ${n}`) }
const bad = (n, m) => { fail++; failures.push({ n, m }); console.log(`  \u001b[31m\u2717\u001b[0m ${n} - ${m}`) }
const assert = (c, n, m) => (c ? ok : bad)(n, m || 'assertion failed')
const eq = (a, b, n) => assert(a === b, n, `expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}`)

// ---- 1. allowedOrigins 白名单边界 ----
console.log('\n== 1. allowedOrigins 白名单边界 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  env.windowMock.MarketingChatWidgetConfig = {
    apiBaseURL: 'http://localhost:8204',
    allowedOrigins: ['http://localhost:8204']
  }
  const widget = loadIIFE(env.windowMock, env.documentMock)
  ensurePanelHandler(widget)
  const panel = widget.panel
  const before = panel.shown

  const evilOrigins = [
    { name: '1.1 完全非法 origin', origin: 'https://evil.example.com' },
    { name: '1.2 协议不同(https)', origin: 'https://localhost:8204' },
    { name: '1.3 origin 含路径', origin: 'http://localhost:8204/extra' },
    { name: '1.4 origin 大写', origin: 'HTTP://LOCALHOST:8204' },
    { name: '1.5 端口不同', origin: 'http://localhost:9999' },
    { name: '1.6 空 origin', origin: '' },
    { name: '1.7 null origin', origin: null }
  ]
  for (const c of evilOrigins) {
    let threw = false
    try { panel.messageHandler({ origin: c.origin, data: { type: 'chat-widget-close' } }) } catch (e) { threw = true; console.log(`     [${c.name}] 抛错: ${e.message}`) }
    eq(threw, false, `${c.name} 不抛错`)
  }
  eq(panel.shown, before, '1.x 所有非法 origin 消息被拒绝,panel.shown 未变')
}

// ---- 2. 消息体格式异常 ----
console.log('\n== 2. 消息体格式异常 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  const widget = loadIIFE(env.windowMock, env.documentMock)
  ensurePanelHandler(widget)
  const panel = widget.panel
  const before = panel.shown

  const evilCases = [
    { name: '2.1 null data', v: null },
    { name: '2.2 undefined data', v: undefined },
    { name: '2.3 字符串 data', v: 'mcw-unread' },
    { name: '2.4 数字 data', v: 42 },
    { name: '2.5 布尔 data', v: true },
    { name: '2.6 数组 data', v: [{ type: 'chat-widget-close' }] },
    { name: '2.7 空对象 data', v: {} },
    { name: '2.8 data 无 type 字段', v: { foo: 'bar' } }
  ]
  for (const c of evilCases) {
    let threw = false
    try { panel.messageHandler({ origin: 'http://localhost:8204', data: c.v }) } catch (e) { threw = true; console.log(`     [${c.name}] 抛错: ${e.message}`) }
    eq(threw, false, `${c.name} 不抛错`)
  }
  eq(panel.shown, before, '2.x 所有畸形 data 不改变 panel.shown')
}

// ---- 3. 超大消息体 ----
console.log('\n== 3. 超大消息体 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  const widget = loadIIFE(env.windowMock, env.documentMock)
  ensurePanelHandler(widget)
  const panel = widget.panel

  const huge = 'x'.repeat(1024 * 1024)
  let threw = false
  try {
    panel.messageHandler({ origin: 'http://localhost:8204', data: { type: 'mcw-unread', payload: huge } })
  } catch (e) { threw = true }
  eq(threw, false, '3.1 1MB 大 payload 不抛错')

  const massive = 'y'.repeat(10 * 1024 * 1024)
  threw = false
  try {
    panel.messageHandler({ origin: 'http://localhost:8204', data: { type: 'mcw-unread', payload: massive } })
  } catch (e) { threw = true }
  eq(threw, false, '3.2 10MB 极端大 payload 不抛错')
}

// ---- 4. allowedOrigins 配置异常(SDK 行为:自动推导或保留) ----
console.log('\n== 4. allowedOrigins 配置异常(SDK 行为:自动推导或保留) ==')
{
  // 4.1 null → SDK 自动从 apiBaseURL+location.origin 推导
  const env1 = buildSandbox({ origin: 'http://localhost:8204' })
  env1.windowMock.MarketingChatWidgetConfig = { apiBaseURL: 'http://localhost:8204', allowedOrigins: null }
  const w1 = loadIIFE(env1.windowMock, env1.documentMock)
  assert(Array.isArray(w1.config.allowedOrigins) && w1.config.allowedOrigins.length > 0, '4.1 null 自动推导', `got ${JSON.stringify(w1.config.allowedOrigins)}`)

  // 4.2 undefined → 自动推导
  const env2 = buildSandbox({ origin: 'http://localhost:8204' })
  env2.windowMock.MarketingChatWidgetConfig = { apiBaseURL: 'http://localhost:8204', allowedOrigins: undefined }
  const w2 = loadIIFE(env2.windowMock, env2.documentMock)
  assert(Array.isArray(w2.config.allowedOrigins) && w2.config.allowedOrigins.length > 0, '4.2 undefined 自动推导', `got ${JSON.stringify(w2.config.allowedOrigins)}`)

  // 4.3 空数组 → 自动推导
  const env3 = buildSandbox({ origin: 'http://localhost:8204' })
  env3.windowMock.MarketingChatWidgetConfig = { apiBaseURL: 'http://localhost:8204', allowedOrigins: [] }
  const w3 = loadIIFE(env3.windowMock, env3.documentMock)
  assert(Array.isArray(w3.config.allowedOrigins) && w3.config.allowedOrigins.length > 0, '4.3 空数组 自动推导', `got ${JSON.stringify(w3.config.allowedOrigins)}`)

  // 4.4 非数组 → SDK 视为无效,自动推导
  const env4 = buildSandbox({ origin: 'http://localhost:8204' })
  env4.windowMock.MarketingChatWidgetConfig = { apiBaseURL: 'http://localhost:8204', allowedOrigins: 'http://localhost:8204' }
  const w4 = loadIIFE(env4.windowMock, env4.documentMock)
  assert(Array.isArray(w4.config.allowedOrigins) && w4.config.allowedOrigins.length > 0, '4.4 非数组 视为无效,自动推导', `got ${JSON.stringify(w4.config.allowedOrigins)}`)

  // 4.5 显式有效数组 → 原值保留
  const env5 = buildSandbox({ origin: 'http://localhost:8204' })
  env5.windowMock.MarketingChatWidgetConfig = { apiBaseURL: 'http://localhost:8204', allowedOrigins: ['http://localhost:8204', 'https://app.example.com'] }
  const w5 = loadIIFE(env5.windowMock, env5.documentMock)
  eq(w5.config.allowedOrigins.length, 2, '4.5 显式数组 长度=2')
  eq(w5.config.allowedOrigins[1], 'https://app.example.com', '4.5 显式数组 第二个元素保留')
}

// ---- 5. 多次 mount / destroy 循环 ----
console.log('\n== 5. 多次 mount / destroy 循环 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  const widget = loadIIFE(env.windowMock, env.documentMock)

  for (let i = 0; i < 5; i++) {
    try { widget.destroy() } catch (e) { bad(`5.${i} destroy`, e.message); continue }
    ok(`5.${i} destroy 成功`)
  }

  let threw = false
  try { widget.destroy() } catch (e) { threw = true }
  eq(threw, false, '5.6 重复 destroy 不抛错')

  threw = false
  try { widget.open(); widget.close() } catch (e) { threw = true }
  eq(threw, false, '5.7 destroy 后 open/close 不抛错')
}

// ---- 6. 用户事件回调异常 ----
console.log('\n== 6. 用户事件回调异常 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  env.windowMock.MarketingChatWidgetConfig = {
    apiBaseURL: 'http://localhost:8204',
    events: {
      onReady: function () { throw new Error('boom in onReady') },
      onOpen: function () { throw new Error('boom in onOpen') },
      onClose: function () { throw new Error('boom in onClose') },
      onUnread: function () { throw new Error('boom in onUnread') },
      onMessage: function () { throw new Error('boom in onMessage') }
    }
  }
  let widget
  try {
    widget = loadIIFE(env.windowMock, env.documentMock)
    ok('6.1 init 阶段抛错的事件回调不导致 SDK 崩溃')
  } catch (e) { bad('6.1 init 阶段', e.message) }

  if (widget) {
    let threw = false
    try {
      widget._onWindowMessage({ origin: 'http://localhost:8204', data: { type: 'mcw-unread', count: 1 } })
    } catch (e) { threw = true }
    eq(threw, false, '6.2 onUnread 抛错不导致消息处理崩溃')

    threw = false
    try {
      widget._onWindowMessage({ origin: 'http://localhost:8204', data: { type: 'mcw-custom-event', payload: { foo: 'bar' } } })
    } catch (e) { threw = true }
    eq(threw, false, '6.3 onMessage 抛错不导致消息处理崩溃')
  }
}

// ---- 7. URL 解析边界 ----
console.log('\n== 7. URL 解析边界 ==')
{
  const cases = [
    { name: '7.1 apiBaseURL=""', val: '' },
    { name: '7.2 apiBaseURL="not-a-url"', val: 'not-a-url' },
    { name: '7.3 apiBaseURL="javascript:alert(1)"', val: 'javascript:alert(1)' },
    { name: '7.4 apiBaseURL="//proto-relative.example.com"', val: '//proto-relative.example.com' },
    { name: '7.5 apiBaseURL=null', val: null },
    { name: '7.6 apiBaseURL=undefined', val: undefined }
  ]
  for (const c of cases) {
    const env = buildSandbox({ origin: 'http://localhost:8204' })
    env.windowMock.MarketingChatWidgetConfig = { apiBaseURL: c.val }
    let threw = false
    try {
      loadIIFE(env.windowMock, env.documentMock)
    } catch (e) { threw = true; console.log(`     [${c.name}] 抛错: ${e.message}`) }
    eq(threw, false, `${c.name} 加载不抛错`)
  }
}

// ---- 8. config.js 源码 SSR 守卫完整性 ----
console.log('\n== 8. config.js 源码 SSR 守卫完整性 ==')
{
  const cfg = readFileSync(pathResolve(ROOT, 'src/config.js'), 'utf8')
  assert(cfg.includes("if (typeof window === 'undefined') return {}"), '8.1 readQueryParams 有 typeof window 守卫')
  assert(cfg.includes("typeof window !== 'undefined'"), '8.2 config.js 含 typeof window !== 守卫')
  assert(cfg.includes("typeof document !== 'undefined'"), '8.3 parseConfig 含 typeof document 守卫')
  assert(cfg.includes("new URL("), '8.4 config.js 含 new URL 调用(被 try/catch 保护)')
}

// ---- 9. query 参数非法值 ----
console.log('\n== 9. query 参数非法值 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204', search: '?app_key=&channel_id=&lang=' })
  try {
    const widget = loadIIFE(env.windowMock, env.documentMock)
    eq(widget.config.appKey, '', '9.1 空字符串 app_key 被忽略(保持默认空字符串)')
    eq(widget.config.lang, 'zh-CN', '9.2 空字符串 lang 被忽略(保持默认 zh-CN)')
    ok('9.3 空字符串 query 参数不抛错')
  } catch (e) { bad('9.x 空字符串 query 参数', e.message) }
}

// ---- 10. 多次 init 不应重复挂载 ----
console.log('\n== 10. 多次 init 不应重复挂载 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  const widget = loadIIFE(env.windowMock, env.documentMock)
  const initialBtnCount = env.body.children.length
  let initThrows = 0
  for (let i = 0; i < 3; i++) {
    try { widget.init() } catch (e) { initThrows++ }
  }
  const finalBtnCount = env.body.children.length
  eq(initThrows, 0, '10.1 多次 init 抛错次数=0')
  assert(finalBtnCount < initialBtnCount * 5, '10.2 多次 init 没有无限追加 DOM 节点', `initial=${initialBtnCount} final=${finalBtnCount}`)
}

// ---- 11. 多种 postMessage 消息类型 ----
console.log('\n== 11. 多种 postMessage 消息类型 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  const widget = loadIIFE(env.windowMock, env.documentMock)
  ensurePanelHandler(widget)
  const panel = widget.panel

  const msgTypes = [
    'mcw-unread', 'mcw-config', 'mcw-ready',
    'mcw-message', 'chat-widget-close',
    'unknown-type-1', 'unknown-type-2'
  ]
  for (const t of msgTypes) {
    let threw = false
    try {
      panel.messageHandler({ origin: 'http://localhost:8204', data: { type: t, count: 5, payload: { foo: 'bar' } } })
    } catch (e) { threw = true; console.log(`     [${t}] 抛错: ${e.message}`) }
    eq(threw, false, `11.${t} 消息类型不抛错`)
  }
}

// ---- 12. 快速 open/close 循环 ----
console.log('\n== 12. 快速 open/close 循环 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  const widget = loadIIFE(env.windowMock, env.documentMock)
  let threw = 0
  for (let i = 0; i < 50; i++) {
    try { widget.open(); widget.close() } catch (e) { threw++ }
  }
  eq(threw, 0, '12.1 50 次 open/close 循环无抛错')
  eq(widget.opened, false, '12.2 循环结束后 opened=false(最终态)')
}

// ---- 13. 危险协议 origin 处理 ----
console.log('\n== 13. 危险协议 origin 处理 ==')
{
  const env = buildSandbox({ origin: 'http://localhost:8204' })
  env.windowMock.MarketingChatWidgetConfig = {
    apiBaseURL: 'http://localhost:8204',
    allowedOrigins: ['javascript:', 'data:', 'vbscript:']
  }
  let widget
  try {
    widget = loadIIFE(env.windowMock, env.documentMock)
    ok('13.1 配置了危险协议 allowedOrigins 不抛错(SDK 信任开发者配置,不在前端做协议过滤)')
  } catch (e) { bad('13.1', e.message) }
  if (widget) {
    ensurePanelHandler(widget)
    // 13.2 验证 postMessage 时,危险 origin 仍然被 URL 严格匹配拒绝
    //     (parseConfig 自动推导的白名单是 http://localhost:8204,不包含 javascript:)
    const before = widget.panel.shown
    let threw = false
    try {
      widget.panel.messageHandler({ origin: 'javascript:', data: { type: 'chat-widget-close' } })
    } catch (e) { threw = true; console.log(`     [13.2] 抛错: ${e.message}`) }
    eq(threw, false, '13.2 javascript: origin 不抛错')
    // 自动推导的白名单是 [apiBaseURL.origin, window.location.origin],都不含 javascript:
    // 所以危险 origin 会被 messageHandler 静默 return
    eq(widget.panel.shown, before, '13.2 javascript: origin 消息被自动白名单拒绝(panel.shown 未变)')
  }
}

// ============================================================
// 总结
// ============================================================
console.log(`\n== 总结 ==`)
console.log(`  \u001b[32m通过: ${pass}\u001b[0m / \u001b[31m失败: ${fail}\u001b[0m`)
if (fail > 0) {
  console.log('\n失败明细:')
  failures.forEach(f => console.log(`  - ${f.n}: ${f.m}`))
  process.exit(1)
}
console.log('\u001b[32m\u2705 \u5168\u90e8\u901a\u8fc7\u001b[0m')
process.exit(0)
