// 闲鱼 content script 入口
// 与抖音/小红书 content-xhs.js 同样模式：构造适配器 + 启动桥接
import { buildXianyuAdapter } from '../channels/xianyu.js';
import { startBridge } from './common.js';
import { CHANNELS } from '../core/types.js';

startBridge(CHANNELS.XIANYU, buildXianyuAdapter);
