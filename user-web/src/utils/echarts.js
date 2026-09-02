/**
 * ECharts 幂等初始化工具
 *
 * 背景（2026-09-01 全页面巡测实测）：
 *   全站 45 处 `echarts.init(el)` 调用点，没有任何一处做幂等防护。
 *   典型崩法：组件在 onMounted 的 setTimeout 里 init 一次，数据回来后在 fetch 回调里
 *   又 init 一次（如 views/xianyuCard/Stats.vue 的 250 行与 301 行）。
 *   后果：
 *     1) 控制台刷 `[ECharts] There is a chart instance already initialized on the dom.`
 *     2) 第一个实例被变量覆盖后永远拿不到引用 → 无法 dispose → DOM 卸载后仍持有
 *        定时器与事件监听，形成内存泄漏
 *     3) 旧实例可能继续响应 resize 往已废弃的 DOM 上绘制，造成图表错乱
 *
 * 设计：与原生 `echarts.init(el, theme, opts)` 语义一一对应，可直接平替，
 *       不引入新的调用契约，避免大范围重构风险。
 *   - 容器上已有实例 → 复用并返回（天然幂等，无 dispose/init 抖动与闪烁）
 *   - 无实例 → 走原生 init
 *   - el 为空 → 返回 null（由调用方决定，不再抛 "Initialize failed: invalid dom"）
 */
import * as echarts from 'echarts'

/**
 * 幂等初始化图表
 * @param {HTMLElement|null} el 图表容器
 * @param {string|object} [theme] 主题
 * @param {object} [opts] init 选项
 * @returns {echarts.ECharts|null}
 */
export function safeInit(el, theme, opts) {
  if (!el) return null
  const existing = echarts.getInstanceByDom(el)
  if (existing) return existing
  return echarts.init(el, theme, opts)
}

/**
 * 安全销毁：容器被 Vue 卸载后 el 可能已脱离文档，直接 dispose 原生实例即可
 * @param {echarts.ECharts|null|undefined} inst
 */
export function safeDispose(inst) {
  if (!inst || typeof inst.dispose !== 'function') return
  try {
    if (!inst.isDisposed()) inst.dispose()
  } catch {
    /* 已销毁则忽略 */
  }
}

export { echarts }
export default { safeInit, safeDispose, echarts }
