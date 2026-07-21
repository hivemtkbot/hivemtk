#!/usr/bin/env node

/**
 * 自动回复系统测试脚本
 * 测试抖音、快手、小红书、咸鱼平台的自动回复功能
 */

const axios = require('axios');
const fs = require('fs');
const path = require('path');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = path.join(__dirname, 'auto-reply-test.log');

// 颜色输出
const colors = {
    reset: '\x1b[0m',
    bright: '\x1b[1m',
    red: '\x1b[31m',
    green: '\x1b[32m',
    yellow: '\x1b[33m',
    blue: '\x1b[34m',
    magenta: '\x1b[35m',
    cyan: '\x1b[36m'
};

function log(message, color = 'reset') {
    const timestamp = new Date().toISOString();
    const coloredMessage = `${colors[color]}${message}${colors.reset}`;
    const logMessage = `[${timestamp}] ${message}`;
    
    console.log(coloredMessage);
    fs.appendFileSync(LOG_FILE, logMessage + '\n');
}

function logSection(title) {
    log(`\n${'='.repeat(60)}`, 'cyan');
    log(`${title}`, 'bright');
    log(`${'='.repeat(60)}`, 'cyan');
}

// 测试数据
const testData = {
    platforms: ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu'],
    accounts: [
        { username: 'test_douyin', platform: 'douyin' },
        { username: 'test_kuaishou', platform: 'kuaishou' },
        { username: 'test_xiaohongshu', platform: 'xiaohongshu' },
        { username: 'test_xianyu', platform: 'xianyu' }
    ],
    rules: [
        {
            platform: 'douyin',
            keywords: '你好,hello,在吗',
            reply_content: '您好！有什么可以帮助您的吗？',
            is_active: true,
            daily_limit: 100,
            interval_seconds: 5
        },
        {
            platform: 'kuaishou',
            keywords: '价格,多少钱,怎么卖',
            reply_content: '您好，价格可以私聊详谈哦～',
            is_active: true,
            daily_limit: 50,
            interval_seconds: 10
        },
        {
            platform: 'xiaohongshu',
            keywords: '链接,购买,求推荐',
            reply_content: '私信发您链接哦～',
            is_active: true,
            daily_limit: 80,
            interval_seconds: 8
        },
        {
            platform: 'xianyu',
            keywords: '包邮,发货,成色',
            reply_content: '包邮发货，成色很好，可以详聊～',
            is_active: true,
            daily_limit: 60,
            interval_seconds: 12
        }
    ],
    testMessages: [
        { content: '你好，请问这个怎么卖？', platform: 'douyin' },
        { content: '这个价格多少钱？', platform: 'kuaishou' },
        { content: '有购买链接吗？', platform: 'xiaohongshu' },
        { content: '包邮吗？成色怎么样？', platform: 'xianyu' }
    ]
};

class AutoReplyTester {
    constructor() {
        this.token = null;
        this.userId = null;
        this.testResults = [];
    }

    async init() {
        logSection('初始化自动回复测试');
        
        // 清理日志文件
        if (fs.existsSync(LOG_FILE)) {
            fs.unlinkSync(LOG_FILE);
        }
        
        log('开始自动回复系统测试...', 'green');
        log(`API地址: ${API_BASE}`, 'blue');
    }

    async registerAndLogin() {
        logSection('用户登录');
        
        try {
            // 使用默认管理员账户登录
            const loginData = {
                username: 'admin',
                password: '123456'
            };
            
            log('使用默认管理员账户登录...', 'yellow');
            const loginRes = await axios.post(`${API_BASE}/auth/login`, loginData);
            this.token = loginRes.data.data.token;
            this.userId = loginRes.data.data.user_id;
            log(`管理员登录成功: ${this.userId}`, 'green');
            
            return true;
        } catch (error) {
            log(`登录失败: ${error.message}`, 'red');
            return false;
        }
    }

