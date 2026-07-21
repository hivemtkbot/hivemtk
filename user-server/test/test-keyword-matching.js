#!/usr/bin/env node

/**
 * 关键词匹配验证测试
 * 测试自动回复系统的关键词匹配算法准确性和性能
 */

const axios = require('axios');
const fs = require('fs');
const path = require('path');

const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = path.join(__dirname, 'keyword-matching-test.log');

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

// 测试用例数据
const testCases = {
    exactMatch: [
        { message: '你好', keywords: ['你好'], expected: true, description: '完全匹配' },
        { message: 'hello world', keywords: ['hello'], expected: true, description: '英文完全匹配' },
        { message: '价格', keywords: ['价格'], expected: true, description: '中文完全匹配' },
        { message: '包邮', keywords: ['包邮'], expected: true, description: '特定词汇匹配' }
    ],
    partialMatch: [
        { message: '你好啊', keywords: ['你好'], expected: true, description: '前缀匹配' },
        { message: '请问你好', keywords: ['你好'], expected: true, description: '后缀匹配' },
        { message: '请问你好啊', keywords: ['你好'], expected: true, description: '中间匹配' },
        { message: '你好HELLO', keywords: ['hello'], expected: true, description: '大小写不敏感匹配' }
    ],
    multipleKeywords: [
        { message: '你好，价格多少？', keywords: ['你好', '价格'], expected: true, description: '多关键词同时匹配' },
        { message: '这个怎么卖，多少钱？', keywords: ['怎么卖', '多少钱'], expected: true, description: '多关键词混合匹配' },
        { message: '包邮吗？成色怎么样？', keywords: ['包邮', '成色'], expected: true, description: '多关键词组合匹配' }
    ],
    noMatch: [
        { message: '今天天气真好', keywords: ['你好', '价格'], expected: false, description: '无匹配关键词' },
        { message: '完全无关的内容', keywords: ['包邮', '链接'], expected: false, description: '完全无关内容' },
        { message: '123456789', keywords: ['你好'], expected: false, description: '纯数字无匹配' }
    ],
    edgeCases: [
        { message: '', keywords: ['你好'], expected: false, description: '空消息' },
        { message: '你好', keywords: [], expected: false, description: '空关键词' },
        { message: '你好', keywords: [''], expected: false, description: '空字符串关键词' },
        { message: '   你好   ', keywords: ['你好'], expected: true, description: '前后空格消息' },
        { message: '你好', keywords: ['  你好  '], expected: true, description: '前后空格关键词' }
    ],
    specialCharacters: [
        { message: '你好！', keywords: ['你好'], expected: true, description: '带标点符号' },
        { message: '你好~', keywords: ['你好'], expected: true, description: '带特殊符号' },
        { message: '你好👋', keywords: ['你好'], expected: true, description: '带表情符号' },
        { message: '你好123', keywords: ['你好'], expected: true, description: '带数字' }
    ],
    longMessages: [
        { message: '你好，我想问一下这个商品的价格是多少，还有请问包邮吗？如果合适的话我就直接购买了，希望尽快回复我谢谢！', keywords: ['价格', '包邮'], expected: true, description: '长消息多关键词' },
        { message: '请问这个商品的详细情况是怎样的，包括价格、发货时间、售后服务等方面，希望能够详细介绍一下，谢谢您的耐心回复', keywords: ['价格', '发货'], expected: true, description: '超长消息' }
    ]
};

