/**
 * 统一渠道/平台常量
 *
 * 单一事实源（Single Source of Truth）：所有渠道 label/tag/icon/group 都从本文件读取，
 * 业务视图（reachPipeline / messageHub / unifiedInbox / customerSession / customer360 /
 * QuickReply / AgentStatus / AgentBindingDialog / ReachChannelSelector 等）禁止再各自维护
 * 独立的 platformLabelMap / channelOptions / inline map。
 *
 * 设计要点：
 * 1. value 统一使用后端 channel_type 字段原值（wecom/weixin/whatsapp/...），不做大小写转换
 * 2. label 优先以"后端最常用"语义为准（wecom => 企业微信；与 messageHub/unifiedInbox 一致），
 *    reachPipeline 早期使用的"企微"通过 alias 兼容，确保存量代码无视觉跳变
 * 3. tagType 对齐 Element Plus el-tag 的 type：success/warning/danger/info/primary/''（空=默认）
 * 4. group 用于渠道分类（reach/im/card/web），便于按场景过滤子集
 * 5. icon 是 Element Plus 图标组件名（ReachChannelSelector 渲染使用）
 * 6. getChannelLabel 找不到时回退 value，绝不返回"未知"以避免歧义
 *
 * 历史背景：原项目 6 处独立定义（reachPipeline/channelOptions 9 条、messageHub/platformLabelMap 10 条、
 * unifiedInbox/platformLabelMap 10 条、ReachChannelSelector/allChannels 11 条、AgentBindingDialog inline 9 条、
 * QuickReply/<el-option> 6 条）互不一致，wecom 一处显"企微"、另一处显"企业微信"；
 * weixin/xianyu/tiktok/telegram 等渠道在多处缺漏。
 *
 * 维护规则：新增/修改渠道时只需修改本文件即可全局生效。
 */

// 渠道分组（用于按场景过滤子集）
export const CHANNEL_GROUP = Object.freeze({
  IM: 'im',           // 即时通讯：wecom/weixin/whatsapp/telegram/feishu
  SOCIAL: 'social',   // 社交内容：douyin/kuaishou/xiaohongshu/xianyu/tiktok/personal_wx
  NOTIFY: 'notify',   // 通知触达：sms/email
  COLLAB: 'collab',   // 协作办公：dingtalk/feishu
  CARD: 'card',       // 卡片：card
  WEB: 'web'          // Web Widget：web
})

// 主渠道定义（权威列表）
// value     后端 channel_type 原值
// label     渠道中文标签（唯一显示文案）
// alias     兼容旧 label（reachPipeline 早期 "企微" 风格），如需历史兼容可读 CHANNEL_LABEL_ALIAS
// tagType   Element Plus el-tag 类型
// group     所属分组
// icon      Element Plus 图标组件名
// description  渠道描述
// newBadge  是否显示 NEW 角标（ReachChannelSelector 用）
export const CHANNEL_OPTIONS = Object.freeze([
  { value: 'wecom',         label: '企业微信', tagType: 'success', group: CHANNEL_GROUP.IM,     icon: 'OfficeBuilding', description: '企业微信（外部联系人）', alias: ['企微'] },
  { value: 'weixin',        label: '微信公众号', tagType: 'success', group: CHANNEL_GROUP.IM,   icon: 'ChatDotRound',    description: '微信公众号（客服消息）', alias: ['微信'] },
  { value: 'personal_wx',   label: '个人微信',   tagType: 'success', group: CHANNEL_GROUP.SOCIAL, icon: 'ChatDotRound',  description: '个人微信（聚合）' },
  { value: 'whatsapp',      label: 'WhatsApp',  tagType: 'success', group: CHANNEL_GROUP.IM,     icon: 'ChatLineRound',   description: 'WhatsApp Cloud API（Meta 商业）', newBadge: true },
  { value: 'telegram',      label: 'Telegram',  tagType: 'primary', group: CHANNEL_GROUP.IM,     icon: 'Promotion',       description: 'Telegram Bot API（境外 IM）', newBadge: true },
  { value: 'feishu',        label: '飞书',      tagType: 'primary', group: CHANNEL_GROUP.COLLAB, icon: 'ChatLineSquare',  description: '飞书 Open API（协作）', newBadge: true },
  { value: 'dingtalk',      label: '钉钉',      tagType: 'primary', group: CHANNEL_GROUP.COLLAB, icon: 'Connection',      description: '钉钉机器人' },
  { value: 'douyin',        label: '抖音',      tagType: '',        group: CHANNEL_GROUP.SOCIAL, icon: 'Share',           description: '抖音私信' },
  { value: 'kuaishou',      label: '快手',      tagType: '',        group: CHANNEL_GROUP.SOCIAL, icon: 'Share',           description: '快手私信' },
  { value: 'xiaohongshu',   label: '小红书',    tagType: 'danger',  group: CHANNEL_GROUP.SOCIAL, icon: 'Postcard',        description: '小红书私信' },
  { value: 'xianyu',        label: '闲鱼',      tagType: 'warning', group: CHANNEL_GROUP.SOCIAL, icon: 'Goods',           description: '闲鱼私信' },
  { value: 'tiktok',        label: 'TikTok',    tagType: '',        group: CHANNEL_GROUP.SOCIAL, icon: 'VideoCamera',     description: 'TikTok 私信' },
  { value: 'sms',           label: '短信',      tagType: 'info',    group: CHANNEL_GROUP.NOTIFY, icon: 'Cellphone',       description: '短信触达（模板/直发）' },
  { value: 'email',         label: '邮件',      tagType: 'info',    group: CHANNEL_GROUP.NOTIFY, icon: 'Message',         description: '邮件触达（支持附件）' },
  { value: 'card',          label: '卡片',      tagType: 'info',    group: CHANNEL_GROUP.CARD,   icon: 'Postcard',        description: '卡片消息（子渠道）' },
  { value: 'web',           label: 'Web Widget', tagType: 'info',   group: CHANNEL_GROUP.WEB,    icon: 'Monitor',         description: '客服 Web Widget 渠道' },
  { value: 'web_embed',     label: '网页嵌入',   tagType: 'info',   group: CHANNEL_GROUP.WEB,    icon: 'Monitor',         description: 'Web Widget 嵌入访客端（第三方网站访客）' }
])

