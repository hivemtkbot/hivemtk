import { buildXianyuAdapter } from '../channels/xianyu.js';
import { startBridge } from './common.js';
import { CHANNELS } from '../core/types.js';

startBridge(CHANNELS.XIANYU, buildXianyuAdapter);

