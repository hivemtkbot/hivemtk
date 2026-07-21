#!/usr/bin/env node

/**
 * 速率限制测试
 * 测试自动回复系统的速率限制功能，包括每日限制和间隔时间限制
 */

const axios = require('axios');
const fs = require('fs');
const path = require('path');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = path.join(__dirname, 'rate-limiting-test.log');

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

// 速率限制测试配置
const rateLimitConfig = {
    dailyLimit: 100,
    intervalSeconds: 5,
    burstThreshold: 10,
    testMessages: [
        '你好，请问这个怎么卖？',
        '价格是多少？',
        '包邮吗？',
        '有链接吗？',
        '成色怎么样？'
    ]
};

class RateLimitingTester {
    constructor() {
        this.token = null;
        this.userId = null;
        this.testResults = [];
        this.rateLimitStats = {
            totalTests: 0,
            allowedRequests: 0,
            blockedRequests: 0,
            dailyLimitHits: 0,
            intervalLimitHits: 0
        };
    }

    async init() {
        logSection('初始化速率限制测试');
        
        // 清理日志文件
        if (fs.existsSync(LOG_FILE)) {
            fs.unlinkSync(LOG_FILE);
        }
        
        log('开始速率限制功能测试...', 'green');
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
                platforms: ['douyin'],
                rate_limit_test: true
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('自动回复服务启动成功', 'green');
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

    // 测试间隔时间限制
    async testIntervalLimiting() {
        logSection('间隔时间限制测试');
        
        const results = [];
        const message = rateLimitConfig.testMessages[0];
        const interval = rateLimitConfig.intervalSeconds * 1000; // 转换为毫秒
        
        log(`测试间隔时间限制: ${rateLimitConfig.intervalSeconds}秒`, 'yellow');
        log(`消息: "${message}"`, 'cyan');
        
        for (let i = 0; i < 5; i++) {
            const testStartTime = Date.now();
            
            try {
                log(`第 ${i + 1} 次请求...`, 'blue');
                
                const response = await axios.post(`${API_BASE}/auto-reply/test-rate-limit`, {
                    platform: 'douyin',
                    message: message,
                    message_id: `interval_test_${i}_${Date.now()}`,
                    sender_id: `interval_user_${i}`,
                    test_type: 'interval'
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const result = response.data.data;
                const allowed = result.allowed;
                const reason = result.reason;
                const remainingTime = result.remaining_time || 0;
                
                results.push({
                    test: 'interval_limiting',
                    iteration: i + 1,
                    allowed: allowed,
                    reason: reason,
                    remainingTime: remainingTime,
                    timestamp: testStartTime,
                    status: 'success'
                });
                
                if (allowed) {
                    log(`✓ 第 ${i + 1} 次请求: 允许 (${reason})`, 'green');
                } else {
                    log(`✗ 第 ${i + 1} 次请求: 拒绝 (${reason})`, 'yellow');
                    if (remainingTime > 0) {
                        log(`  剩余等待时间: ${remainingTime}秒`, 'cyan');
                    }
                }
                
                // 如果不是最后一次，等待一段时间
                if (i < 4) {
                    const waitTime = i === 0 ? 1000 : interval; // 第一次等待1秒，后续等待完整间隔
                    log(`等待 ${waitTime}ms...`, 'blue');
                    await this.sleep(waitTime);
                }
                
            } catch (error) {
                results.push({
                    test: 'interval_limiting',
                    iteration: i + 1,
                    error: error.message,
                    status: 'failed'
                });
                
                log(`✗ 第 ${i + 1} 次请求: 失败 - ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    // 测试每日限制
    async testDailyLimiting() {
        logSection('每日限制测试');
        
        const results = [];
        const message = rateLimitConfig.testMessages[1];
        const dailyLimit = rateLimitConfig.dailyLimit;
        
        log(`测试每日限制: ${dailyLimit}条消息`, 'yellow');
        log(`消息: "${message}"`, 'cyan');
        
        // 首先重置每日计数（测试目的）
        try {
            await axios.post(`${API_BASE}/auto-reply/reset-daily-limit`, {
                platform: 'douyin',
                test_mode: true
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('已重置每日计数', 'green');
        } catch (error) {
            log(`重置每日计数失败: ${error.message}`, 'yellow');
        }
        
        // 测试接近限制的情况
        const testCount = Math.min(dailyLimit + 5, 15); // 最多测试15条，避免测试时间过长
        
        for (let i = 0; i < testCount; i++) {
            const testStartTime = Date.now();
            
            try {
                log(`第 ${i + 1} 次请求 (限制: ${dailyLimit})...`, 'blue');
                
                const response = await axios.post(`${API_BASE}/auto-reply/test-rate-limit`, {
                    platform: 'douyin',
                    message: message,
                    message_id: `daily_test_${i}_${Date.now()}`,
                    sender_id: `daily_user_${i}`,
                    test_type: 'daily'
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const result = response.data.data;
                const allowed = result.allowed;
                const reason = result.reason;
                const remainingCount = result.remaining_count || 0;
                const usedCount = result.used_count || 0;
                
                results.push({
                    test: 'daily_limiting',
                    iteration: i + 1,
                    allowed: allowed,
                    reason: reason,
                    remainingCount: remainingCount,
                    usedCount: usedCount,
                    timestamp: testStartTime,
                    status: 'success'
                });
                
                if (allowed) {
                    log(`✓ 第 ${i + 1} 次请求: 允许 (${reason}) - 剩余: ${remainingCount}`, 'green');
                } else {
                    log(`✗ 第 ${i + 1} 次请求: 拒绝 (${reason}) - 已使用: ${usedCount}/${dailyLimit}`, 'yellow');
                    this.rateLimitStats.dailyLimitHits++;
                }
                
                // 短暂延迟避免过载
                await this.sleep(200);
                
            } catch (error) {
                results.push({
                    test: 'daily_limiting',
                    iteration: i + 1,
                    error: error.message,
                    status: 'failed'
                });
                
                log(`✗ 第 ${i + 1} 次请求: 失败 - ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    // 测试突发流量限制
    async testBurstLimiting() {
        logSection('突发流量限制测试');
        
        const results = [];
        const burstSize = rateLimitConfig.burstThreshold;
        const message = rateLimitConfig.testMessages[2];
        
        log(`测试突发流量限制: ${burstSize}条消息快速发送`, 'yellow');
        log(`消息: "${message}"`, 'cyan');
        
        // 快速发送突发消息
        const burstPromises = [];
        
        for (let i = 0; i < burstSize; i++) {
            const promise = this.sendBurstMessage(i, message);
            burstPromises.push(promise);
        }
        
        // 并行执行所有突发请求
        const burstResults = await Promise.allSettled(burstPromises);
        
        for (let i = 0; i < burstResults.length; i++) {
            const result = burstResults[i];
            
            if (result.status === 'fulfilled') {
                results.push(result.value);
                
                if (result.value.allowed) {
                    log(`✓ 突发消息 ${i + 1}: 允许`, 'green');
                } else {
                    log(`✗ 突发消息 ${i + 1}: 拒绝 (${result.value.reason})`, 'yellow');
                    this.rateLimitStats.intervalLimitHits++;
                }
            } else {
                results.push({
                    test: 'burst_limiting',
                    iteration: i + 1,
                    error: result.reason.message,
                    status: 'failed'
                });
                
                log(`✗ 突发消息 ${i + 1}: 请求失败 - ${result.reason.message}`, 'red');
            }
        }
        
        // 等待一段时间后再次测试
        log(`等待 ${rateLimitConfig.intervalSeconds * 2}秒后再次测试...`, 'blue');
        await this.sleep(rateLimitConfig.intervalSeconds * 2 * 1000);
        
        // 再次发送一条消息，应该可以通过
        try {
            const response = await axios.post(`${API_BASE}/auto-reply/test-rate-limit`, {
                platform: 'douyin',
                message: message,
                message_id: `burst_recovery_${Date.now()}`,
                sender_id: 'burst_recovery_user',
                test_type: 'burst_recovery'
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const result = response.data.data;
            
            results.push({
                test: 'burst_recovery',
                allowed: result.allowed,
                reason: result.reason,
                status: 'success'
            });
            
            if (result.allowed) {
                log('✓ 恢复测试: 允许 - 速率限制已恢复', 'green');
            } else {
                log('✗ 恢复测试: 仍然被拒绝', 'yellow');
            }
            
        } catch (error) {
            results.push({
                test: 'burst_recovery',
                error: error.message,
                status: 'failed'
            });
            
            log(`✗ 恢复测试: 失败 - ${error.message}`, 'red');
        }
        
        return results;
    }

    // 发送突发消息
    async sendBurstMessage(index, message) {
        try {
            const response = await axios.post(`${API_BASE}/auto-reply/test-rate-limit`, {
                platform: 'douyin',
                message: message,
                message_id: `burst_${index}_${Date.now()}`,
                sender_id: `burst_user_${index}`,
                test_type: 'burst',
                burst_index: index
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const result = response.data.data;
            
            return {
                test: 'burst_limiting',
                iteration: index + 1,
                allowed: result.allowed,
                reason: result.reason,
                timestamp: Date.now(),
                status: 'success'
            };
            
        } catch (error) {
            return {
                test: 'burst_limiting',
                iteration: index + 1,
                error: error.message,
                status: 'failed'
            };
        }
    }

    // 测试多平台速率限制
    async testMultiPlatformRateLimiting() {
        logSection('多平台速率限制测试');
        
        const platforms = ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu'];
        const results = [];
        
        log('测试多平台速率限制', 'yellow');
        
        for (let i = 0; i < platforms.length; i++) {
            const platform = platforms[i];
            const message = rateLimitConfig.testMessages[i % rateLimitConfig.testMessages.length];
            
            try {
                log(`测试平台 ${platform}...`, 'blue');
                
                const response = await axios.post(`${API_BASE}/auto-reply/test-rate-limit`, {
                    platform: platform,
                    message: message,
                    message_id: `multi_platform_${platform}_${Date.now()}`,
                    sender_id: `multi_platform_user_${i}`,
                    test_type: 'multi_platform'
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const result = response.data.data;
                
                results.push({
                    test: 'multi_platform_rate_limiting',
                    platform: platform,
                    allowed: result.allowed,
                    reason: result.reason,
                    timestamp: Date.now(),
                    status: 'success'
                });
                
                if (result.allowed) {
                    log(`✓ ${platform}: 允许`, 'green');
                } else {
                    log(`✗ ${platform}: 拒绝 (${result.reason})`, 'yellow');
                }
                
                // 短暂延迟
                await this.sleep(500);
                
            } catch (error) {
                results.push({
                    test: 'multi_platform_rate_limiting',
                    platform: platform,
                    error: error.message,
                    status: 'failed'
                });
                
                log(`✗ ${platform}: 失败 - ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    // 测试速率限制统计
    async testRateLimitStatistics() {
        logSection('速率限制统计测试');
        
        try {
            const response = await axios.get(`${API_BASE}/auto-reply/rate-limit-stats`, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const stats = response.data.data;
            
            log('速率限制统计:', 'cyan');
            log(`今日已使用: ${stats.used_today || 0}`, 'cyan');
            log(`今日剩余: ${stats.remaining_today || 0}`, 'cyan');
            log(`下次可用时间: ${stats.next_available_time || '无'}`, 'cyan');
            log(`限制重置时间: ${stats.reset_time || '无'}`, 'cyan');
            
            return {
                test: 'rate_limit_statistics',
                stats: stats,
                status: 'success'
            };
            
        } catch (error) {
            log(`获取速率限制统计失败: ${error.message}`, 'red');
            return {
                test: 'rate_limit_statistics',
                error: error.message,
                status: 'failed'
            };
        }
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // 生成速率限制测试报告
    async generateRateLimitReport(allResults) {
        logSection('生成速率限制测试报告');
        
        // 计算统计信息
        let totalTests = 0;
        let allowedRequests = 0;
        let blockedRequests = 0;
        
        Object.values(allResults).forEach(result => {
            if (Array.isArray(result)) {
                result.forEach(test => {
                    if (test.status === 'success' && test.allowed !== undefined) {
                        totalTests++;
                        if (test.allowed) {
                            allowedRequests++;
                        } else {
                            blockedRequests++;
                        }
                    }
                });
            }
        });
        
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalTests: totalTests,
                allowedRequests: allowedRequests,
                blockedRequests: blockedRequests,
                dailyLimitHits: this.rateLimitStats.dailyLimitHits,
                intervalLimitHits: this.rateLimitStats.intervalLimitHits,
                blockRate: totalTests > 0 ? ((blockedRequests / totalTests) * 100).toFixed(2) : 0
            },
            config: rateLimitConfig,
            results: allResults
        };
        
        const reportPath = path.join(__dirname, 'rate-limiting-test-report.json');
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
        
        log(`速率限制测试报告已生成: ${reportPath}`, 'green');
        log(`总测试数: ${report.summary.totalTests}`, 'cyan');
        log(`允许请求: ${report.summary.allowedRequests}`, 'green');
        log(`阻止请求: ${report.summary.blockedRequests}`, 'yellow');
        log(`阻止率: ${report.summary.blockRate}%`, 'cyan');
        log(`每日限制触发: ${report.summary.dailyLimitHits}次`, 'cyan');
        log(`间隔限制触发: ${report.summary.intervalLimitHits}次`, 'cyan');
        
        return report;
    }

    async runRateLimitTests() {
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
        await this.sleep(3000);
        
        const allResults = {};
        
        // 1. 间隔时间限制测试
        const intervalResults = await this.testIntervalLimiting();
        allResults.interval_limiting = intervalResults;
        
        // 2. 每日限制测试
        const dailyResults = await this.testDailyLimiting();
        allResults.daily_limiting = dailyResults;
        
        // 3. 突发流量限制测试
        const burstResults = await this.testBurstLimiting();
        allResults.burst_limiting = burstResults;
        
        // 4. 多平台速率限制测试
        const multiPlatformResults = await this.testMultiPlatformRateLimiting();
        allResults.multi_platform_rate_limiting = multiPlatformResults;
        
        // 5. 速率限制统计测试
        const statisticsResults = await this.testRateLimitStatistics();
        allResults.rate_limit_statistics = statisticsResults;
        
        // 生成测试报告
        await this.generateRateLimitReport(allResults);
        
        logSection('速率限制测试完成');
        log('所有速率限制测试已执行完成！', 'green');
        log(`详细日志请查看: ${LOG_FILE}`, 'blue');
    }
}

// 运行测试
if (require.main === module) {
    const tester = new RateLimitingTester();
    tester.runRateLimitTests().catch(error => {
        log(`速率限制测试执行失败: ${error.message}`, 'red');
        process.exit(1);
    });
}

module.exports = RateLimitingTester;