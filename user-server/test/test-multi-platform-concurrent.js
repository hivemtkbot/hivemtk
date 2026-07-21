#!/usr/bin/env node

/**
 * 多平台并发测试
 * 测试自动回复系统在多个平台同时运行时的并发处理能力
 */

const axios = require('axios');
const fs = require('fs');
const path = require('path');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = path.join(__dirname, 'multi-platform-concurrent-test.log');

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

// 平台配置
const platformConfigs = {
    douyin: {
        name: '抖音',
        messages: ['你好', '怎么卖', '价格多少', '有链接吗', '包邮吗'],
        keywords: ['你好', '怎么卖', '价格', '链接', '包邮'],
        replyRules: {
            '你好': '您好！有什么可以帮助您的吗？',
            '怎么卖': '您好！有什么可以帮助您的吗？',
            '价格': '您好！有什么可以帮助您的吗？',
            '链接': '您好！有什么可以帮助您的吗？',
            '包邮': '您好！有什么可以帮助您的吗？'
        }
    },
    kuaishou: {
        name: '快手',
        messages: ['老铁价格', '多少钱', '怎么购买', '支持面交吗', '最低多少'],
        keywords: ['价格', '多少钱', '购买', '面交', '最低'],
        replyRules: {
            '价格': '您好，价格可以私聊详谈哦～',
            '多少钱': '您好，价格可以私聊详谈哦～',
            '购买': '您好，价格可以私聊详谈哦～',
            '面交': '您好，价格可以私聊详谈哦～',
            '最低': '您好，价格可以私聊详谈哦～'
        }
    },
    xiaohongshu: {
        name: '小红书',
        messages: ['姐妹求链接', '是正品吗', '种草了', '已收藏', '求推荐'],
        keywords: ['链接', '正品', '种草', '收藏', '推荐'],
        replyRules: {
            '链接': '私信发您链接哦～',
            '正品': '私信发您链接哦～',
            '种草': '私信发您链接哦～',
            '收藏': '私信发您链接哦～',
            '推荐': '私信发您链接哦～'
        }
    },
    xianyu: {
        name: '咸鱼',
        messages: ['包邮吗', '成色如何', '几成新', '有瑕疵吗', '急出吗'],
        keywords: ['包邮', '成色', '成新', '瑕疵', '急出'],
        replyRules: {
            '包邮': '包邮发货，成色很好，可以详聊～',
            '成色': '包邮发货，成色很好，可以详聊～',
            '成新': '包邮发货，成色很好，可以详聊～',
            '瑕疵': '包邮发货，成色很好，可以详聊～',
            '急出': '包邮发货，成色很好，可以详聊～'
        }
    }
};

class MultiPlatformConcurrentTester {
    constructor() {
        this.token = null;
        this.userId = null;
        this.testResults = [];
        this.concurrentStats = {
            totalMessages: 0,
            successfulReplies: 0,
            failedReplies: 0,
            averageResponseTime: 0,
            platformStats: {}
        };
    }

    async init() {
        logSection('初始化多平台并发测试');
        
        // 清理日志文件
        if (fs.existsSync(LOG_FILE)) {
            fs.unlinkSync(LOG_FILE);
        }
        
        log('开始多平台并发测试...', 'green');
        log(`API地址: ${API_BASE}`, 'blue');
    }

