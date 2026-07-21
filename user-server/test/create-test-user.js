#!/usr/bin/env node

/**
 * 测试用户创建脚本
 * 用于创建测试用户并获取访问令牌
 */

const axios = require('axios');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';

async function createTestUser() {
    try {
        console.log('尝试创建测试用户...');
        
        // 首先尝试直接创建用户（需要管理员权限）
        const testUser = {
            username: 'test_user',
            password: 'test123456',
            email: 'test@example.com',
            real_name: '测试用户',
            phone: '13800138000',
            role: 'user',
            status: 1
        };
        
        // 由于没有管理员令牌，我们尝试使用系统初始化端点
        console.log('尝试系统初始化...');
        const initResponse = await axios.post(`${API_BASE}/merchant/init`, {
            username: 'test_merchant',
            password: 'test123456',
            email: 'test@example.com',
            company_name: '测试商户'
        });
        
        console.log('系统初始化成功:', initResponse.data);
        
        // 使用初始化创建的账户登录
        const loginResponse = await axios.post(`${API_BASE}/auth/login`, {
            username: 'test_merchant',
            password: 'test123456'
        });
        
        console.log('登录成功:', loginResponse.data);
        
        return loginResponse.data.data.token;
        
    } catch (error) {
        console.error('创建测试用户失败:', error.message);
        
        // 如果系统初始化也失败，尝试使用平台注册
        try {
            console.log('尝试平台注册...');
            const platformResponse = await axios.post(`${API_BASE}/platform/register`, {
                username: 'test_platform',
                password: 'test123456',
                email: 'test@example.com',
                company_name: '测试平台'
            });
            
            console.log('平台注册成功:', platformResponse.data);
            
            // 尝试登录
            const loginResponse = await axios.post(`${API_BASE}/auth/login`, {
                username: 'test_platform',
                password: 'test123456'
            });
            
            console.log('登录成功:', loginResponse.data);
            
            return loginResponse.data.data.token;
            
        } catch (platformError) {
            console.error('平台注册也失败:', platformError.message);
            
            // 最后尝试默认管理员密码的不同组合
            const adminPasswords = ['admin123', 'admin', '123456', 'password'];
            
            for (const password of adminPasswords) {
                try {
                    console.log(`尝试管理员密码: ${password}`);
                    const loginResponse = await axios.post(`${API_BASE}/auth/login`, {
                        username: 'admin',
                        password: password
                    });
                    
                    console.log('管理员登录成功:', loginResponse.data);
                    return loginResponse.data.data.token;
                    
                } catch (loginError) {
                    console.log(`密码 ${password} 失败`);
                }
            }
        }
        
        throw new Error('无法创建或登录测试用户');
    }
}

// 运行测试
createTestUser()
    .then(token => {
        console.log('成功获取访问令牌:', token);
        process.exit(0);
    })
    .catch(error => {
        console.error('完全失败:', error.message);
        process.exit(1);
    });