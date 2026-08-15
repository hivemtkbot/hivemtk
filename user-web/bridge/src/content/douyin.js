import { buildDouyinAdapter } from '../channels/douyin.js';
import { startBridge } from './common.js';
import { CHANNELS } from '../core/types.js';

startBridge(CHANNELS.DOUYIN, buildDouyinAdapter);