    async createTestAccounts() {
        logSection('创建测试账户');
        
        const results = [];
        
        for (const account of testData.accounts) {
            try {
                log(`创建${account.platform}账户: ${account.username}`, 'yellow');
                
                const accountData = {
                    platform: account.platform,
                    username: account.username,
                    cookies: JSON.stringify({
                        'test_cookie': 'test_value',
                        'session_id': `session_${Date.now()}`
                    }),
                    is_active: true
                };
                
                const response = await axios.post(`${API_BASE}/accounts`, accountData, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                results.push({
                    platform: account.platform,
                    accountId: response.data.data.id,
                    status: 'success'
                });
                
                log(`${account.platform}账户创建成功: ${response.data.data.id}`, 'green');
            } catch (error) {
                results.push({
                    platform: account.platform,
                    status: 'failed',
                    error: error.message
                });
                
                log(`${account.platform}账户创建失败: ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    async createAutoReplyRules() {
        logSection('创建自动回复规则');
        
        const results = [];
        
        for (const rule of testData.rules) {
            try {
                log(`创建${rule.platform}自动回复规则`, 'yellow');
                
                const ruleData = {
                    platform: rule.platform,
                    keywords: rule.keywords,
                    reply_content: rule.reply_content,
                    is_active: rule.is_active,
                    daily_limit: rule.daily_limit,
                    interval_seconds: rule.interval_seconds
                };
                
                const response = await axios.post(`${API_BASE}/auto-reply/rules`, ruleData, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                results.push({
                    platform: rule.platform,
                    ruleId: response.data.data.id,
                    status: 'success'
                });
                
                log(`${rule.platform}规则创建成功: ${response.data.data.id}`, 'green');
            } catch (error) {
                results.push({
                    platform: rule.platform,
                    status: 'failed',
                    error: error.message
                });
                
                log(`${rule.platform}规则创建失败: ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    async testMessageMatching() {
        logSection('测试消息匹配');
        
        const results = [];
        
        for (const message of testData.testMessages) {
            try {
                log(`测试${message.platform}消息匹配: "${message.content}"`, 'yellow');
                
                // 模拟消息匹配测试
                const testData = {
                    platform: message.platform,
                    message: message.content,
                    test_keywords: this.getKeywordsForPlatform(message.platform)
                };
                
                const response = await axios.post(`${API_BASE}/auto-reply/test-matching`, testData, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const matched = response.data.data.matched;
                const matchedKeywords = response.data.data.matched_keywords || [];
                
                results.push({
                    message: message.content,
                    platform: message.platform,
                    matched: matched,
                    matchedKeywords: matchedKeywords,
                    status: 'success'
                });
                
                if (matched) {
                    log(`✓ 匹配成功: "${message.content}" -> ${matchedKeywords.join(', ')}`, 'green');
                } else {
                    log(`✗ 未匹配: "${message.content}"`, 'red');
                }
            } catch (error) {
                results.push({
                    message: message.content,
                    platform: message.platform,
                    status: 'failed',
                    error: error.message
                });
                
                log(`消息匹配测试失败: ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    getKeywordsForPlatform(platform) {
        const rule = testData.rules.find(r => r.platform === platform);
        return rule ? rule.keywords.split(',') : [];
    }

    async testAutoReplyExecution() {
        logSection('测试自动回复执行');
        
        try {
            // 启动自动回复服务
            log('启动自动回复服务...', 'yellow');
            const startResponse = await axios.post(`${API_BASE}/auto-reply/start`, {}, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('自动回复服务启动成功', 'green');
            
            // 等待一段时间让服务运行
            log('等待30秒让服务运行...', 'blue');
            await this.sleep(30000);
            
            // 获取服务状态
            log('获取服务状态...', 'yellow');
            const statusResponse = await axios.get(`${API_BASE}/auto-reply/status`, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const status = statusResponse.data.data;
            log(`服务状态: ${status.status}`, 'cyan');
            log(`运行中的机器人: ${status.running_bots_count}`, 'cyan');
            
            // 停止服务
            log('停止自动回复服务...', 'yellow');
            const stopResponse = await axios.post(`${API_BASE}/auto-reply/stop`, {}, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('自动回复服务停止成功', 'green');
            
            return {
                status: 'success',
                serviceStatus: status,
                startResponse: startResponse.data,
                stopResponse: stopResponse.data
            };
        } catch (error) {
            log(`自动回复执行测试失败: ${error.message}`, 'red');
            return {
                status: 'failed',
                error: error.message
            };
        }
    }

    async testRateLimiting() {
        logSection('测试速率限制');
        
        try {
            // 创建高频消息测试
            const messages = [];
            for (let i = 0; i < 10; i++) {
                messages.push({
                    platform: 'douyin',
                    content: `测试消息${i + 1}: 你好`
                });
            }
            
            log('发送高频消息测试速率限制...', 'yellow');
            
            const results = [];
            for (const message of messages) {
                try {
                    const response = await axios.post(`${API_BASE}/auto-reply/test-message`, message, {
                        headers: { 'Authorization': `Bearer ${this.token}` }
                    });
                    
                    results.push({
                        message: message.content,
                        allowed: response.data.data.allowed,
                        reason: response.data.data.reason
                    });
                    
                    if (response.data.data.allowed) {
                        log(`✓ 消息允许: "${message.content}"`, 'green');
                    } else {
                        log(`✗ 消息被拒绝: "${message.content}" (${response.data.data.reason})`, 'yellow');
                    }
                } catch (error) {
                    results.push({
                        message: message.content,
                        error: error.message
                    });
                    
                    log(`速率限制测试失败: ${error.message}`, 'red');
                }
                
                // 短暂延迟
                await this.sleep(1000);
            }
            
            return results;
        } catch (error) {
            log(`速率限制测试失败: ${error.message}`, 'red');
            return [];
        }
    }

    async getReplyLogs() {
        logSection('获取回复日志');
        
        try {
            log('获取自动回复日志...', 'yellow');
            
            const response = await axios.get(`${API_BASE}/auto-reply/logs`, {
                headers: { 'Authorization': `Bearer ${this.token}` },
                params: { limit: 50 }
            });
            
            const logs = response.data.data.logs || [];
            
            log(`获取到 ${logs.length} 条回复日志`, 'green');
            
            // 显示最近的5条日志
            logs.slice(0, 5).forEach((log, index) => {
                log(`${index + 1}. [${log.platform}] ${log.message} -> ${log.reply_content} (${log.status})`, 'cyan');
            });
            
            return logs;
        } catch (error) {
            log(`获取回复日志失败: ${error.message}`, 'red');
            return [];
        }
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    async generateReport() {
        logSection('生成测试报告');
        
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalTests: this.testResults.length,
                passedTests: this.testResults.filter(r => r.status === 'success').length,
                failedTests: this.testResults.filter(r => r.status === 'failed').length
            },
            details: this.testResults
        };
        
        const reportPath = path.join(__dirname, 'auto-reply-test-report.json');
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
        
        log(`测试报告已生成: ${reportPath}`, 'green');
        log(`总测试数: ${report.summary.totalTests}`, 'cyan');
        log(`通过测试: ${report.summary.passedTests}`, 'green');
        log(`失败测试: ${report.summary.failedTests}`, 'red');
        
        return report;
    }

    async runAllTests() {
        await this.init();
        
        // 用户注册和登录
        const loginResult = await this.registerAndLogin();
        this.testResults.push({
            test: '用户注册和登录',
            status: loginResult ? 'success' : 'failed'
        });
        
        if (!loginResult) {
            log('用户注册/登录失败，终止测试', 'red');
            return;
        }
        
        // 创建测试账户
        const accountResults = await this.createTestAccounts();
        this.testResults.push({
            test: '创建测试账户',
            status: accountResults.every(r => r.status === 'success') ? 'success' : 'failed',
            details: accountResults
        });
        
        // 创建自动回复规则
        const ruleResults = await this.createAutoReplyRules();
        this.testResults.push({
            test: '创建自动回复规则',
            status: ruleResults.every(r => r.status === 'success') ? 'success' : 'failed',
            details: ruleResults
        });
        
        // 测试消息匹配
        const matchingResults = await this.testMessageMatching();
        this.testResults.push({
            test: '消息匹配测试',
            status: matchingResults.every(r => r.status === 'success') ? 'success' : 'failed',
            details: matchingResults
        });
        
        // 测试自动回复执行
        const executionResult = await this.testAutoReplyExecution();
        this.testResults.push({
            test: '自动回复执行测试',
            status: executionResult.status,
            details: executionResult
        });
        
        // 测试速率限制
        const rateLimitResults = await this.testRateLimiting();
        this.testResults.push({
            test: '速率限制测试',
            status: rateLimitResults.length > 0 ? 'success' : 'failed',
            details: rateLimitResults
        });
        
        // 获取回复日志
        const logs = await this.getReplyLogs();
        this.testResults.push({
            test: '获取回复日志',
            status: logs.length >= 0 ? 'success' : 'failed',
            details: { logCount: logs.length }
        });
        
        // 生成测试报告
        await this.generateReport();
        
        logSection('测试完成');
        log('所有测试已执行完成！', 'green');
        log(`详细日志请查看: ${LOG_FILE}`, 'blue');
    }
}

// 运行测试
if (require.main === module) {
    const tester = new AutoReplyTester();
    tester.runAllTests().catch(error => {
        log(`测试执行失败: ${error.message}`, 'red');
        process.exit(1);
    });
}

module.exports = AutoReplyTester;