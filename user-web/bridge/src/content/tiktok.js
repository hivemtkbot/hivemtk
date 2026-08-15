import { buildTiktokAdapter } from '../channels/tiktok.js';
import { startBridge } from './common.js';
import { CHANNELS } from '../core/types.js';

startBridge(CHANNELS.TIKTOK, buildTiktokAdapter);

