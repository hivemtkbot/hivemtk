const axios = require('axios');

const API_BASE = 'http://localhost:8204/api';
const USERNAME = 'admin';
const PASSWORD = '123456';

async function testUIRoutes() {
    console.log('🚀 Starting UI Routes Test...');

    try {
        // 1. Login to get token
        console.log('📦 Logging in...');
        const loginRes = await axios.post(`${API_BASE}/auth/login`, {
            username: USERNAME,
            password: PASSWORD
        });
        const token = loginRes.data.data.token;
        console.log('✅ Login successful');

        const config = {
            headers: { Authorization: `Bearer ${token}` }
        };

        // 2. Test GetLatestMessage
        console.log('📦 Testing /platform/message/latest...');
        try {
            const latestMsgRes = await axios.get(`${API_BASE}/platform/message/latest`, config);
            console.log(`✅ GetLatestMessage: ${latestMsgRes.status} ${latestMsgRes.statusText}`);
        } catch (error) {
            console.error(`❌ GetLatestMessage Failed: ${error.message}`);
            if (error.response) {
                console.log(`   Status: ${error.response.status}`);
            }
        }

        // 3. Test License Status Alias
        console.log('📦 Testing /platform/license/status (Alias)...');
        try {
            const licenseRes = await axios.get(`${API_BASE}/platform/license/status`, config);
            console.log(`✅ License Status: ${licenseRes.status} ${licenseRes.statusText}`);
        } catch (error) {
            console.error(`❌ License Status Failed: ${error.message}`);
             if (error.response) {
                console.log(`   Status: ${error.response.status}`);
            }
        }
        
        // 4. Test Live Codes Alias
        console.log('📦 Testing /live-codes/list (Alias)...');
        try {
            const liveCodesRes = await axios.get(`${API_BASE}/live-codes/list`, config);
            console.log(`✅ Live Codes List: ${liveCodesRes.status} ${liveCodesRes.statusText}`);
        } catch (error) {
            console.error(`❌ Live Codes List Failed: ${error.message}`);
             if (error.response) {
                console.log(`   Status: ${error.response.status}`);
            }
        }

    } catch (error) {
        console.error('❌ Test Setup Failed:', error.message);
    }
}

testUIRoutes();
