#!/usr/bin/env node

/**
 * 回复执行测试
 * 测试自动回复系统的回复执行功能，包括浏览器自动化和消息发送
 */

const axios = require('axios');
const fs = require('fs');
const path = require('path');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = path.join(__dirname, 'reply-execution-test.log');

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

// 测试用例
const testScenarios = {
    basicReply: [
        {
            platform: 'douyin',
            message: '你好，请问这个怎么卖？',
            expectedReply: '您好！有什么可以帮助您的吗？',
            keywords: ['你好', '怎么卖'],
            description: '基础回复测试'
        },
        {
            platform: 'kuaishou',
            message: '这个价格多少钱？',
            expectedReply: '您好，价格可以私聊详谈哦～',
            keywords: ['价格', '多少钱'],
            description: '价格询问回复测试'
        },
        {
            platform: 'xiaohongshu',
            message: '有购买链接吗？',
            expectedReply: '私信发您链接哦～',
            keywords: ['购买', '链接'],
            description: '链接请求回复测试'
        },
        {
            platform: 'xianyu',
            message: '包邮吗？成色怎么样？',
            expectedReply: '包邮发货，成色很好，可以详聊～',
            keywords: ['包邮', '成色'],
            description: '多关键词回复测试'
        }
    ],
    noReply: [
        {
            platform: 'douyin',
            message: '这个视频拍得真好',
            expectedReply: null,
            keywords: ['价格', '多少钱'],
            description: '无匹配关键词测试'
        },
        {
            platform: 'kuaishou',
            message: '双击666',
            expectedReply: null,
            keywords: ['购买', '链接'],
            description: '平台无关消息测试'
        }
    ],
    rateLimiting: [
        {
            platform: 'douyin',
            message: '你好',
            expectedReply: '您好！有什么可以帮助您的吗？',
            keywords: ['你好'],
            description: '速率限制测试-第一次',
            sequence: 1
        },
        {
            platform: 'douyin',
            message: '你好',
            expectedReply: null,
            keywords: ['你好'],
            description: '速率限制测试-第二次（间隔太短）',
            sequence: 2,
            delay: 1000 // 1秒间隔，应该触发速率限制
        },
        {
            platform: 'douyin',
            message: '你好',
            expectedReply: '您好！有什么可以帮助您的吗？',
            keywords: ['你好'],
            description: '速率限制测试-第三次（间隔足够）',
            sequence: 3,
            delay: 6000 // 6秒间隔，应该可以通过
        }
    ]
};

class ReplyExecutionTester {
    constructor() {
        this.token = null;
        this.userId = null;
        this.testResults = [];
        this.replyLogs = [];
    }

    async init() {
        logSection('初始化回复执行测试');
        
        // 清理日志文件
        if (fs.existsSync(LOG_FILE)) {
            fs.unlinkSync(LOG_FILE);
        }
        
        log('开始回复执行功能测试...', 'green');
        log(`API地址: ${API_BASE}`, 'blue');
    }

    async login() {
        logSection('用户登录');
        
        try {
            const loginData = {
                username: 'admin',
                password: '123456'
            };
            
            log('使用测试账户登录...', 'yellow');
            const loginRes = await axios.post(`${API_BASE}/auth/login`, loginData);
            this.token = loginRes.data.data.token;
            this.userId = loginRes.data.data.user_id;
            
            log(`登录成功: ${this.userId}`, 'green');
            return true;
        } catch (error) {
            log(`登录失败: ${error.message}`, 'red');
            return false;
        }
    }

