#!/usr/bin/env node

/**
 * 消息模拟测试用例
 * 模拟各平台的消息流，测试自动回复系统的消息检测和响应能力
 */

const axios = require('axios');
const fs = require('fs');
const path = require('path');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = path.join(__dirname, 'message-simulation-test.log');

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

// 各平台消息模板
const messageTemplates = {
    douyin: [
        { type: 'greeting', content: '你好，在吗？', keywords: ['你好', '在吗'] },
        { type: 'price', content: '这个多少钱？', keywords: ['多少钱'] },
        { type: 'link', content: '有链接吗？', keywords: ['链接'] },
        { type: 'shipping', content: '包邮吗？', keywords: ['包邮'] },
        { type: 'condition', content: '成色怎么样？', keywords: ['成色'] },
        { type: 'purchase', content: '怎么购买？', keywords: ['购买'] },
        { type: 'random', content: '看起来不错', keywords: [] },
        { type: 'negotiation', content: '能便宜点吗？', keywords: ['便宜'] }
    ],
    kuaishou: [
        { type: 'greeting', content: 'hello，请问这个怎么卖？', keywords: ['hello', '怎么卖'] },
        { type: 'price', content: '价格多少？', keywords: ['价格', '多少'] },
        { type: 'discount', content: '有优惠吗？', keywords: ['优惠'] },
        { type: 'quality', content: '质量怎么样？', keywords: ['质量'] },
        { type: 'delivery', content: '多久发货？', keywords: ['发货'] },
        { type: 'payment', content: '支持什么支付方式？', keywords: ['支付'] },
        { type: 'random', content: '关注了', keywords: [] },
        { type: 'interest', content: '很感兴趣', keywords: [] }
    ],
    xiaohongshu: [
        { type: 'greeting', content: '你好呀～', keywords: ['你好'] },
        { type: 'link', content: '求购买链接！', keywords: ['购买', '链接'] },
        { type: 'recommendation', content: '求推荐类似的产品', keywords: ['推荐'] },
        { type: 'review', content: '好用吗？', keywords: ['好用'] },
        { type: 'authenticity', content: '是正品吗？', keywords: ['正品'] },
        { type: 'usage', content: '怎么用？', keywords: ['怎么'] },
        { type: 'random', content: '种草了', keywords: [] },
        { type: 'share', content: '已收藏', keywords: [] }
    ],
    xianyu: [
        { type: 'greeting', content: '在吗？', keywords: ['在吗'] },
        { type: 'shipping', content: '包邮不？', keywords: ['包邮'] },
        { type: 'condition', content: '几成新？', keywords: ['成新'] },
        { type: 'defects', content: '有瑕疵吗？', keywords: ['瑕疵'] },
        { type: 'negotiation', content: '最低多少？', keywords: ['最低', '多少'] },
        { type: 'trade', content: '支持面交吗？', keywords: ['面交'] },
        { type: 'random', content: '还在吗？', keywords: [] },
        { type: 'urgency', content: '急出吗？', keywords: ['急出'] }
    ]
};

class MessageSimulator {
    constructor() {
        this.token = null;
        this.userId = null;
        this.simulationResults = [];
        this.messageCounter = 0;
    }

    async init() {
        logSection('初始化消息模拟测试');
        
        // 清理日志文件
        if (fs.existsSync(LOG_FILE)) {
            fs.unlinkSync(LOG_FILE);
        }
        
        log('开始消息模拟测试...', 'green');
        log(`API地址: ${API_BASE}`, 'blue');
    }