// 平台特定测试用例
const platformTestCases = {
    douyin: [
        { message: '你好，请问这个怎么卖？', keywords: ['你好', '怎么卖'], expected: true, platform: 'douyin' },
        { message: '这个视频拍得真好', keywords: ['价格', '多少钱'], expected: false, platform: 'douyin' },
        { message: 'UP主在吗？求链接', keywords: ['在吗', '链接'], expected: true, platform: 'douyin' }
    ],
    kuaishou: [
        { message: '老铁，这个价格多少钱？', keywords: ['价格', '多少钱'], expected: true, platform: 'kuaishou' },
        { message: '双击666', keywords: ['价格', '包邮'], expected: false, platform: 'kuaishou' },
        { message: '怎么购买啊老铁', keywords: ['购买'], expected: true, platform: 'kuaishou' }
    ],
    xiaohongshu: [
        { message: '姐妹，求购买链接！', keywords: ['购买', '链接'], expected: true, platform: 'xiaohongshu' },
        { message: '种草了，已收藏', keywords: ['价格', '多少钱'], expected: false, platform: 'xiaohongshu' },
        { message: '是正品吗？求推荐', keywords: ['正品', '推荐'], expected: true, platform: 'xiaohongshu' }
    ],
    xianyu: [
        { message: '包邮吗？成色怎么样？', keywords: ['包邮', '成色'], expected: true, platform: 'xianyu' },
        { message: '还在吗？急出吗？', keywords: ['在吗', '急出'], expected: true, platform: 'xianyu' },
        { message: '已拍下', keywords: ['价格', '发货'], expected: false, platform: 'xianyu' }
    ]
};

class KeywordMatchingTester {
    constructor() {
        this.token = null;
        this.userId = null;
        this.testResults = [];
        this.performanceResults = [];
    }

