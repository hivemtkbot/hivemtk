// 路由配置中使用的图标映射表
//
// 用途：替代 `import * as ElementPlusIconsVue from '@element-plus/icons-vue'` 的全量命名空间导入,
// 仅显式导入路由配置 (router/modules/*.js) 与 Layout/Breadcrumb 中通过字符串引用的图标,
// 使打包工具能进行 tree-shaking,显著减小 elementPlus chunk 体积。
//
// 详见: docs/audit/USER_PROJECT_INSPECTION_BRAINSTORM.md P1-1
//
// 维护说明：新增路由模块若使用新的图标字符串,需同步在此处补 import + map 条目。
//
// 注意：以下路由配置中引用但 @element-plus/icons-vue 不导出的图标名,不在此处 import,
//       运行时由 resolveRouteIcon 回退到 Document:
//   - Cloud  (router/modules/system.js 存储配置)
//   - Inbox  (layout/Layout.vue 统一收件箱)
//   - Shield (router/modules/securityAudit.js 安全审计)

import {
  Aim,
  Bell,
  ChatDotRound,
  ChatDotSquare,
  ChatLineRound,
  ChatLineSquare,
  ChatSquare,
  Collection,
  CollectionTag,
  Connection,
  Cpu,
  DataAnalysis,
  DataBoard,
  DataLine,
  Document,
  Edit,
  EditPen,
  Files,
  Filter,
  FolderOpened,
  Goods,
  Guide,
  Headset,
  HomeFilled,
  Key,
  Link,
  List,
  Lock,
  MagicStick,
  Message,
  MessageBox,
  Monitor,
  Operation,
  Picture,
  PieChart,
  Platform,
  Plus,
  PriceTag,
  Promotion,
  Service,
  Setting,
  SetUp,
  Shop,
  ShoppingBag,
  Tickets,
  Tools,
  TrendCharts,
  UploadFilled,
  User,
  UserFilled,
  VideoPlay,
  Warning,
} from '@element-plus/icons-vue'

// 路由图标映射：菜单配置中的 icon 字符串 → 图标组件
export const routeIconMap = {
  Aim,
  Bell,
  ChatDotRound,
  ChatDotSquare,
  ChatLineRound,
  ChatLineSquare,
  ChatSquare,
  Collection,
  CollectionTag,
  Connection,
  Cpu,
  DataAnalysis,
  DataBoard,
  DataLine,
  Document,
  Edit,
  EditPen,
  Files,
  Filter,
  FolderOpened,
  Goods,
  Guide,
  Headset,
  HomeFilled,
  Key,
  Link,
  List,
  Lock,
  MagicStick,
  Message,
  MessageBox,
  Monitor,
  Operation,
  Picture,
  PieChart,
  Platform,
  Plus,
  PriceTag,
  Promotion,
  Service,
  Setting,
  SetUp,
  Shop,
  ShoppingBag,
  Tickets,
  Tools,
  TrendCharts,
  UploadFilled,
  User,
  UserFilled,
  VideoPlay,
  Warning,
}

// 默认回退图标：菜单图标未命中(或引用了不存在的图标名)时使用
const DEFAULT_ROUTE_ICON = Document

// resolveRouteIcon 按字符串名取出路由图标组件,未命中时回退到 Document
export function resolveRouteIcon(name) {
  if (!name) return DEFAULT_ROUTE_ICON
  return routeIconMap[name] || DEFAULT_ROUTE_ICON
}