// 兼容历史 label（如 reachPipeline 旧的"企微"），用于内部展示/迁移判断
// 注意：此 alias 仅用于历史数据/迁移期，业务 UI 一律使用 getChannelLabel 取标准 label
export const CHANNEL_LABEL_ALIAS = Object.freeze(
  CHANNEL_OPTIONS.reduce((acc, o) => {
    if (o.alias && o.alias.length) {
      o.alias.forEach((a) => { if (!acc[a]) acc[a] = o.label })
    }
    return acc
  }, {})
)

// value -> label 快查（性能优化：避免在 table 渲染时每次 .find）
export const CHANNEL_LABEL_MAP = Object.freeze(
  CHANNEL_OPTIONS.reduce((acc, o) => { acc[o.value] = o.label; return acc }, {})
)

// value -> tagType 快查
export const CHANNEL_TAG_TYPE_MAP = Object.freeze(
  CHANNEL_OPTIONS.reduce((acc, o) => { acc[o.value] = o.tagType; return acc }, {})
)

// value -> 完整定义 快查
export const CHANNEL_MAP = Object.freeze(
  CHANNEL_OPTIONS.reduce((acc, o) => { acc[o.value] = o; return acc }, {})
)

/**
 * 获取渠道中文标签
 * @param {string|undefined|null} value 渠道 value
 * @returns {string} label；找不到时回退 value，未知值返回 '-' 避免渲染 undefined
 */
export const getChannelLabel = (value) => {
  if (value === undefined || value === null || value === '') return '-'
  if (CHANNEL_LABEL_MAP[value]) return CHANNEL_LABEL_MAP[value]
  // 兼容历史 alias（极少情况）
  if (CHANNEL_LABEL_ALIAS[value]) return CHANNEL_LABEL_ALIAS[value]
  return value
}

/**
 * 获取渠道 el-tag 类型
 * @param {string} value
 * @returns {string} tag type，找不到时返回 ''（el-tag 默认色）
 */
export const getChannelTagType = (value) => CHANNEL_TAG_TYPE_MAP[value] || ''

/**
 * 获取渠道完整定义
 * @param {string} value
 * @returns {object|null}
 */
export const getChannelOption = (value) => CHANNEL_MAP[value] || null

/**
 * 按 group 过滤渠道列表
 * @param {string|string[]} groups 一个或多个 group 名
 * @returns {Array}
 */
export const filterChannelsByGroup = (groups) => {
  const list = Array.isArray(groups) ? groups : [groups]
  return CHANNEL_OPTIONS.filter((o) => list.includes(o.group))
}

/**
 * 排除指定渠道（按 value）
 * @param {string[]} excludeValues
 * @returns {Array}
 */
export const excludeChannels = (excludeValues = []) => {
  const set = new Set(excludeValues)
  return CHANNEL_OPTIONS.filter((o) => !set.has(o.value))
}

/**
 * 仅包含指定渠道（按 value）
 * @param {string[]} includeValues
 * @returns {Array}
 */
export const includeChannels = (includeValues = []) => {
  const set = new Set(includeValues)
  return CHANNEL_OPTIONS.filter((o) => set.has(o.value))
}

export default {
  CHANNEL_GROUP,
  CHANNEL_OPTIONS,
  CHANNEL_LABEL_MAP,
  CHANNEL_TAG_TYPE_MAP,
  CHANNEL_LABEL_ALIAS,
  CHANNEL_MAP,
  getChannelLabel,
  getChannelTagType,
  getChannelOption,
  filterChannelsByGroup,
  excludeChannels,
  includeChannels
}