    // 启动自动回复服务
    async startAutoReplyService() {
        logSection('启动自动回复服务');
        
        try {
            log('正在启动自动回复服务...', 'yellow');
            
            const response = await axios.post(`${API_BASE}/auto-reply/start`, {
                platforms: ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu'],
                test_mode: true
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('自动回复服务启动成功', 'green');
            log(`启动结果: ${JSON.stringify(response.data.data)}`, 'cyan');
            
            return {
                status: 'success',
                data: response.data.data
            };
        } catch (error) {
            log(`启动自动回复服务失败: ${error.message}`, 'red');
            return {
                status: 'failed',
                error: error.message
            };
        }
    }

    // 停止自动回复服务
    async stopAutoReplyService() {
        logSection('停止自动回复服务');
        
        try {
            log('正在停止自动回复服务...', 'yellow');
            
            const response = await axios.post(`${API_BASE}/auto-reply/stop`, {}, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('自动回复服务停止成功', 'green');
            
            return {
                status: 'success',
                data: response.data.data
            };
        } catch (error) {
            log(`停止自动回复服务失败: ${error.message}`, 'red');
            return {
                status: 'failed',
                error: error.message
            };
        }
    }

    // 获取服务状态
    async getServiceStatus() {
        logSection('获取服务状态');
        
        try {
            const response = await axios.get(`${API_BASE}/auto-reply/status`, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const status = response.data.data;
            
            log(`服务状态: ${status.status}`, 'cyan');
            log(`运行中的机器人: ${status.running_bots_count}`, 'cyan');
            log(`总回复数: ${status.total_replies || 0}`, 'cyan');
            
            return status;
        } catch (error) {
            log(`获取服务状态失败: ${error.message}`, 'red');
            return null;
        }
    }

    // 模拟消息并测试回复
    async simulateMessageAndTestReply(testCase) {
        const { platform, message, expectedReply, keywords, description } = testCase;
        
        log(`测试场景: ${description}`, 'yellow');
        log(`平台: ${platform}, 消息: "${message}"`, 'cyan');
        
        try {
            // 如果有延迟设置，等待指定时间
            if (testCase.delay) {
                log(`等待 ${testCase.delay}ms...`, 'blue');
                await this.sleep(testCase.delay);
            }
            
            // 发送模拟消息
            const simulateResponse = await axios.post(`${API_BASE}/auto-reply/simulate-message`, {
                platform: platform,
                message: message,
                message_id: `test_msg_${Date.now()}`,
                sender_id: `test_user_${Math.floor(Math.random() * 1000)}`,
                test_scenario: description
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const result = simulateResponse.data.data;
            
            // 等待回复处理
            log('等待回复处理...', 'blue');
            await this.sleep(2000);
            
            // 获取回复日志
            const logsResponse = await axios.get(`${API_BASE}/auto-reply/logs`, {
                headers: { 'Authorization': `Bearer ${this.token}` },
                params: {
                    platform: platform,
                    limit: 5,
                    message_content: message
                }
            });
            
            const recentLogs = logsResponse.data.data.logs || [];
            const replyLog = recentLogs.find(log => log.message_content === message);
            
            // 验证结果
            let passed = false;
            let actualReply = null;
            let errorMessage = null;
            
            if (expectedReply === null) {
                // 期望无回复
                if (!replyLog || !replyLog.reply_content) {
                    passed = true;
                    log('✓ 正确无回复', 'green');
                } else {
                    passed = false;
                    actualReply = replyLog.reply_content;
                    errorMessage = `期望无回复，但实际回复: "${actualReply}"`;
                    log(`✗ 期望无回复，但实际回复: "${actualReply}"`, 'red');
                }
            } else {
                // 期望有回复
                if (replyLog && replyLog.reply_content === expectedReply) {
                    passed = true;
                    actualReply = replyLog.reply_content;
                    log(`✓ 回复正确: "${actualReply}"`, 'green');
                } else if (replyLog && replyLog.reply_content) {
                    passed = false;
                    actualReply = replyLog.reply_content;
                    errorMessage = `回复内容不匹配。期望: "${expectedReply}", 实际: "${actualReply}"`;
                    log(`✗ 回复内容不匹配。期望: "${expectedReply}", 实际: "${actualReply}"`, 'red');
                } else {
                    passed = false;
                    errorMessage = '期望有回复，但没有收到回复';
                    log(`✗ 期望有回复，但没有收到回复`, 'red');
                }
            }
            
            const testResult = {
                testCase: testCase,
                passed: passed,
                actualReply: actualReply,
                expectedReply: expectedReply,
                replyLog: replyLog,
                simulationResult: result,
                errorMessage: errorMessage,
                status: 'success'
            };
            
            this.testResults.push(testResult);
            
            if (replyLog) {
                this.replyLogs.push(replyLog);
            }
            
            return testResult;
            
        } catch (error) {
            const testResult = {
                testCase: testCase,
                passed: false,
                error: error.message,
                status: 'failed'
            };
            
            this.testResults.push(testResult);
            log(`✗ 测试失败: ${error.message}`, 'red');
            
            return testResult;
        }
    }

    // 测试基础回复功能
    async testBasicReply() {
        logSection('基础回复功能测试');
        
        const results = [];
        
        for (const testCase of testScenarios.basicReply) {
            const result = await this.simulateMessageAndTestReply(testCase);
            results.push(result);
        }
        
        return results;
    }

    // 测试无回复场景
    async testNoReply() {
        logSection('无回复场景测试');
        
        const results = [];
        
        for (const testCase of testScenarios.noReply) {
            const result = await this.simulateMessageAndTestReply(testCase);
            results.push(result);
        }
        
        return results;
    }

    // 测试速率限制
    async testRateLimiting() {
        logSection('速率限制测试');
        
        const results = [];
        
        for (const testCase of testScenarios.rateLimiting) {
            const result = await this.simulateMessageAndTestReply(testCase);
            results.push(result);
        }
        
        return results;
    }

    // 测试多平台并发回复
    async testMultiPlatformConcurrentReply() {
        logSection('多平台并发回复测试');
        
        const concurrentMessages = [
            { platform: 'douyin', message: '你好', delay: 0 },
            { platform: 'kuaishou', message: '价格多少？', delay: 500 },
            { platform: 'xiaohongshu', message: '有链接吗？', delay: 1000 },
            { platform: 'xianyu', message: '包邮吗？', delay: 1500 }
        ];
        
        const results = [];
        const promises = [];
        
        log('开始多平台并发回复测试...', 'yellow');
        
        for (const msg of concurrentMessages) {
            const promise = new Promise(async (resolve) => {
                await this.sleep(msg.delay);
                
                const testCase = {
                    platform: msg.platform,
                    message: msg.message,
                    expectedReply: this.getExpectedReply(msg.platform, msg.message),
                    keywords: this.getKeywords(msg.message),
                    description: `${msg.platform}并发测试`
                };
                
                const result = await this.simulateMessageAndTestReply(testCase);
                resolve(result);
            });
            
            promises.push(promise);
        }
        
        // 等待所有并发测试完成
        const concurrentResults = await Promise.all(promises);
        results.push(...concurrentResults);
        
        return results;
    }

    // 获取期望回复
    getExpectedReply(platform, message) {
        const replyMap = {
            'douyin': {
                '你好': '您好！有什么可以帮助您的吗？',
                '价格多少？': '您好！有什么可以帮助您的吗？',
                '有链接吗？': '您好！有什么可以帮助您的吗？',
                '包邮吗？': '您好！有什么可以帮助您的吗？'
            },
            'kuaishou': {
                '你好': '您好，价格可以私聊详谈哦～',
                '价格多少？': '您好，价格可以私聊详谈哦～',
                '有链接吗？': '您好，价格可以私聊详谈哦～',
                '包邮吗？': '您好，价格可以私聊详谈哦～'
            },
            'xiaohongshu': {
                '你好': '私信发您链接哦～',
                '价格多少？': '私信发您链接哦～',
                '有链接吗？': '私信发您链接哦～',
                '包邮吗？': '私信发您链接哦～'
            },
            'xianyu': {
                '你好': '包邮发货，成色很好，可以详聊～',
                '价格多少？': '包邮发货，成色很好，可以详聊～',
                '有链接吗？': '包邮发货，成色很好，可以详聊～',
                '包邮吗？': '包邮发货，成色很好，可以详聊～'
            }
        };
        
        return replyMap[platform] && replyMap[platform][message] ? replyMap[platform][message] : '您好！有什么可以帮助您的吗？';
    }

    // 获取关键词
    getKeywords(message) {
        const keywordMap = {
            '你好': ['你好'],
            '价格多少？': ['价格', '多少'],
            '有链接吗？': ['链接'],
            '包邮吗？': ['包邮']
        };
        
        return keywordMap[message] || ['你好'];
    }

    // 测试错误处理
    async testErrorHandling() {
        logSection('错误处理测试');
        
        const errorTestCases = [
            {
                platform: 'invalid_platform',
                message: '你好',
                description: '无效平台测试'
            },
            {
                platform: 'douyin',
                message: '',
                description: '空消息测试'
            },
            {
                platform: 'douyin',
                message: null,
                description: 'null消息测试'
            }
        ];
        
        const results = [];
        
        for (const testCase of errorTestCases) {
            log(`测试错误处理: ${testCase.description}`, 'yellow');
            
            try {
                const response = await axios.post(`${API_BASE}/auto-reply/simulate-message`, {
                    platform: testCase.platform,
                    message: testCase.message,
                    message_id: `error_test_${Date.now()}`,
                    sender_id: 'error_test_user'
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                results.push({
                    testCase: testCase,
                    unexpectedSuccess: true,
                    status: 'failed'
                });
                
                log(`✗ 期望错误但未发生错误`, 'red');
                
            } catch (error) {
                results.push({
                    testCase: testCase,
                    error: error.message,
                    expectedError: true,
                    status: 'success'
                });
                
                log(`✓ 正确处理错误: ${error.message}`, 'green');
            }
        }
        
        return results;
    }

    // 获取统计信息
    async getStatistics() {
        logSection('获取统计信息');
        
        try {
            const response = await axios.get(`${API_BASE}/auto-reply/statistics`, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const stats = response.data.data;
            
            log('自动回复统计信息:', 'cyan');
            log(`总消息数: ${stats.total_messages || 0}`, 'cyan');
            log(`总回复数: ${stats.total_replies || 0}`, 'cyan');
            log(`匹配成功率: ${((stats.match_rate || 0) * 100).toFixed(2)}%`, 'cyan');
            log(`平均响应时间: ${(stats.average_response_time || 0).toFixed(2)}ms`, 'cyan');
            
            return stats;
        } catch (error) {
            log(`获取统计信息失败: ${error.message}`, 'red');
            return null;
        }
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // 生成测试报告
    async generateReport() {
        logSection('生成回复执行测试报告');
        
        const totalTests = this.testResults.length;
        const passedTests = this.testResults.filter(r => r.passed === true).length;
        const failedTests = this.testResults.filter(r => r.passed === false).length;
        const errorTests = this.testResults.filter(r => r.status === 'failed').length;
        
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalTests: totalTests,
                passedTests: passedTests,
                failedTests: failedTests,
                errorTests: errorTests,
                passRate: totalTests > 0 ? ((passedTests / totalTests) * 100).toFixed(2) : 0
            },
            replyLogs: this.replyLogs,
            testResults: this.testResults,
            statistics: await this.getStatistics()
        };
        
        const reportPath = path.join(__dirname, 'reply-execution-test-report.json');
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
        
        log(`回复执行测试报告已生成: ${reportPath}`, 'green');
        log(`总测试数: ${report.summary.totalTests}`, 'cyan');
        log(`通过测试: ${report.summary.passedTests}`, 'green');
        log(`失败测试: ${report.summary.failedTests}`, 'red');
        log(`错误测试: ${report.summary.errorTests}`, 'red');
        log(`通过率: ${report.summary.passRate}%`, 'cyan');
        
        return report;
    }

    async runAllTests() {
        await this.init();
        
        // 登录
        const loginSuccess = await this.login();
        if (!loginSuccess) {
            log('登录失败，终止测试', 'red');
            return;
        }
        
        // 启动自动回复服务
        const startResult = await this.startAutoReplyService();
        if (startResult.status !== 'success') {
            log('无法启动自动回复服务，终止测试', 'red');
            return;
        }
        
        // 等待服务完全启动
        log('等待服务完全启动...', 'blue');
        await this.sleep(5000);
        
        // 获取初始状态
        await this.getServiceStatus();
        
        // 运行各种测试
        const basicResults = await this.testBasicReply();
        const noReplyResults = await this.testNoReply();
        const rateLimitingResults = await this.testRateLimiting();
        const concurrentResults = await this.testMultiPlatformConcurrentReply();
        const errorHandlingResults = await this.testErrorHandling();
        
        // 停止服务
        await this.stopAutoReplyService();
        
        // 生成测试报告
        await this.generateReport();
        
        logSection('回复执行测试完成');
        log('所有回复执行测试已执行完成！', 'green');
        log(`详细日志请查看: ${LOG_FILE}`, 'blue');
    }
}

// 运行测试
if (require.main === module) {
    const tester = new ReplyExecutionTester();
    tester.runAllTests().catch(error => {
        log(`回复执行测试失败: ${error.message}`, 'red');
        process.exit(1);
    });
}

module.exports = ReplyExecutionTester;