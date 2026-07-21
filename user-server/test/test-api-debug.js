const axios = require('axios');

async function testAPI() {
    try {
        // 1. 用户登录
        const loginResponse = await axios.post('http://localhost:8204/api/auth/login', {
            username: 'admin',
            password: '123456'
        });
        
        const token = loginResponse.data.data.token;
        const headers = { Authorization: `Bearer ${token}` };
        
        console.log('✅ 登录成功');
        
        // 2. 获取当前无头模式状态
        console.log('\n📍 获取当前无头模式状态...');
        const getResponse = await axios.get('http://localhost:8204/api/auto-reply/headless', { headers });
        console.log('响应状态:', getResponse.status);
        console.log('响应数据:', JSON.stringify(getResponse.data, null, 2));
        
        // 3. 设置无头模式
        console.log('\n📍 设置无头模式为false...');
        const setResponse = await axios.post('http://localhost:8204/api/auto-reply/headless', {
            headless: false
        }, { headers });
        console.log('响应状态:', setResponse.status);
        console.log('响应数据:', JSON.stringify(setResponse.data, null, 2));
        
    } catch (error) {
        console.error('❌ 错误详情:');
        console.error('错误消息:', error.message);
        
        if (error.response) {
            console.error('响应状态:', error.response.status);
            console.error('响应数据:', JSON.stringify(error.response.data, null, 2));
            console.error('响应头:', error.response.headers);
        } else if (error.request) {
            console.error('请求已发出但没有收到响应');
            console.error('请求:', error.request);
        } else {
            console.error('请求配置错误:', error.message);
        }
    }
}

testAPI();