import { buildXhsAdapter } from '../channels/xhs.js';
import { startBridge } from './common.js';
import { CHANNELS } from '../core/types.js';

startBridge(CHANNELS.XHS, buildXhsAdapter);
