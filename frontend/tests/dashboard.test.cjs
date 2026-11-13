// Exercises the real component's hooks/API wiring without a browser or accounts.
const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

const source = fs.readFileSync(path.join(__dirname, '../src/pages/Dashboard.tsx'), 'utf8')
const code = ts.transpileModule(source, { compilerOptions: {
  module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX,
}}).outputText

function mount() {
  const cells = [], effects = [], requests = [], timers = new Set()
  let index = 0, event, unsubscribed = false
  const react = {
    useState(value) {
      const i = index++; cells[i] = value
      return [value, next => { cells[i] = typeof next === 'function' ? next(cells[i]) : next }]
    },
    useRef(value) { const ref = { current: value }; cells[index++] = ref; return ref },
    useEffect(effect) { effects.push(effect) },
  }
  const api = {
    getDashboard() { return new Promise((resolve, reject) => requests.push({ resolve, reject })) },
  }
  const exports = {}
  vm.runInNewContext(code, {
    exports,
    require(name) {
      if (name === 'react') return react
      if (name === 'react/jsx-runtime') return { jsx: (...args) => args, jsxs: (...args) => args }
      if (name === '../lib/api') return { api, onAnyLog(cb) { event = cb; return () => { event = undefined; unsubscribed = true } } }
      return {}
    },
    setInterval(fn) { timers.add(fn); return fn },
    clearInterval(fn) { timers.delete(fn) },
  })
  exports.default()
  const cleanups = effects.map(f => f())
  return {
    cells, requests, timers,
    emit(msg) { event?.({ uid: 'account', msg }) },
    poll() { for (const fn of timers) fn() },
    unmount() { cleanups.forEach(f => f?.()); assert.ok(unsubscribed); assert.equal(timers.size, 0) },
  }
}
const response = logs => ({ ok: true, data: { totalAccounts: 1, runningTasks: 1, configPath: 'test', configOK: true, recentLogs: logs } })
const settle = () => new Promise(resolve => setImmediate(resolve))

test('dashboard loads history, receives live logs and bounds history to 20', async () => {
  const h = mount()
  h.requests[0].resolve(response(['history'])); await settle()
  h.emit('live')
  assert.equal(h.cells[0].recentLogs.join(','), 'history,live')
  for (let i = 0; i < 30; i++) h.emit(`line-${i}`)
  assert.equal(h.cells[0].recentLogs.length, 20)
  assert.equal(h.cells[0].recentLogs.at(-1), 'line-29')
  h.unmount()
})

test('late dashboard snapshot cannot overwrite a newer event', async () => {
  const h = mount()
  h.requests[0].resolve(response(['history'])); await settle()
  h.poll(); h.emit('newer')
  h.requests[1].resolve(response(['stale'])); await settle()
  assert.equal(h.cells[0].recentLogs.join(','), 'history,newer')
  h.unmount()
})

test('events arriving before initial snapshot are preserved', async () => {
  const h = mount()
  h.emit('early')
  h.requests[0].resolve(response([])); await settle()
  assert.equal(h.cells[0].recentLogs.join(','), 'early')
  h.unmount()
})

test('failed/out-of-order requests and unmount do not erase live state', async () => {
  const h = mount()
  h.requests[0].resolve(response(['history'])); await settle()
  h.poll(); h.poll()
  h.requests[2].resolve(response(['latest'])); await settle()
  h.requests[1].resolve(response(['old'])); await settle()
  assert.equal(h.cells[0].recentLogs.join(','), 'latest')
  h.poll(); h.requests[3].reject(new Error('offline')); await settle()
  assert.equal(h.cells[0].recentLogs.join(','), 'latest')
  h.poll(); h.unmount()
  h.requests[4].resolve(response(['after-unmount'])); await settle()
  assert.equal(h.cells[0].recentLogs.join(','), 'latest')
})
