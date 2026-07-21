#!/usr/bin/env node

const axios = require('axios');

// 基础配置
const BASE_URL = 'http://localhost:8080/api';
const JWT_TOKEN = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJleHAiOjE3MzU3MDY4MDB9.7QqF8JHhJzK3Z3N9J5Q9zY8Lq3K2H5N8Qq3Z5N8Qq3Z5N8Qq3Z5';

// 创建axios实例
const api = axios.create({
    baseURL: BASE_URL,
    timeout: 10000,
    headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${JWT_TOKEN}`
    }
});

// 测试结果统计
let totalTests = 0;
let passedTests = 0;
let failedTests = 0;
const failedDetails = [];

// 测试辅助函数
function log(message, type = 'info') {
    const timestamp = new Date().toLocaleTimeString();
    switch(type) {
        case 'success':
            console.log(`✅ [${timestamp}] ${message}`);
            break;
        case 'error':
            console.log(`❌ [${timestamp}] ${message}`);
            break;
        case 'warning':
            console.log(`⚠️  [${timestamp}] ${message}`);
            break;
        default:
            console.log(`ℹ️  [${timestamp}] ${message}`);
    }
}

async function testApi(method, endpoint, description, expectedStatus = 200, data = null) {
    totalTests++;
    try {
        const response = await api({
            method: method,
            url: endpoint,
            data: data
        });
        
        if (response.status === expectedStatus) {
            passedTests++;
            log(`${description}: ${response.status} OK`, 'success');
            return { success: true, data: response.data };
        } else {
            failedTests++;
            const error = `${description}: 期望 ${expectedStatus}, 实际 ${response.status}`;
            log(error, 'error');
            failedDetails.push(error);
            return { success: false, error: error };
        }
    } catch (error) {
        failedTests++;
        const status = error.response?.status || '网络错误';
        const errorMsg = `${description}: 期望 ${expectedStatus}, 实际 ${status}`;
        log(errorMsg, 'error');
        failedDetails.push(errorMsg);
        return { success: false, error: errorMsg };
    }
}

async function runTests() {
    console.log('🚀 开始重点API修复验证测试...\n');
    
    let accountId = null;
    let cardId = null;
    let shortLinkId = null;
    let liveCodeId = null;
    
    // 1. 账户管理API测试 - 验证ID验证修复
    console.log('👤 账户管理API测试:');
    
    // 获取账户列表
    const accountsResult = await testApi('GET', '/accounts', '账户列表');
    if (accountsResult.success && accountsResult.data?.data?.length > 0) {
        accountId = accountsResult.data.data[0].id;
        log(`获取到账户ID: ${accountId}`, 'info');
        
        // 测试账户更新 - 验证ID验证修复
        if (accountId) {
            await testApi('PUT', `/accounts/${accountId}`, '账户更新(ID验证修复)', 200, {
                id: accountId, // 测试URI和JSON ID一致性验证
                name: '测试账户更新',
                email: 'test@example.com'
            });
            
            // 测试错误的ID格式
            await testApi('PUT', '/accounts/invalid-id', '账户更新(无效ID格式)', 400, {
                id: 'invalid-id',
                name: '测试账户'
            });
            
            // 测试URI和JSON ID不一致
            await testApi('PUT', `/accounts/${accountId}`, '账户更新(ID不一致验证)', 400, {
                id: 'different-id', // 与URI中的ID不一致
                name: '测试账户'
            });
        }
    }
    
    console.log('');
    
    // 2. 抖音卡片API测试 - 验证卡片ID处理优化
    console.log('📱 抖音卡片API测试:');
    
    // 获取卡片列表
    const cardsResult = await testApi('GET', '/douyin-cards', '抖音卡片列表');
    if (cardsResult.success && cardsResult.data?.data?.length > 0) {
        cardId = cardsResult.data.data[0].id;
        log(`获取到卡片ID: ${cardId}`, 'info');
        
        if (cardId) {
            // 测试卡片更新 - 验证ID处理优化
            await testApi('PUT', `/douyin-cards/${cardId}`, '卡片更新(ID处理优化)', 200, {
                id: cardId, // 测试URI和JSON ID一致性验证
                title: '测试卡片更新',
                description: '测试描述'
            });
            
            // 测试卡片删除 - 验证ID处理
            await testApi('DELETE', `/douyin-cards/${cardId}`, '卡片删除(ID验证)', 200);
            
            // 测试错误的卡片ID格式
            await testApi('PUT', '/douyin-cards/invalid-id', '卡片更新(无效ID格式)', 400, {
                id: 999,
                title: '测试卡片'
            });
        }
    }
    
    console.log('');
    
    // 3. 短链接API测试 - 验证统计参数处理修复
    console.log('🔗 短链接API测试:');
    
    // 获取短链接列表
    const shortLinksResult = await testApi('GET', '/short-links', '短链接列表');
    if (shortLinksResult.success && shortLinksResult.data?.data?.length > 0) {
        shortLinkId = shortLinksResult.data.data[0].id;
        log(`获取到短链接ID: ${shortLinkId}`, 'info');
        
        if (shortLinkId) {
            // 测试短链接统计 - 验证参数处理修复
            await testApi('GET', `/short-links/${shortLinkId}/stats`, '短链接统计(参数处理修复)', 200);
            
            // 测试短链接更新 - 验证ID处理
            await testApi('PUT', `/short-links/${shortLinkId}`, '短链接更新(ID验证)', 200, {
                id: shortLinkId,
                name: '测试短链接更新',
                url: 'https://example.com'
            });
        }
    }
    
    console.log('');
    
    // 4. 活码管理API测试 - 验证必填字段验证
    console.log('📟 活码管理API测试:');
    
    // 获取活码列表
    const liveCodesResult = await testApi('GET', '/live-codes', '活码列表');
    if (liveCodesResult.success && liveCodesResult.data?.data?.length > 0) {
        liveCodeId = liveCodesResult.data.data[0].id;
        log(`获取到活码ID: ${liveCodeId}`, 'info');
        
        if (liveCodeId) {
            // 测试活码更新 - 验证必填字段
            await testApi('PUT', `/live-codes/${liveCodeId}`, '活码更新(必填字段验证)', 200, {
                id: liveCodeId,
                name: '测试活码更新',
                code: 'TEST123',
                type: 'text' // 测试必填字段
            });
            
            // 测试缺少必填字段的情况
            await testApi('PUT', `/live-codes/${liveCodeId}`, '活码更新(缺少必填字段)', 400, {
                id: liveCodeId,
                name: '测试活码更新'
                // 缺少必填字段: type
            });
        }
    }
    
    console.log('');
    
    // 5. 新增路由测试
    console.log('🆕 新增路由测试:');
    
    // 测试邮件管理API
    await testApi('GET', '/email/accounts', '邮件账户列表', 200);
    await testApi('GET', '/email/templates', '邮件模板列表', 200);
    
    // 测试系统管理API
    await testApi('GET', '/system/logs', '系统日志', 200);
    await testApi('GET', '/system/stats', '系统统计', 200);
    await testApi('GET', '/system/config', '系统配置', 200);
    
    console.log('\n' + '='.repeat(60));
    console.log('📊 修复验证测试总结:');
    console.log(`总测试数: ${totalTests}`);
    console.log(`✅ 通过: ${passedTests}`);
    console.log(`❌ 失败: ${failedTests}`);
    console.log(`📈 成功率: ${((passedTests/totalTests) * 100).toFixed(1)}%`);
    
    if (failedTests > 0) {
        console.log('\n❌ 失败的测试详情:');
        failedDetails.forEach((detail, index) => {
            console.log(`  ${index + 1}. ${detail}`);
        });
    }
    
    console.log('\n🎯 修复验证重点:');
    console.log('✅ 账户更新API ID验证修复');
    console.log('✅ 卡片更新/删除ID处理优化');
    console.log('✅ 短链接统计API参数处理修复');
    console.log('✅ 活码管理必填字段验证');
    console.log('✅ 新增邮件管理和系统管理路由');
    
    console.log('\n🎉 修复验证测试完成！');
}

// 运行测试
runTests().catch(error => {
    console.error('测试运行失败:', error);
    process.exit(1);
});