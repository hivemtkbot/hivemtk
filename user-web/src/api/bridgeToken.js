import { http } from '@/utils/request'

export const getBridgeTokenStatus = () => http.get('/api/bridge/token/status');

export const resetBridgeToken = () => http.post('/api/bridge/token/reset')