    async login() {
        logSection('用户登录');
        
        try {
            const loginData = {
                username: 'admin',
                password: '123456'
            };
            
            log('使用默认管理员账户登录...', 'yellow');
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

    // 启动所有平台的自动回复服务
    async startAllPlatformServices() {
        logSection('启动所有平台自动回复服务');
        
        try {
            log('正在启动多平台自动回复服务...', 'yellow');
            
            const response = await axios.post(`${API_BASE}/auto-reply/start`, {
                platforms: ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu'],
                concurrent_mode: true,
                max_concurrent_bots: 4
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('多平台自动回复服务启动成功', 'green');
            log(`启动结果: ${JSON.stringify(response.data.data)}`, 'cyan');
            
            return {
                status: 'success',
                data: response.data.data
            };
        } catch (error) {
            log(`启动多平台服务失败: ${error.message}`, 'red');
            return {
                status: 'failed',
                error: error.message
            };
        }
    }

    // 停止所有平台服务
    async stopAllPlatformServices() {
        logSection('停止所有平台自动回复服务');
        
        try {
            log('正在停止多平台自动回复服务...', 'yellow');
            
            const response = await axios.post(`${API_BASE}/auto-reply/stop`, {
                stop_all: true
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            log('多平台自动回复服务停止成功', 'green');
            
            return {
                status: 'success',
                data: response.data.data
            };
        } catch (error) {
            log(`停止多平台服务失败: ${error.message}`, 'red');
            return {
                status: 'failed',
                error: error.message
            };
        }
    }

    // 获取多平台服务状态
    async getMultiPlatformStatus() {
        logSection('获取多平台服务状态');
        
        try {
            const response = await axios.get(`${API_BASE}/auto-reply/status`, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const status = response.data.data;
            
            log('多平台服务状态:', 'cyan');
            log(`服务状态: ${status.status}`, 'cyan');
            log(`运行中的机器人: ${status.running_bots_count}`, 'cyan');
            log(`平台状态: ${JSON.stringify(status.platform_status || {})}`, 'cyan');
            
            return status;
        } catch (error) {
            log(`获取多平台服务状态失败: ${error.message}`, 'red');
            return null;
        }
    }

    // 单平台并发消息测试
    async testSinglePlatformConcurrent(platform, concurrentCount = 10) {
        logSection(`${platformConfigs[platform].name}平台并发测试`);
        
        const results = [];
        const startTime = Date.now();
        const promises = [];
        
        log(`开始${platformConfigs[platform].name}平台并发测试: ${concurrentCount}个并发消息`, 'yellow');
        
        for (let i = 0; i < concurrentCount; i++) {
            const message = platformConfigs[platform].messages[i % platformConfigs[platform].messages.length];
            const messageId = `${platform}_msg_${i}_${Date.now()}`;
            
            const promise = this.simulatePlatformMessage(platform, message, messageId);
            promises.push(promise);
        }
        
        // 等待所有并发请求完成
        const concurrentResults = await Promise.allSettled(promises);
        
        for (let i = 0; i < concurrentResults.length; i++) {
            const result = concurrentResults[i];
            
            if (result.status === 'fulfilled') {
                results.push(result.value);
                
                if (result.value.replySent) {
                    log(`✓ 消息 ${i + 1}: 回复成功 - "${result.value.replyContent}"`, 'green');
                } else {
                    log(`✗ 消息 ${i + 1}: 回复失败 - ${result.value.error || '无回复'}`, 'red');
                }
            } else {
                results.push({
                    platform: platform,
                    message: platformConfigs[platform].messages[i % platformConfigs[platform].messages.length],
                    error: result.reason.message,
                    status: 'failed'
                });
                
                log(`✗ 消息 ${i + 1}: 请求失败 - ${result.reason.message}`, 'red');
            }
        }
        
        const endTime = Date.now();
        const totalTime = endTime - startTime;
        
        // 统计结果
        const successfulReplies = results.filter(r => r.replySent).length;
        const failedReplies = results.filter(r => !r.replySent && r.status !== 'failed').length;
        const requestFailures = results.filter(r => r.status === 'failed').length;
        
        const stats = {
            platform: platform,
            totalMessages: concurrentCount,
            successfulReplies: successfulReplies,
            failedReplies: failedReplies,
            requestFailures: requestFailures,
            successRate: ((successfulReplies / concurrentCount) * 100).toFixed(2),
            totalTime: totalTime,
            averageResponseTime: totalTime / concurrentCount
        };
        
        this.concurrentStats.platformStats[platform] = stats;
        
        log(`${platformConfigs[platform].name}平台并发测试完成:`, 'cyan');
        log(`总消息数: ${concurrentCount}, 成功回复: ${successfulReplies}, 失败回复: ${failedReplies}`, 'cyan');
        log(`成功率: ${stats.successRate}%, 总耗时: ${totalTime}ms`, 'cyan');
        
        return {
            results: results,
            stats: stats
        };
    }

    // 模拟平台消息
    async simulatePlatformMessage(platform, message, messageId) {
        try {
            const response = await axios.post(`${API_BASE}/auto-reply/simulate-message`, {
                platform: platform,
                message: message,
                message_id: messageId,
                sender_id: `${platform}_user_${Math.floor(Math.random() * 1000)}`,
                timestamp: new Date().toISOString(),
                concurrent_test: true
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const result = response.data.data;
            
            // 等待回复处理
            await this.sleep(2000);
            
            // 获取回复日志
            const logsResponse = await axios.get(`${API_BASE}/auto-reply/logs`, {
                headers: { 'Authorization': `Bearer ${this.token}` },
                params: {
                    platform: platform,
                    message_id: messageId,
                    limit: 1
                }
            });
            
            const logs = logsResponse.data.data.logs || [];
            const replyLog = logs.find(log => log.message_id === messageId);
            
            return {
                platform: platform,
                message: message,
                messageId: messageId,
                replySent: !!replyLog && !!replyLog.reply_content,
                replyContent: replyLog ? replyLog.reply_content : null,
                replyLog: replyLog,
                simulationResult: result,
                status: 'success'
            };
            
        } catch (error) {
            return {
                platform: platform,
                message: message,
                messageId: messageId,
                error: error.message,
                status: 'failed'
            };
        }
    }

    // 多平台同时并发测试
    async testMultiPlatformConcurrent(concurrentPerPlatform = 5) {
        logSection('多平台同时并发测试');
        
        const platforms = Object.keys(platformConfigs);
        const results = {};
        const promises = [];
        
        log(`开始多平台同时并发测试: ${platforms.length}个平台，每平台${concurrentPerPlatform}个并发消息`, 'yellow');
        
        const startTime = Date.now();
        
        for (const platform of platforms) {
            const promise = this.testSinglePlatformConcurrent(platform, concurrentPerPlatform);
            promises.push({
                platform: platform,
                promise: promise
            });
        }
        
        // 等待所有平台测试完成
        for (const item of promises) {
            try {
                const result = await item.promise;
                results[item.platform] = result;
                
                log(`${platformConfigs[item.platform].name}平台测试完成`, 'green');
            } catch (error) {
                results[item.platform] = {
                    error: error.message,
                    status: 'failed'
                };
                
                log(`${platformConfigs[item.platform].name}平台测试失败: ${error.message}`, 'red');
            }
        }
        
        const endTime = Date.now();
        const totalTime = endTime - startTime;
        
        // 计算总体统计
        let totalMessages = 0;
        let totalSuccessful = 0;
        let totalFailed = 0;
        
        Object.values(results).forEach(result => {
            if (result.stats) {
                totalMessages += result.stats.totalMessages;
                totalSuccessful += result.stats.successfulReplies;
                totalFailed += result.stats.failedReplies;
            }
        });
        
        const overallStats = {
            totalPlatforms: platforms.length,
            totalMessages: totalMessages,
            totalSuccessful: totalSuccessful,
            totalFailed: totalFailed,
            overallSuccessRate: totalMessages > 0 ? ((totalSuccessful / totalMessages) * 100).toFixed(2) : 0,
            totalTime: totalTime,
            averageTimePerMessage: totalTime / totalMessages
        };
        
        this.concurrentStats = overallStats;
        
        log('多平台同时并发测试完成:', 'cyan');
        log(`总平台数: ${platforms.length}, 总消息数: ${totalMessages}`, 'cyan');
        log(`成功回复: ${totalSuccessful}, 失败回复: ${totalFailed}`, 'cyan');
        log(`总体成功率: ${overallStats.overallSuccessRate}%, 总耗时: ${totalTime}ms`, 'cyan');
        
        return {
            results: results,
            overallStats: overallStats
        };
    }

    // 混合并发测试（不同平台消息交错发送）
    async testMixedConcurrent(totalMessages = 40) {
        logSection('混合并发测试');
        
        const platforms = Object.keys(platformConfigs);
        const messages = [];
        const results = [];
        
        // 生成混合消息序列
        for (let i = 0; i < totalMessages; i++) {
            const platform = platforms[i % platforms.length];
            const message = platformConfigs[platform].messages[i % platformConfigs[platform].messages.length];
            
            messages.push({
                platform: platform,
                message: message,
                messageId: `mixed_${platform}_${i}_${Date.now()}`,
                sequence: i
            });
        }
        
        log(`开始混合并发测试: ${totalMessages}条消息，交错发送`, 'yellow');
        
        const startTime = Date.now();
        
        // 并行发送所有消息
        const promises = messages.map(msg => 
            this.simulatePlatformMessage(msg.platform, msg.message, msg.messageId)
        );
        
        const concurrentResults = await Promise.allSettled(promises);
        
        for (let i = 0; i < concurrentResults.length; i++) {
            const result = concurrentResults[i];
            const originalMessage = messages[i];
            
            if (result.status === 'fulfilled') {
                results.push({
                    ...result.value,
                    sequence: originalMessage.sequence
                });
            } else {
                results.push({
                    platform: originalMessage.platform,
                    message: originalMessage.message,
                    messageId: originalMessage.messageId,
                    sequence: originalMessage.sequence,
                    error: result.reason.message,
                    status: 'failed'
                });
            }
        }
        
        const endTime = Date.now();
        const totalTime = endTime - startTime;
        
        // 统计结果
        const successfulReplies = results.filter(r => r.replySent).length;
        const failedReplies = results.filter(r => !r.replySent && r.status !== 'failed').length;
        const requestFailures = results.filter(r => r.status === 'failed').length;
        
        // 按平台统计
        const platformStats = {};
        platforms.forEach(platform => {
            const platformResults = results.filter(r => r.platform === platform);
            const platformSuccess = platformResults.filter(r => r.replySent).length;
            
            platformStats[platform] = {
                total: platformResults.length,
                successful: platformSuccess,
                successRate: platformResults.length > 0 ? ((platformSuccess / platformResults.length) * 100).toFixed(2) : 0
            };
        });
        
        const mixedStats = {
            totalMessages: totalMessages,
            successfulReplies: successfulReplies,
            failedReplies: failedReplies,
            requestFailures: requestFailures,
            successRate: ((successfulReplies / totalMessages) * 100).toFixed(2),
            totalTime: totalTime,
            averageTimePerMessage: totalTime / totalMessages,
            platformStats: platformStats
        };
        
        log('混合并发测试完成:', 'cyan');
        log(`总消息数: ${totalMessages}, 成功回复: ${successfulReplies}, 失败回复: ${failedReplies}`, 'cyan');
        log(`成功率: ${mixedStats.successRate}%, 总耗时: ${totalTime}ms`, 'cyan');
        
        // 输出各平台统计
        log('各平台统计:', 'blue');
        Object.entries(platformStats).forEach(([platform, stats]) => {
            log(`  ${platformConfigs[platform].name}: ${stats.successful}/${stats.total} (${stats.successRate}%)`, 'cyan');
        });
        
        return {
            results: results,
            stats: mixedStats
        };
    }

    // 压力测试（大量并发）
    async testStressLoad(messagesPerPlatform = 20) {
        logSection('压力测试');
        
        const platforms = Object.keys(platformConfigs);
        const totalMessages = messagesPerPlatform * platforms.length;
        
        log(`开始压力测试: ${platforms.length}个平台，每平台${messagesPerPlatform}条消息，总计${totalMessages}条消息`, 'yellow');
        
        const startTime = Date.now();
        
        // 使用更大的并发量
        const results = await this.testMultiPlatformConcurrent(messagesPerPlatform);
        
        const endTime = Date.now();
        const totalTime = endTime - startTime;
        
        // 计算压力测试结果
        const stressStats = {
            ...results.overallStats,
            totalTime: totalTime,
            messagesPerSecond: (totalMessages / (totalTime / 1000)).toFixed(2),
            testType: 'stress'
        };
        
        log('压力测试完成:', 'cyan');
        log(`总消息数: ${totalMessages}, 处理时间: ${totalTime}ms`, 'cyan');
        log(`处理速度: ${stressStats.messagesPerSecond} 消息/秒`, 'cyan');
        log(`成功率: ${stressStats.overallSuccessRate}%`, 'cyan');
        
        return {
            results: results,
            stressStats: stressStats
        };
    }

    // 获取并发统计信息
    async getConcurrentStatistics() {
        logSection('获取并发统计信息');
        
        try {
            const response = await axios.get(`${API_BASE}/auto-reply/concurrent-stats`, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            });
            
            const stats = response.data.data;
            
            log('并发统计信息:', 'cyan');
            log(`最大并发数: ${stats.max_concurrent || 0}`, 'cyan');
            log(`平均并发数: ${stats.average_concurrent || 0}`, 'cyan');
            log(`并发峰值时间: ${stats.peak_time || '无'}`, 'cyan');
            log(`总并发消息数: ${stats.total_concurrent_messages || 0}`, 'cyan');
            
            return stats;
        } catch (error) {
            log(`获取并发统计信息失败: ${error.message}`, 'red');
            return null;
        }
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // 生成并发测试报告
    async generateConcurrentReport(allResults) {
        logSection('生成多平台并发测试报告');
        
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalTests: Object.keys(allResults).length,
                platforms: Object.keys(platformConfigs),
                testScenarios: ['单平台并发', '多平台并发', '混合并发', '压力测试']
            },
            concurrentStats: this.concurrentStats,
            platformConfigs: platformConfigs,
            results: allResults,
            concurrentStatistics: await this.getConcurrentStatistics()
        };
        
        const reportPath = path.join(__dirname, 'multi-platform-concurrent-test-report.json');
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
        
        log(`多平台并发测试报告已生成: ${reportPath}`, 'green');
        log(`测试平台数: ${report.summary.platforms.length}`, 'cyan');
        log(`测试场景数: ${report.summary.testScenarios.length}`, 'cyan');
        
        return report;
    }

    async runConcurrentTests() {
        await this.init();
        
        // 登录
        const loginSuccess = await this.login();
        if (!loginSuccess) {
            log('登录失败，终止测试', 'red');
            return;
        }
        
        // 启动多平台服务
        const startResult = await this.startAllPlatformServices();
        if (startResult.status !== 'success') {
            log('无法启动多平台服务，终止测试', 'red');
            return;
        }
        
        // 等待服务完全启动
        log('等待多平台服务完全启动...', 'blue');
        await this.sleep(5000);
        
        // 获取初始状态
        await this.getMultiPlatformStatus();
        
        const allResults = {};
        
        // 1. 单平台并发测试
        logSection('开始单平台并发测试阶段');
        for (const platform of Object.keys(platformConfigs)) {
            const result = await this.testSinglePlatformConcurrent(platform, 10);
            allResults[`${platform}_concurrent`] = result;
            
            // 短暂休息
            await this.sleep(2000);
        }
        
        // 2. 多平台同时并发测试
        logSection('开始多平台同时并发测试阶段');
        const multiPlatformResult = await this.testMultiPlatformConcurrent(8);
        allResults['multi_platform_concurrent'] = multiPlatformResult;
        
        // 3. 混合并发测试
        logSection('开始混合并发测试阶段');
        const mixedResult = await this.testMixedConcurrent(32);
        allResults['mixed_concurrent'] = mixedResult;
        
        // 4. 压力测试
        logSection('开始压力测试阶段');
        const stressResult = await this.testStressLoad(15);
        allResults['stress_test'] = stressResult;
        
        // 停止所有服务
        await this.stopAllPlatformServices();
        
        // 生成测试报告
        await this.generateConcurrentReport(allResults);
        
        logSection('多平台并发测试完成');
        log('所有多平台并发测试已执行完成！', 'green');
        log(`详细日志请查看: ${LOG_FILE}`, 'blue');
    }
}

// 运行测试
if (require.main === module) {
    const tester = new MultiPlatformConcurrentTester();
    tester.runConcurrentTests().catch(error => {
        log(`多平台并发测试执行失败: ${error.message}`, 'red');
        process.exit(1);
    });
}

module.exports = MultiPlatformConcurrentTester;