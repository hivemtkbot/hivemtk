// 批量调度：读 manifest.json，挑选 pending 路由，并发派生子进程执行 walk.mjs
// 主进程监控进度并回写 manifest（持久化，避免任务丢失）
// 用法:
//   node run_batch.mjs [limit] [concurrency]            # 跑 pending 路由
//   node run_batch.mjs retry [limit] [concurrency]      # 重跑 issue/error 路由（默认串行）
import { spawn } from 'child_process'
import { readFileSync, writeFileSync, existsSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const USERWEB = join(__dirname, '..')
const MANIFEST = join(__dirname, 'manifest.json')
const BASE = 'http://127.0.0.1:8211'

const mode = process.argv[2] === 'retry' ? 'retry' : 'pending'
const limit = parseInt(mode === 'retry' ? process.argv[3] : process.argv[2] || '6', 10)
const concurrency = parseInt(mode === 'retry' ? process.argv[4] || '1' : process.argv[3] || '2', 10)

function safeName(p) {
  return p.replace(/^\//, '').replace(/\//g, '_') || 'root'
}

async function runOne(route) {
  return new Promise((resolve) => {
    const args = [join(__dirname, 'walk.mjs'), route.path, BASE, route.needsAuth ? 'true' : 'false']
    const child = spawn(process.execPath, args, {
      cwd: USERWEB,
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    let out = ''
    child.stdout.on('data', (d) => (out += d))
    child.stderr.on('data', (d) => (out += d))
    const timer = setTimeout(() => {
      child.kill('SIGKILL')
      resolve({ route, exitCode: -1, out: out + '\n[TIMEOUT]' })
    }, 300000)
    child.on('close', (code) => {
      clearTimeout(timer)
      resolve({ route, exitCode: code, out })
    })
  })
}

async function main() {
  const manifest = JSON.parse(readFileSync(MANIFEST, 'utf8'))
  const pending = manifest.routes
    .filter((r) => (mode === 'retry' ? (r.status === 'issue' || r.status === 'error') : r.status === 'pending'))
    .slice(0, limit)
  if (pending.length === 0) {
    console.log(`没有 ${mode} 路由可跑。`)
    return
  }
  console.log(`[mode=${mode}] 调度 ${pending.length} 个路由，并发 ${concurrency}...`)

  const queue = [...pending]
  const running = new Set()
  const results = []

  async function pump() {
    while (queue.length > 0 && running.size < concurrency) {
      const route = queue.shift()
      running.add(route.path)
      console.log(`[start] ${route.path} (${route.title || route.name})`)
      runOne(route).then((res) => {
        running.delete(route.path)
        results.push(res)
        console.log(`[done ] ${route.path} exit=${res.exitCode}`)
      })
    }
  }
  // 路由索引（供并发池内即时回写）
  const byPath = Object.fromEntries(manifest.routes.map((r) => [r.path, r]))
  // 简易并发池
  await new Promise((resolveAll) => {
    let active = 0
    const startNext = () => {
      if (queue.length === 0 && active === 0) return resolveAll()
      while (queue.length > 0 && active < concurrency) {
      const route = queue.shift()
      active++
      console.log(`[start] ${route.path} (${route.title || route.name})`)
      runOne(route).then((res) => {
        // 每完成一个路由立即回写 manifest（避免中断丢失进度）
        const r = byPath[res.route.path]
        const reportPath = join(__dirname, 'reports', safeName(r.path) + '.json')
        if (res.exitCode === -1) { r.status = 'error'; r.note = '超时/被杀死' }
        else if (existsSync(reportPath)) {
          try {
            const rep = JSON.parse(readFileSync(reportPath, 'utf8'))
            if (rep.loadError) { r.status = 'error'; r.note = '加载失败: ' + rep.loadError.slice(0, 120) }
            else if ((rep.summary && rep.summary.withIssue > 0) || (rep.pageLevelIssues && rep.pageLevelIssues.length)) {
              r.status = 'issue'
              const reasons = (rep.summary?.issues || []).map((i) => `#${i.index} ${i.text}: ${i.reason}`)
              if (rep.pageLevelIssues) reasons.push('PAGE: ' + rep.pageLevelIssues.join(' | '))
              r.note = reasons.slice(0, 5).join(' || ').slice(0, 400)
            } else { r.status = 'done'; r.note = '' }
            r.report = 'reports/' + safeName(r.path) + '.json'
          } catch (e) { r.status = 'error'; r.note = '报告解析失败: ' + String(e).slice(0, 120) }
        } else { r.status = 'error'; r.note = '无报告文件' }
        r.updatedAt = new Date().toISOString()
        writeFileSync(MANIFEST, JSON.stringify(manifest, null, 2))
        results.push(res)
        active--
        console.log(`[done ] ${route.path} -> ${r.status}`)
        startNext()
      })
      }
    }
    startNext()
  })

  // 回写 manifest（汇总统计）
  let doneCnt = 0, issueCnt = 0, errCnt = 0
  for (const res of results) {
    const r = byPath[res.route.path]
    const reportPath = join(__dirname, 'reports', safeName(r.path) + '.json')
    let status = 'done'
    let note = ''
    if (res.exitCode === -1) {
      status = 'error'
      note = '超时/被杀死'
      errCnt++
    } else if (existsSync(reportPath)) {
      try {
        const rep = JSON.parse(readFileSync(reportPath, 'utf8'))
        if (rep.loadError) {
          status = 'error'
          note = '加载失败: ' + rep.loadError.slice(0, 120)
          errCnt++
        } else if ((rep.summary && rep.summary.withIssue > 0) || (rep.pageLevelIssues && rep.pageLevelIssues.length)) {
          status = 'issue'
          const reasons = (rep.summary?.issues || []).map((i) => `#${i.index} ${i.text}: ${i.reason}`)
          if (rep.pageLevelIssues) reasons.push('PAGE: ' + rep.pageLevelIssues.join(' | '))
          note = reasons.slice(0, 5).join(' || ').slice(0, 400)
          issueCnt++
        } else {
          status = 'done'
          doneCnt++
        }
        r.report = 'reports/' + safeName(r.path) + '.json'
      } catch (e) {
        status = 'error'
        note = '报告解析失败: ' + String(e).slice(0, 120)
        errCnt++
      }
    } else {
      status = 'error'
      note = '无报告文件'
      errCnt++
    }
    r.status = status
    r.note = note
    r.updatedAt = new Date().toISOString()
  }
  writeFileSync(MANIFEST, JSON.stringify(manifest, null, 2))
  console.log(`\n批次完成: done=${doneCnt} issue=${issueCnt} error=${errCnt}`)
}

main().catch((e) => {
  console.error('调度器崩溃:', e)
  process.exit(1)
})