    async init() {
        logSection('初始化关键词匹配测试');
        
        // 清理日志文件
        if (fs.existsSync(LOG_FILE)) {
            fs.unlinkSync(LOG_FILE);
        }
        
        log('开始关键词匹配验证测试...', 'green');
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

    // 测试基础匹配算法
    async testBasicMatching() {
        logSection('基础关键词匹配测试');
        
        const results = {};
        
        for (const [category, cases] of Object.entries(testCases)) {
            log(`测试 ${category} 类别...`, 'yellow');
            
            const categoryResults = [];
            
            for (const testCase of cases) {
                try {
                    const startTime = Date.now();
                    
                    const response = await axios.post(`${API_BASE}/auto-reply/test-matching`, {
                        message: testCase.message,
                        keywords: testCase.keywords,
                        platform: 'test',
                        test_case: category
                    }, {
                        headers: { 'Authorization': `Bearer ${this.token}` }
                    });
                    
                    const endTime = Date.now();
                    const processingTime = endTime - startTime;
                    
                    const result = response.data.data;
                    const actual = result.matched;
                    const expected = testCase.expected;
                    const passed = actual === expected;
                    
                    categoryResults.push({
                        testCase: testCase,
                        actual: actual,
                        expected: expected,
                        passed: passed,
                        processingTime: processingTime,
                        matchedKeywords: result.matched_keywords || [],
                        status: 'success'
                    });
                    
                    const status = passed ? '✓' : '✗';
                    const color = passed ? 'green' : 'red';
                    
                    log(`${status} ${testCase.description}: "${testCase.message}" -> ${actual} (期望: ${expected}) [${processingTime}ms]`, color);
                    
                    // 记录性能数据
                    this.performanceResults.push({
                        category: category,
                        description: testCase.description,
                        processingTime: processingTime,
                        messageLength: testCase.message.length,
                        keywordCount: testCase.keywords.length
                    });
                    
                } catch (error) {
                    categoryResults.push({
                        testCase: testCase,
                        error: error.message,
                        status: 'failed'
                    });
                    
                    log(`✗ ${testCase.description}: 测试失败 - ${error.message}`, 'red');
                }
            }
            
            results[category] = categoryResults;
        }
        
        return results;
    }

    // 测试平台特定匹配
    async testPlatformSpecificMatching() {
        logSection('平台特定关键词匹配测试');
        
        const results = {};
        
        for (const [platform, cases] of Object.entries(platformTestCases)) {
            log(`测试 ${platform} 平台...`, 'yellow');
            
            const platformResults = [];
            
            for (const testCase of cases) {
                try {
                    const startTime = Date.now();
                    
                    const response = await axios.post(`${API_BASE}/auto-reply/test-matching`, {
                        message: testCase.message,
                        keywords: testCase.keywords,
                        platform: platform,
                        test_case: 'platform_specific'
                    }, {
                        headers: { 'Authorization': `Bearer ${this.token}` }
                    });
                    
                    const endTime = Date.now();
                    const processingTime = endTime - startTime;
                    
                    const result = response.data.data;
                    const actual = result.matched;
                    const expected = testCase.expected;
                    const passed = actual === expected;
                    
                    platformResults.push({
                        testCase: testCase,
                        actual: actual,
                        expected: expected,
                        passed: passed,
                        processingTime: processingTime,
                        matchedKeywords: result.matched_keywords || [],
                        status: 'success'
                    });
                    
                    const status = passed ? '✓' : '✗';
                    const color = passed ? 'green' : 'red';
                    
                    log(`${status} ${platform}: "${testCase.message}" -> ${actual} (期望: ${expected}) [${processingTime}ms]`, color);
                    
                } catch (error) {
                    platformResults.push({
                        testCase: testCase,
                        error: error.message,
                        status: 'failed'
                    });
                    
                    log(`✗ ${platform}: 测试失败 - ${error.message}`, 'red');
                }
            }
            
            results[platform] = platformResults;
        }
        
        return results;
    }

    // 测试性能基准
    async testPerformanceBenchmark() {
        logSection('关键词匹配性能基准测试');
        
        const benchmarkCases = [
            { messageLength: 10, keywordCount: 1, iterations: 100 },
            { messageLength: 50, keywordCount: 5, iterations: 100 },
            { messageLength: 100, keywordCount: 10, iterations: 100 },
            { messageLength: 200, keywordCount: 20, iterations: 50 },
            { messageLength: 500, keywordCount: 50, iterations: 20 }
        ];
        
        const results = [];
        
        for (const benchmark of benchmarkCases) {
            log(`测试消息长度 ${benchmark.messageLength}, 关键词数量 ${benchmark.keywordCount}, 迭代次数 ${benchmark.iterations}`, 'yellow');
            
            const processingTimes = [];
            
            // 生成测试数据
            const message = '测'.repeat(benchmark.messageLength);
            const keywords = Array.from({ length: benchmark.keywordCount }, (_, i) => `关键词${i}`);
            keywords[0] = '测'; // 确保有一个匹配的关键词
            
            for (let i = 0; i < benchmark.iterations; i++) {
                try {
                    const startTime = Date.now();
                    
                    const response = await axios.post(`${API_BASE}/auto-reply/test-matching`, {
                        message: message,
                        keywords: keywords,
                        platform: 'benchmark',
                        test_case: 'performance'
                    }, {
                        headers: { 'Authorization': `Bearer ${this.token}` }
                    });
                    
                    const endTime = Date.now();
                    const processingTime = endTime - startTime;
                    
                    processingTimes.push(processingTime);
                    
                } catch (error) {
                    log(`性能测试失败: ${error.message}`, 'red');
                }
            }
            
            // 计算统计信息
            const avgTime = processingTimes.reduce((a, b) => a + b, 0) / processingTimes.length;
            const minTime = Math.min(...processingTimes);
            const maxTime = Math.max(...processingTimes);
            
            const result = {
                messageLength: benchmark.messageLength,
                keywordCount: benchmark.keywordCount,
                iterations: benchmark.iterations,
                averageTime: avgTime.toFixed(2),
                minTime: minTime,
                maxTime: maxTime,
                processingTimes: processingTimes
            };
            
            results.push(result);
            
            log(`平均处理时间: ${avgTime.toFixed(2)}ms, 最小: ${minTime}ms, 最大: ${maxTime}ms`, 'cyan');
        }
        
        return results;
    }

    // 测试批量匹配
    async testBatchMatching() {
        logSection('批量关键词匹配测试');
        
        const batchSizes = [10, 50, 100, 200];
        const results = [];
        
        for (const batchSize of batchSizes) {
            log(`测试批量匹配，批量大小: ${batchSize}`, 'yellow');
            
            // 生成批量测试数据
            const batchMessages = [];
            for (let i = 0; i < batchSize; i++) {
                const messages = [
                    '你好，请问价格多少？',
                    '这个怎么卖？',
                    '包邮吗？',
                    '完全无关的消息',
                    '成色怎么样？'
                ];
                
                batchMessages.push({
                    message_id: `batch_msg_${i}`,
                    content: messages[i % messages.length],
                    sender_id: `user_${i}`
                });
            }
            
            try {
                const startTime = Date.now();
                
                const response = await axios.post(`${API_BASE}/auto-reply/test-batch-matching`, {
                    platform: 'douyin',
                    messages: batchMessages,
                    keywords: ['你好', '价格', '怎么卖', '包邮', '成色'],
                    batch_size: batchSize
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const endTime = Date.now();
                const totalTime = endTime - startTime;
                
                const batchResults = response.data.data.results;
                const matchedCount = batchResults.filter(r => r.matched).length;
                
                const result = {
                    batchSize: batchSize,
                    totalTime: totalTime,
                    averageTimePerMessage: (totalTime / batchSize).toFixed(2),
                    matchedCount: matchedCount,
                    matchRate: ((matchedCount / batchSize) * 100).toFixed(2),
                    results: batchResults
                };
                
                results.push(result);
                
                log(`批量处理完成: ${totalTime}ms, 平均每条 ${(totalTime / batchSize).toFixed(2)}ms, 匹配率: ${((matchedCount / batchSize) * 100).toFixed(2)}%`, 'cyan');
                
            } catch (error) {
                results.push({
                    batchSize: batchSize,
                    error: error.message,
                    status: 'failed'
                });
                
                log(`批量匹配测试失败: ${error.message}`, 'red');
            }
        }
        
        return results;
    }

    // 测试匹配算法准确性
    async testMatchingAccuracy() {
        logSection('关键词匹配算法准确性测试');
        
        // 创建大量随机测试用例
        const testMessages = [
            '你好，请问这个商品怎么卖？价格是多少？',
            '这个看起来不错，有购买链接吗？',
            '包邮吗？成色怎么样？能便宜点吗？',
            '支持什么支付方式？多久能发货？',
            '是正品吗？有售后服务吗？',
            '可以面交吗？在哪个城市？',
            '还在吗？急出吗？可以刀吗？',
            '已关注，求回复！',
            '这个价格有点贵，能优惠吗？',
            '请问有详细的产品介绍吗？'
        ];
        
        const keywords = ['你好', '价格', '多少钱', '怎么卖', '购买', '链接', '包邮', '成色', '便宜', '优惠', '正品', '发货', '急出', '面交'];
        
        const results = [];
        let correctPredictions = 0;
        let totalPredictions = 0;
        
        log('开始准确性测试...', 'yellow');
        
        for (let i = 0; i < 50; i++) {
            const message = testMessages[i % testMessages.length];
            const selectedKeywords = keywords.slice(0, Math.floor(Math.random() * keywords.length) + 1);
            
            try {
                const response = await axios.post(`${API_BASE}/auto-reply/test-matching`, {
                    message: message,
                    keywords: selectedKeywords,
                    platform: 'accuracy_test',
                    test_case: 'accuracy'
                }, {
                    headers: { 'Authorization': `Bearer ${this.token}` }
                });
                
                const result = response.data.data;
                
                // 手动计算预期结果（简单规则）
                const expected = selectedKeywords.some(keyword => 
                    message.toLowerCase().includes(keyword.toLowerCase())
                );
                
                const actual = result.matched;
                const correct = actual === expected;
                
                if (correct) {
                    correctPredictions++;
                }
                totalPredictions++;
                
                results.push({
                    message: message,
                    keywords: selectedKeywords,
                    expected: expected,
                    actual: actual,
                    correct: correct,
                    matchedKeywords: result.matched_keywords || [],
                    status: 'success'
                });
                
                if (i % 10 === 0) {
                    log(`已完成 ${i}/50 个准确性测试`, 'cyan');
                }
                
            } catch (error) {
                results.push({
                    message: message,
                    keywords: selectedKeywords,
                    error: error.message,
                    status: 'failed'
                });
                
                log(`准确性测试失败: ${error.message}`, 'red');
            }
        }
        
        const accuracy = ((correctPredictions / totalPredictions) * 100).toFixed(2);
        
        log(`准确性测试完成: ${accuracy}% 正确率 (${correctPredictions}/${totalPredictions})`, 'green');
        
        return {
            accuracy: accuracy,
            correctPredictions: correctPredictions,
            totalPredictions: totalPredictions,
            results: results
        };
    }

    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    async generateReport(allResults) {
        logSection('生成关键词匹配测试报告');
        
        // 计算总体统计
        let totalTests = 0;
        let passedTests = 0;
        let failedTests = 0;
        
        Object.values(allResults).forEach(categoryResults => {
            if (typeof categoryResults === 'object' && !Array.isArray(categoryResults)) {
                Object.values(categoryResults).forEach(results => {
                    if (Array.isArray(results)) {
                        results.forEach(result => {
                            if (result.status === 'success') {
                                totalTests++;
                                if (result.passed) {
                                    passedTests++;
                                } else {
                                    failedTests++;
                                }
                            }
                        });
                    }
                });
            }
        });
        
        // 性能分析
        const avgProcessingTime = this.performanceResults.length > 0 ? 
            (this.performanceResults.reduce((sum, r) => sum + r.processingTime, 0) / this.performanceResults.length).toFixed(2) : 0;
        
        const report = {
            timestamp: new Date().toISOString(),
            summary: {
                totalTests: totalTests,
                passedTests: passedTests,
                failedTests: failedTests,
                passRate: totalTests > 0 ? ((passedTests / totalTests) * 100).toFixed(2) : 0,
                averageProcessingTime: avgProcessingTime
            },
            performance: {
                averageTime: avgProcessingTime,
                totalTests: this.performanceResults.length,
                byCategory: this.analyzePerformanceByCategory()
            },
            details: allResults
        };
        
        const reportPath = path.join(__dirname, 'keyword-matching-test-report.json');
        fs.writeFileSync(reportPath, JSON.stringify(report, null, 2));
        
        log(`关键词匹配测试报告已生成: ${reportPath}`, 'green');
        log(`总测试数: ${report.summary.totalTests}`, 'cyan');
        log(`通过测试: ${report.summary.passedTests}`, 'green');
        log(`失败测试: ${report.summary.failedTests}`, 'red');
        log(`通过率: ${report.summary.passRate}%`, 'cyan');
        log(`平均处理时间: ${report.summary.averageProcessingTime}ms`, 'cyan');
        
        return report;
    }

    analyzePerformanceByCategory() {
        const categories = {};
        
        this.performanceResults.forEach(result => {
            if (!categories[result.category]) {
                categories[result.category] = {
                    count: 0,
                    totalTime: 0,
                    avgTime: 0,
                    minTime: Infinity,
                    maxTime: 0
                };
            }
            
            const cat = categories[result.category];
            cat.count++;
            cat.totalTime += result.processingTime;
            cat.minTime = Math.min(cat.minTime, result.processingTime);
            cat.maxTime = Math.max(cat.maxTime, result.processingTime);
        });
        
        Object.values(categories).forEach(cat => {
            cat.avgTime = (cat.totalTime / cat.count).toFixed(2);
            cat.minTime = cat.minTime === Infinity ? 0 : cat.minTime;
        });
        
        return categories;
    }

    async runAllTests() {
        await this.init();
        
        // 登录
        const loginSuccess = await this.login();
        if (!loginSuccess) {
            log('登录失败，终止测试', 'red');
            return;
        }
        
        const allResults = {};
        
        // 基础匹配测试
        const basicResults = await this.testBasicMatching();
        allResults.basic_matching = basicResults;
        
        // 平台特定匹配测试
        const platformResults = await this.testPlatformSpecificMatching();
        allResults.platform_matching = platformResults;
        
        // 性能基准测试
        const performanceResults = await this.testPerformanceBenchmark();
        allResults.performance_benchmark = performanceResults;
        
        // 批量匹配测试
        const batchResults = await this.testBatchMatching();
        allResults.batch_matching = batchResults;
        
        // 准确性测试
        const accuracyResults = await this.testMatchingAccuracy();
        allResults.accuracy_test = accuracyResults;
        
        // 生成测试报告
        await this.generateReport(allResults);
        
        logSection('关键词匹配测试完成');
        log('所有关键词匹配测试已执行完成！', 'green');
        log(`详细日志请查看: ${LOG_FILE}`, 'blue');
    }
}

// 运行测试
if (require.main === module) {
    const tester = new KeywordMatchingTester();
    tester.runAllTests().catch(error => {
        log(`关键词匹配测试执行失败: ${error.message}`, 'red');
        process.exit(1);
    });
}

module.exports = KeywordMatchingTester;