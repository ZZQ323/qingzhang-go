// Pinia store：离线优先 + 增量同步
// 思路：所有操作先写本地缓存（秒回，无网也能记），再后台 push；进 App 时 pull 增量。
import { defineStore } from 'pinia';
import { api } from '@/api/request';

const LS_RECORDS = 'qz_records';
const LS_SINCE   = 'qz_since';
const LS_QUEUE   = 'qz_pushqueue'; // 待推送队列（离线时暂存）

function load(key, def) {
  try { return JSON.parse(uni.getStorageSync(key)) || def; } catch { return def; }
}
function save(key, val) { uni.setStorageSync(key, JSON.stringify(val)); }

// 本地临时 id，推送成功后服务端会回真实自增 id
function tmpId() { return 'tmp_' + Date.now() + '_' + Math.random().toString(36).slice(2, 6); }

export const useLedger = defineStore('ledger', {
  state: () => ({
    records: load(LS_RECORDS, []),  // 已确认的记录（带服务端 id）
    queue: load(LS_QUEUE, []),      // 待推送的本地变更
    since: uni.getStorageSync(LS_SINCE) || '',
    syncing: false
  }),

  getters: {
    // 对外可见的记录 = 服务端记录 + 队列里的本地变更，过滤软删
    visible: (s) => {
      const map = {};
      s.records.forEach(r => { map[r.id] = r; });
      s.queue.forEach(r => { map[r.id] = r; });   // 本地变更覆盖
      return Object.values(map)
        .filter(r => !r.isDeleted)
        .sort((a, b) => (b.happenedAt + '' + b.id).localeCompare(a.happenedAt + '' + a.id));
    }
  },

  actions: {
    persist() { save(LS_RECORDS, this.records); save(LS_QUEUE, this.queue); },

    // 新增/编辑一条
    upsert(rec) {
      if (!rec.id) rec.id = tmpId();
      rec.updatedAt = new Date().toISOString();
      const i = this.queue.findIndex(r => r.id === rec.id);
      if (i >= 0) this.queue[i] = rec; else this.queue.push(rec);
      this.persist();
      this.syncPush(); // 异步尝试推送，失败也没关系，下次再推
    },

    // 软删
    remove(id) {
      const r = this.visible.find(x => x.id === id);
      if (!r) return;
      this.upsert({ ...r, isDeleted: 1 });
    },

    // 推送队列到服务端
    async syncPush() {
      if (this.syncing || this.queue.length === 0) return;
      if (!uni.getStorageSync('token')) return;
      this.syncing = true;
      try {
        // 临时 id 的记录上传时去掉 id，让服务端生成
        const payload = this.queue.map(r => {
          const c = { ...r };
          if (typeof c.id === 'string' && c.id.startsWith('tmp_')) delete c.id;
          return c;
        });
        await api.push(payload);
        this.queue = [];
        this.persist();
        await this.syncPull(); // 推完拉一次，拿到服务端真实 id
      } catch (e) {
        // 离线或失败，保留队列，下次再试
      } finally {
        this.syncing = false;
      }
    },

    // 拉取增量
    async syncPull() {
      if (!uni.getStorageSync('token')) return;
      try {
        const res = await api.pull(this.since);
        const map = {};
        this.records.forEach(r => { map[r.id] = r; });
        res.records.forEach(r => { map[r.id] = r; }); // 服务端为准
        this.records = Object.values(map);
        this.since = res.serverTime;
        uni.setStorageSync(LS_SINCE, this.since);
        this.persist();
      } catch (e) {}
    },

    // App 启动时调用
    async init() {
      await this.syncPush();
      await this.syncPull();
    }
  }
});