    async login() {
        logSection('用户登录');
        
        try {
            // 使用默认管理员账户登录
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

    // 生成随机消息
    generateRandomMessage(platform) {
        const templates = messageTemplates[platform];
        const randomTemplate = templates[Math.floor(Math.random() * templates.length)];
        
        // 添加一些变化
        const variations = [
            randomTemplate.content,
            `${randomTemplate.content}～`,
            `${randomTemplate.content}？`,
            `请问${randomTemplate.content}`,
            `${randomTemplate.content}谢谢！`
        ];
        
        const content = variations[Math.floor(Math.random() * variations.length)];
        
        return {
            content: content,
            type: randomTemplate.type,
            keywords: randomTemplate.keywords,
            platform: platform,
            timestamp: new Date().toISOString(),
            messageId: `msg_${++this.messageCounter}_${Date.now()}`
        };
    }

    // 模拟消息流
    async simulateMessageStream(platform, messageCount = 20, interval = 2000) {
        logSection(`${platform}平台消息流模拟`);
        
        const results = [];
        log(`开始模拟 ${messageCount} 条消息，间隔 ${interval}ms`, 'yellow');
        
        for (let i = 0; i < messageCount; i++) {
            const message = this.generateRandomMessage(platform);
            
            try {
                log(`[${i + 1}/${messageCount}] 发送消息: "${message.content}"`, 'cyan');
                
                // 发送消息到自动回复系统
                const response = await axios.post(`${API_BASE}/auto-reply/simulate-message`, {
                    platform: platform,
                    message: message.content,
                    message_id: message.messageId,
                    sender_id: `user_${Math.floor(Math.random() * 1000)}`,
                    timestamp: message.timestamp
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const result = response.data.data;
                
                results.push({
                    message: message,
                    matched: result.matched,
                    reply_content: result.reply_content,
                    reply_sent: result.reply_sent,
                    processing_time: result.processing_time,
                    status: 'success'
                });
                
                if (result.matched) {
                    log(`✓ 匹配成功，回复: "${result.reply_content}" (${result.processing_time}ms)`, 'green');
                } else {
                    log(`✗ 未匹配关键词`, 'yellow');
                }
                
                if (result.reply_sent) {
                    log(`✓ 回复已发送`, 'blue');
                }
                
            } catch (error) {
                results.push({
                    message: message,
                    error: error.message,
                    status: 'failed'
                });
                
                log(`消息处理失败: ${error.message}`, 'red');
            }
            
            // 等待间隔时间
            if (i < messageCount - 1) {
                await this.sleep(interval);
            }
        }
        
        return results;
    }

    // 模拟突发消息高峰
    async simulateMessageBurst(platform, burstSize = 50, burstInterval = 100) {
        logSection(`${platform}平台突发消息高峰模拟`);
        
        const results = [];
        log(`开始模拟突发消息高峰: ${burstSize} 条消息，间隔 ${burstInterval}ms`, 'yellow');
        
        const startTime = Date.now();
        
        for (let i = 0; i < burstSize; i++) {
            const message = this.generateRandomMessage(platform);
            
            try {
                // 并行发送消息
                const promise = axios.post(`${API_BASE}/auto-reply/simulate-message`, {
                    platform: platform,
                    message: message.content,
                    message_id: message.messageId,
                    sender_id: `user_burst_${Math.floor(Math.random() * 1000)}`,
                    timestamp: message.timestamp,
                    burst_mode: true
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                results.push({
                    promise: promise,
                    message: message,
                    index: i
                });
                
                log(`[突发] 发送消息 ${i + 1}/${burstSize}: "${message.content}"`, 'magenta');
                
            } catch (error) {
                results.push({
                    message: message,
                    error: error.message,
                    status: 'failed',
                    index: i
                });
                
                log(`突发消息发送失败: ${error.message}`, 'red');
            }
            
            await this.sleep(burstInterval);
        }
        
        // 等待所有请求完成
        log('等待所有突发消息处理完成...', 'blue');
        const processedResults = [];
        
        for (const item of results) {
            if (item.promise) {
                try {
                    const response = await item.promise;
                    const result = response.data.data;
                    
                    processedResults.push({
                        message: item.message,
                        matched: result.matched,
                        reply_content: result.reply_content,
                        reply_sent: result.reply_sent,
                        processing_time: result.processing_time,
                        status: 'success',
                        index: item.index
                    });
                    
                    if (result.matched) {
                        log(`✓ 突发消息 ${item.index + 1} 处理成功`, 'green');
                    }
                } catch (error) {
                    processedResults.push({
                        message: item.message,
                        error: error.message,
                        status: 'failed',
                        index: item.index
                    });
                    
                    log(`突发消息 ${item.index + 1} 处理失败: ${error.message}`, 'red');
                }
            }
        }
        
        const endTime = Date.now();
        const totalTime = endTime - startTime;
        
        log(`突发消息高峰处理完成，总耗时: ${totalTime}ms`, 'cyan');
        
        return {
            results: processedResults,
            totalTime: totalTime,
            messageCount: burstSize
        };
    }

    // 模拟多平台并发消息
    async simulateMultiPlatformConcurrent(platforms = ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu'], messageCount = 10) {
        logSection('多平台并发消息模拟');
        
        const results = {};
        const promises = [];
        
        log(`开始模拟 ${platforms.length} 个平台并发消息，每平台 ${messageCount} 条`, 'yellow');
        
        for (const platform of platforms) {
            const promise = this.simulateConcurrentMessages(platform, messageCount);
            promises.push({
                platform: platform,
                promise: promise
            });
        }
        
        // 等待所有平台完成
        for (const item of promises) {
            try {
                const result = await item.promise;
                results[item.platform] = result;
                
                log(`${item.platform}平台并发测试完成: ${result.successful}/${result.total}`, 'green');
            } catch (error) {
                results[item.platform] = {
                    error: error.message,
                    status: 'failed'
                };
                
                log(`${item.platform}平台并发测试失败: ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    // 模拟单个平台并发消息
    async simulateConcurrentMessages(platform, messageCount) {
        const results = [];
        let successful = 0;
        
        const promises = [];
        
        for (let i = 0; i < messageCount; i++) {
            const message = this.generateRandomMessage(platform);
            
            const promise = axios.post(`${API_BASE}/auto-reply/simulate-message`, {
                platform: platform,
                message: message.content,
                message_id: message.messageId,
                sender_id: `concurrent_user_${Math.floor(Math.random() * 1000)}`,
                timestamp: message.timestamp,
                concurrent_mode: true
            }, {
                headers: { 'Authorization': `Bearer ${this.token}` }
            }).then(response => {
                const result = response.data.data;
                successful++;
                
                return {
                    message: message,
                    matched: result.matched,
                    reply_content: result.reply_content,
                    reply_sent: result.reply_sent,
                    processing_time: result.processing_time,
                    status: 'success'
                };
            }).catch(error => ({
                message: message,
                error: error.message,
                status: 'failed'
            }));
            
            promises.push(promise);
        }
        
        // 等待所有并发请求完成
        const concurrentResults = await Promise.all(promises);
        results.push(...concurrentResults);
        
        return {
            results: results,
            successful: successful,
            total: messageCount,
            successRate: (successful / messageCount * 100).toFixed(2)
        };
    }

    // 模拟消息模式分析
    async analyzeMessagePatterns(platform, messageCount = 100) {
        logSection(`${platform}平台消息模式分析`);
        
        const patterns = {
            messageTypes: {},
            keywordMatches: {},
            responseTimes: [],
            matchRate: 0,
            averageResponseTime: 0
        };
        
        log(`分析 ${messageCount} 条消息的模式...`, 'yellow');
        
        for (let i = 0; i < messageCount; i++) {
            const message = this.generateRandomMessage(platform);
            
            try {
                const startTime = Date.now();
                
                const response = await axios.post(`${API_BASE}/auto-reply/analyze-message`, {
                    platform: platform,
                    message: message.content,
                    message_id: message.messageId,
                    timestamp: message.timestamp
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const endTime = Date.now();
                const processingTime = endTime - startTime;
                
                const result = response.data.data;
                
                // 统计消息类型
                patterns.messageTypes[message.type] = (patterns.messageTypes[message.type] || 0) + 1;
                
                // 统计关键词匹配
                if (result.matched_keywords) {
                    result.matched_keywords.forEach(keyword => {
                        patterns.keywordMatches[keyword] = (patterns.keywordMatches[keyword] || 0) + 1;
                    });
                }
                
                patterns.responseTimes.push(processingTime);
                
                if (i % 10 === 0) {
                    log(`已分析 ${i + 1}/${messageCount} 条消息`, 'cyan');
                }
                
            } catch (error) {
                log(`消息分析失败: ${error.message}`, 'red');
            }
            
            // 短暂延迟避免过载
            await this.sleep(100);
        }
        
        // 计算统计信息
        const totalMessages = Object.values(patterns.messageTypes).reduce((a, b) => a + b, 0);
        const matchedMessages = Object.values(patterns.keywordMatches).reduce((a, b) => a + b, 0);
        
        patterns.matchRate = ((matchedMessages / totalMessages) * 100).toFixed(2);
        patterns.averageResponseTime = (patterns.responseTimes.reduce((a, b) => a + b, 0) / patterns.responseTimes.length).toFixed(2);
        
        // 输出分析结果
        log('消息模式分析完成:', 'green');
        log(`总消息数: ${totalMessages}`, 'cyan');
        log(`匹配率: ${patterns.matchRate}%`, 'cyan');
        log(`平均响应时间: ${patterns.averageResponseTime}ms`, 'cyan');
        
        log('消息类型分布:', 'blue');
        Object.entries(patterns.messageTypes).forEach(([type, count]) => {
            const percentage = ((count / totalMessages) * 100).toFixed(1);
            log(`  ${type}: ${count} (${percentage}%)`, 'cyan');
        });
        
        log('关键词匹配分布:', 'blue');
        Object.entries(patterns.keywordMatches).forEach(([keyword, count]) => {
            log(`  "${keyword}": ${count} 次`, 'cyan');
        });
        
        return patterns;
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    async generateSimulationReport(allResults) {
        logSection('生成模拟测试报告');
        
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                platforms: Object.keys(allResults),
                totalSimulatedMessages: 0,
                totalMatchedMessages: 0,
                overallMatchRate: 0,
                averageResponseTime: 0
            },
            details: allResults
        };
        
        // 计算总体统计
        let totalMessages = 0;
        let totalMatched = 0;
        let totalResponseTime = 0;
        let responseTimeCount = 0;
        
        Object.values(allResults).forEach(platformResults => {
            if (platformResults.results) {
                platformResults.results.forEach(result => {
                    totalMessages++;
                    if (result.matched) {
                        totalMatched++;
                    }
                    if (result.processing_time) {
                        totalResponseTime += result.processing_time;
                        responseTimeCount++;
                    }
                });
            }
        });
        
        report.summary.totalSimulatedMessages = totalMessages;
        report.summary.totalMatchedMessages = totalMatched;
        report.summary.overallMatchRate = totalMessages > 0 ? ((totalMatched / totalMessages) * 100).toFixed(2) : 0;
        report.summary.averageResponseTime = responseTimeCount > 0 ? (totalResponseTime / responseTimeCount).toFixed(2) : 0;
        
        const reportPath = path.join(__dirname, 'message-simulation-report.json');
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
        
        log(`模拟测试报告已生成: ${reportPath}`, 'green');
        log(`总模拟消息数: ${report.summary.totalSimulatedMessages}`, 'cyan');
        log(`总匹配消息数: ${report.summary.totalMatchedMessages}`, 'cyan');
        log(`总体匹配率: ${report.summary.overallMatchRate}%`, 'cyan');
        log(`平均响应时间: ${report.summary.averageResponseTime}ms`, 'cyan');
        
        return report;
    }

    async runSimulation() {
        await this.init();
        
        // 登录
        const loginSuccess = await this.login();
        if (!loginSuccess) {
            log('登录失败，终止模拟测试', 'red');
            return;
        }
        
        const allResults = {};
        
        // 1. 基础消息流模拟
        logSection('基础消息流模拟测试');
        for (const platform of ['douyin', 'kuaishou', 'xiaohongshu', 'xianyu']) {
            const results = await this.simulateMessageStream(platform, 15, 1500);
            allResults[`${platform}_stream`] = {
                type: 'stream',
                platform: platform,
                results: results
            };
        }
        
        // 2. 突发消息高峰模拟
        logSection('突发消息高峰模拟测试');
        for (const platform of ['douyin', 'kuaishou']) {
            const burstResults = await this.simulateMessageBurst(platform, 30, 200);
            allResults[`${platform}_burst`] = {
                type: 'burst',
                platform: platform,
                ...burstResults
            };
        }
        
        // 3. 多平台并发消息模拟
        logSection('多平台并发消息模拟测试');
        const concurrentResults = await this.simulateMultiPlatformConcurrent(['douyin', 'kuaishou', 'xiaohongshu', 'xianyu'], 8);
        allResults['multi_platform_concurrent'] = {
            type: 'concurrent',
            results: concurrentResults
        };
        
        // 4. 消息模式分析
        logSection('消息模式分析测试');
        for (const platform of ['douyin', 'kuaishou']) {
            const patterns = await this.analyzeMessagePatterns(platform, 50);
            allResults[`${platform}_patterns`] = {
                type: 'pattern_analysis',
                platform: platform,
                patterns: patterns
            };
        }
        
        // 生成综合报告
        await this.generateSimulationReport(allResults);
        
        logSection('消息模拟测试完成');
        log('所有消息模拟测试已执行完成！', 'green');
        log(`详细日志请查看: ${LOG_FILE}`, 'blue');
    }
}

// 运行测试
if (require.main === module) {
    const simulator = new MessageSimulator();
    simulator.runSimulation().catch(error => {
        log(`消息模拟测试执行失败: ${error.message}`, 'red');
        process.exit(1);
    });
}

module.exports = MessageSimulator;