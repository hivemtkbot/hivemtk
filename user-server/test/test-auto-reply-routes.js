const axios = require('axios');

const API_BASE = 'http://localhost:8204/api';
const USERNAME = 'admin';
const PASSWORD = '123456';

async function testAutoReplyRoutes() {
    console.log('🚀 Starting Auto Reply Routes Test...');

    try {
        // 1. Login
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

        // 2. Test Get Rule (Singular)
        console.log('📦 Testing Get Rule (Singular)...');
        try {
            const ruleRes = await axios.get(`${API_BASE}/auto-reply/rule`, { ...config, params: { platform: 'douyin' } });
            console.log(`✅ Get Rule: ${ruleRes.status} ${ruleRes.statusText}`);
            if (ruleRes.data.data.rule) {
                console.log('   Rule Data:', JSON.stringify(ruleRes.data.data.rule));
            } else {
                console.log('   Rule Data: null (expected if no rule set)');
            }
        } catch (error) {
             console.error(`❌ Get Rule Failed: ${error.message}`);
             if (error.response) console.log(`   Status: ${error.response.status}`);
        }

        // 3. Test Save Cookies Route (Check 404)
        console.log('📦 Testing Save Cookies Route...');
        try {
            // Using a fake ID 99999
            await axios.post(`${API_BASE}/auto-reply/accounts/99999/cookies`, { cookie: 'test' }, config);
            console.log(`✅ Save Cookies Route Exists`);
        } catch (error) {
            if (error.response && error.response.status === 404) {
                 console.error(`❌ Save Cookies Route NOT FOUND (404)`);
            } else if (error.response && error.response.status === 500) {
                 // 500 is expected because ID doesn't exist or DB error, but route exists
                 console.log(`✅ Save Cookies Route Exists (Got 500 as expected for fake ID)`);
            } else {
                 console.log(`✅ Save Cookies Route Exists (Status: ${error.response ? error.response.status : error.message})`);
            }
        }
        
        // 4. Test Login Status Route
        console.log('📦 Testing Login Status Route...');
        try {
            await axios.get(`${API_BASE}/auto-reply/login-status`, { ...config, params: { platform: 'douyin', username: 'test' } });
            console.log(`✅ Login Status Route Exists`);
        } catch (error) {
            if (error.response && error.response.status === 404) {
                 console.error(`❌ Login Status Route NOT FOUND (404)`);
            } else {
                 console.log(`✅ Login Status Route Exists (Status: ${error.response ? error.response.status : error.message})`);
            }
        }

    } catch (error) {
        console.error('❌ Test Setup Failed:', error.message);
    }
}

testAutoReplyRoutes();
