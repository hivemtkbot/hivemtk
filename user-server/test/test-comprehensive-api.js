const axios = require('axios');
const jwt = require('jsonwebtoken');

// JWT配置
const JWT_SECRET = 'marketing-system-secret-key';
const JWT_ISSUER = 'marketing-system';

// API基础URL
const BASE_URL = 'http://localhost:8204';

// 生成有效的JWT令牌
function generateValidToken() {
    const payload = {
        user_id: 1,
        username: 'admin',
        role: 'admin',
        license_id: 'system_admin', // 平台用户使用system_admin
        iss: JWT_ISSUER,
        iat: Math.floor(Date.now() / 1000),
        exp: Math.floor(Date.now() / 1000) + (24 * 60 * 60)
    };
    return jwt.sign(payload, JWT_SECRET, { algorithm: 'HS256' });
}

// 测试总结
const testResults = {
    total: 0,
    passed: 0,
    failed: 0,
    details: []
};

// 测试函数
async function testEndpoint(name, method, url, data = null, expectedStatus = 200) {
    testResults.total++;
    
    try {
        const token = generateValidToken();
        const config = {
            method: method,
            url: `${BASE_URL}${url}`,
            headers: {
                'Authorization': `Bearer ${token}`,
                'Content-Type': 'application/json'
            },
            validateStatus: () => true // 不抛出错误，手动处理状态码
        };
        
        if (data && (method === 'POST' || method === 'PUT')) {
            config.data = data;
        }
        
        const response = await axios(config);
        
        const passed = response.status === expectedStatus;
        
        if (passed) {
            testResults.passed++;
            console.log(`✅ ${name}: ${response.status} ${response.statusText}`);
        } else {
            testResults.failed++;
            console.log(`❌ ${name}: 期望 ${expectedStatus}, 实际 ${response.status} ${response.statusText}`);
        }
        
        testResults.details.push({
            name,
            method,
            url,
            expectedStatus,
            actualStatus: response.status,
            passed,
            response: response.data
        });
        
        return response;
    } catch (error) {
        testResults.failed++;
        console.log(`❌ ${name}: 错误 - ${error.message}`);
        testResults.details.push({
            name,
            method,
            url,
            expectedStatus,
            actualStatus: 'ERROR',
            passed: false,
            error: error.message
        });
        return null;
    }
}

// 主测试函数
async function runComprehensiveTests() {
    console.log('🚀 开始综合API功能测试...\n');
    
    // 1. 素材管理API测试
    console.log('📁 素材管理API测试:');
    await testEndpoint('素材列表', 'GET', '/api/material/list');
    await testEndpoint('素材分类列表', 'GET', '/api/material/categories');
    await testEndpoint('素材统计', 'GET', '/api/material/stats');
    await testEndpoint('素材选择器', 'GET', '/api/material/selector');
    
    // 2. OBS配置API测试
    console.log('\n☁️ OBS配置API测试:');
    await testEndpoint('OBS配置列表', 'GET', '/api/obs/config');
    await testEndpoint('默认OBS配置', 'GET', '/api/obs/config/default');
    
    // 3. 活码管理API测试
    console.log('\n📱 活码管理API测试:');
    await testEndpoint('活码列表', 'GET', '/api/live-codes/list');
    
    // 4. 平台集成API测试
    console.log('\n🔧 平台集成API测试:');
    await testEndpoint('最新消息', 'GET', '/api/platform/message/latest');
    await testEndpoint('授权状态', 'GET', '/api/platform/license/status');
    
    // 5. 用户认证API测试
    console.log('\n👤 用户认证API测试:');
    await testEndpoint('当前用户信息', 'GET', '/api/user');
    await testEndpoint('用户列表', 'GET', '/api/users');
    
    // 6. 测试一些POST接口（创建操作）
    console.log('\n➕ 创建操作API测试:');
    await testEndpoint('创建素材分类', 'POST', '/api/material/categories', {
        name: '测试分类',
        type: 'image',
        description: '测试描述'
    });
    
    await testEndpoint('创建OBS配置', 'POST', '/api/obs/config', {
        name: '测试配置',
        provider: 'aliyun',
        access_key: 'test-key',
        secret_key: 'test-secret',
        bucket: 'test-bucket',
        region: 'cn-hangzhou'
    });
    
    await testEndpoint('创建活码', 'POST', '/api/live-codes/create', {
        name: '测试活码',
        description: '测试描述',
        entry_url: 'https://example.com/entry',
        landing_url: 'https://example.com/landing'
    });
    
    // 输出测试总结
    console.log('\n' + '='.repeat(60));
    console.log('📊 测试总结:');
    console.log(`总测试数: ${testResults.total}`);
    console.log(`✅ 通过: ${testResults.passed}`);
    console.log(`❌ 失败: ${testResults.failed}`);
    console.log(`📈 成功率: ${((testResults.passed / testResults.total) * 100).toFixed(1)}%`);
    
    if (testResults.failed > 0) {
        console.log('\n❌ 失败的测试详情:');
        testResults.details.filter(d => !d.passed).forEach(detail => {
            console.log(`  - ${detail.name}: ${detail.method} ${detail.url}`);
            console.log(`    期望: ${detail.expectedStatus}, 实际: ${detail.actualStatus}`);
            if (detail.error) {
                console.log(`    错误: ${detail.error}`);
            }
        });
    }
    
    console.log('\n✅ 通过的测试详情:');
    testResults.details.filter(d => d.passed).forEach(detail => {
        console.log(`  - ${detail.name}: ${detail.method} ${detail.url}`);
    });
    
    return testResults;
}

// 运行测试
runComprehensiveTests().then(results => {
    console.log('\n🎉 测试完成！');
    process.exit(results.failed > 0 ? 1 : 0);
}).catch(error => {
    console.error('❌ 测试执行失败:', error);
    process.exit(1);
});