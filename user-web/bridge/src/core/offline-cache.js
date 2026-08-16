/**
 * Bridge 离线消息缓存（USR-BR-02）
 * 借鉴：Service Worker 离线存储
 * 使用 IndexedDB 缓存未上报消息，断网时本地堆积，恢复后批量重发
 */

const DB_NAME = 'hivemtk-bridge'
const DB_VERSION = 1
const STORE_NAME = 'pending_messages'

class OfflineCache {
  constructor() {
    this.db = null
  }

  async open() {
    if (this.db) return this.db
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION)
      req.onupgradeneeded = (e) => {
        const db = e.target.result
        if (!db.objectStoreNames.contains(STORE_NAME)) {
          const store = db.createObjectStore(STORE_NAME, { keyPath: 'id' })
          store.createIndex('channel', 'channel')
          store.createIndex('timestamp', 'timestamp')
        }
      }
      req.onsuccess = (e) => {
        this.db = e.target.result
        resolve(this.db)
      }
      req.onerror = (e) => reject(e)
    })
  }

  async put(message) {
    const db = await this.open()
    return new Promise((resolve, reject) => {
      const tx = db.transaction([STORE_NAME], 'readwrite')
      const store = tx.objectStore(STORE_NAME)
      store.put(message)
      tx.oncomplete = () => resolve()
      tx.onerror = (e) => reject(e)
    })
  }

  async getAll() {
    const db = await this.open()
    return new Promise((resolve, reject) => {
      const tx = db.transaction([STORE_NAME], 'readonly')
      const store = tx.objectStore(STORE_NAME)
      const req = store.getAll()
      req.onsuccess = (e) => resolve(e.target.result || [])
      req.onerror = (e) => reject(e)
    })
  }

  async delete(id) {
    const db = await this.open()
    return new Promise((resolve, reject) => {
      const tx = db.transaction([STORE_NAME], 'readwrite')
      const store = tx.objectStore(STORE_NAME)
      store.delete(id)
      tx.oncomplete = () => resolve()
      tx.onerror = (e) => reject(e)
    })
  }

  async clear() {
    const db = await this.open()
    return new Promise((resolve, reject) => {
      const tx = db.transaction([STORE_NAME], 'readwrite')
      const store = tx.objectStore(STORE_NAME)
      store.clear()
      tx.oncomplete = () => resolve()
      tx.onerror = (e) => reject(e)
    })
  }

  async count() {
    const db = await this.open()
    return new Promise((resolve, reject) => {
      const tx = db.transaction([STORE_NAME], 'readonly')
      const store = tx.objectStore(STORE_NAME)
      const req = store.count()
      req.onsuccess = (e) => resolve(e.target.result)
      req.onerror = (e) => reject(e)
    })
  }
}

export default new OfflineCache()
export { OfflineCache }
