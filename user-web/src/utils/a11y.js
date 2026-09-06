/**
 * a11y 接入（USR-PF-04）
 * 工具：键盘导航、ARIA 标签、focus 管理
 * 已有：eslint-plugin-vuejs-accessibility（devDep）
 */

let _focusTrapStack = []

export const trapFocus = (container) => {
  if (!container) return () => {}
  const focusable = container.querySelectorAll(
    'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
  )
  if (focusable.length === 0) return () => {}
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  const previousActive = document.activeElement

  const onKey = (e) => {
    if (e.key !== 'Tab') return
    if (e.shiftKey) {
      if (document.activeElement === first) {
        e.preventDefault()
        last.focus()
      }
    } else {
      if (document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
  }

  container.addEventListener('keydown', onKey)
  first.focus()
  _focusTrapStack.push({ container, onKey, previousActive })

  return () => {
    container.removeEventListener('keydown', onKey)
    if (previousActive && previousActive.focus) previousActive.focus()
    _focusTrapStack = _focusTrapStack.filter((x) => x.container !== container)
  }
};

let _announcer = null;
export const announce = (message, priority = 'polite') => {
  if (typeof document === 'undefined') return
  if (!_announcer) {
    _announcer = document.createElement('div')
    _announcer.setAttribute('role', 'status')
    _announcer.setAttribute('aria-live', priority)
    _announcer.setAttribute('aria-atomic', 'true')
    _announcer.style.cssText = 'position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0;'
    document.body.appendChild(_announcer)
  }
  _announcer.textContent = ''
  setTimeout(() => (_announcer.textContent = message), 100)
}

export const checkContrast = (foreground, background) => {
  const lum = (rgb) => {
    const [r, g, b] = rgb.map((v) => {
      v /= 255
      return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
    })
    return 0.2126 * r + 0.7152 * g + 0.0722 * b
  }
  const parseColor = (c) => {
    if (c.startsWith('#')) {
      const hex = c.slice(1)
      return [parseInt(hex.slice(0, 2), 16), parseInt(hex.slice(2, 4), 16), parseInt(hex.slice(4, 6), 16)]
    }
    return [0, 0, 0]
  }
  const fg = parseColor(foreground)
  const bg = parseColor(background)
  const l1 = lum(fg) + 0.05
  const l2 = lum(bg) + 0.05
  return l1 > l2 ? l1 / l2 : l2 / l1
};

export const isAccessible = (fg, bg) => checkContrast(fg, bg) >= 4.5

export default { trapFocus, announce, checkContrast, isAccessible }
