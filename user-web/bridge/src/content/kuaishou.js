import { buildKuaishouAdapter } from '../channels/kuaishou.js';
import { startBridge } from './common.js';
import { CHANNELS } from '../core/types.js';

startBridge(CHANNELS.KUAISHOU, buildKuaishouAdapter);