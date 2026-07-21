const axios = require('axios');
const fs = require('fs');

// API配置
const API_BASE = process.env.API_BASE || 'http://localhost:8204/api';
const LOG_FILE = 'headless-mode-test.log';

// 日志函数
function log(message) {
    const timestamp = new Date().toISOString();
    const logMessage = `[${timestamp}] ${message}\n`;
    console.log(logMessage.trim());
    fs.appendFileSync(LOG_FILE, logMessage);
}

// 测试函数
async function testHeadlessMode() {
    log('开始无头模式测试...');
    
    try {
        // 1. 用户登录
        log('用户登录...');
        const loginResponse = await axios.post(`${API_BASE}/auth/login`, {
            username: 'admin',
            password: '123456'
        });
        
        const token = loginResponse.data.data.token;
        const headers = { Authorization: `Bearer ${token}` };
        log(`登录成功，获取token: ${token.substring(0, 20)}...`);
        
        // 2. 获取当前无头模式状态
        log('\n获取当前无头模式状态...');
        const getResponse = await axios.get(`${API_BASE}/auto-reply/headless`, { headers });
        const currentHeadless = getResponse.data.data.headless;
        log(`当前无头模式: ${currentHeadless ? '后台运行' : '可视化模式'}`);
        
        // 3. 切换无头模式
        const newHeadless = !currentHeadless;
        log(`\n切换无头模式为: ${newHeadless ? '后台运行' : '可视化模式'}`);
        
        const setResponse = await axios.post(`${API_BASE}/auto-reply/headless`, {
            headless: newHeadless
        }, { headers });
        
        log(`切换结果: ${setResponse.data.msg}`);
        log(`描述: ${setResponse.data.data.description}`);
        
        // 4. 验证切换结果
        log('\n验证切换结果...');
        const verifyResponse = await axios.get(`${API_BASE}/auto-reply/headless`, { headers });
        const verifyHeadless = verifyResponse.data.data.headless;
        
        if (verifyHeadless === newHeadless) {
            log('✅ 无头模式切换验证成功');
        } else {
            log('❌ 无头模式切换验证失败');
        }
        
        // 5. 测试启动机器人（使用新设置）
        log('\n测试启动机器人（使用新设置）...');
        
        // 首先获取账号信息
        log('获取账号列表...');
        const accountsResponse = await axios.get(`${API_BASE}/auto-reply/accounts?platform=douyin`, { headers });
        const accounts = accountsResponse.data.data;
        
        if (accounts && accounts.length > 0) {
            const account = accounts[0];
            log(`使用账号: ${account.username} (ID: ${account.id})`);
            
            // 启动机器人
            log('启动自动回复机器人...');
            const startResponse = await axios.post(`${API_BASE}/auto-reply/start`, {
                platform: 'douyin',
                account_id: account.id
            }, { headers });
            
            log(`启动结果: ${startResponse.data.msg}`);
            log(`当前无头模式: ${startResponse.data.data.headless ? '后台运行' : '可视化模式'}`);
            
            // 等待几秒钟让浏览器启动
            log('等待5秒让浏览器启动...');
            await new Promise(resolve => setTimeout(resolve, 5000));
            
            // 停止机器人
            log('\n停止自动回复机器人...');
            const stopResponse = await axios.post(`${API_BASE}/auto-reply/stop`, {
                platform: 'douyin'
            }, { headers });
            
            log(`停止结果: ${stopResponse.data.msg}`);
        } else {
            log('⚠️  没有找到可用的账号，请先创建账号');
        }
        
        // 6. 恢复原始设置
        log('\n恢复原始无头模式设置...');
        const restoreResponse = await axios.post(`${API_BASE}/auto-reply/headless`, {
            headless: currentHeadless
        }, { headers });
        
        log(`恢复结果: ${restoreResponse.data.data.description}`);
        
        log('\n✅ 无头模式测试完成');
        
    } catch (error) {
        log(`❌ 测试失败: ${error.message}`);
        if (error.response) {
            log(`错误响应: ${JSON.stringify(error.response.data)}`);
        }
    }
}

// 主函数
async function main() {
    // 清空日志文件
    fs.writeFileSync(LOG_FILE, '');
    
    log('============================================================');
    log('无头浏览器模式控制测试');
    log('============================================================');
    log(`API地址: ${API_BASE}`);
    log(`日志文件: ${LOG_FILE}`);
    log('');
    
    await testHeadlessMode();
    
    log('');
    log('============================================================');
    log('测试完成，详细日志请查看: ' + LOG_FILE);
    log('============================================================');
}

// 运行测试
if (require.main === module) {
    main().catch(console.error);
}

module.exports = { testHeadlessMode };