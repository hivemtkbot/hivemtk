import { http } from '@/utils/request'

// 桥接通道凭证管理（v3 BRIDGE_TOKEN_PROTOCOL）
export const getBridgeTokenStatus = () => http.get('/api/bridge/token/status')

export const resetBridgeToken = () => http.post('/api/bridge/token/reset')
