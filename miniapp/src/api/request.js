// 统一请求封装：自动带 token，401 自动跳登录
const BASE = 'https://api.yourdomain.com'; // 换成你的后端域名

function request(opt) {
  const token = uni.getStorageSync('token');
  return new Promise((resolve, reject) => {
    uni.request({
      url: BASE + opt.url,
      method: opt.method || 'GET',
      data: opt.data || {},
      header: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: 'Bearer ' + token } : {})
      },
      success: (res) => {
        if (res.statusCode === 401) {
          uni.removeStorageSync('token');
          reject(new Error('未登录'));
          return;
        }
        const body = res.data;
        if (body && body.code === 0) resolve(body.data);
        else reject(new Error(body?.msg || '请求失败'));
      },
      fail: (e) => reject(e)
    });
  });
}

export const api = {
  login: (code) => request({ url: '/api/auth/login', method: 'POST', data: { code } }),
  pull: (since) => request({ url: '/api/sync/pull' + (since ? '?since=' + encodeURIComponent(since) : '') }),
  push: (records) => request({ url: '/api/sync/push', method: 'POST', data: { records } })
};
