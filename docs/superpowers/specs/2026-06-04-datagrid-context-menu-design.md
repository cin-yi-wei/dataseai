# DataGrid 右键菜单 + 编辑功能设计文档

**日期**: 2026-06-04  
**项目**: dataseai  
**范围**: Web 前端 DataGrid 组件增强

---

## 概述

当前 DataGrid 仅支持双击编辑单个格子。本设计添加：

1. **右键菜单** — 15+ 个菜单项，支持子菜单
2. **编辑 Modal** — 支持简单值和 JSON 树形编辑
3. **Bug 修复** — 空字符串无法编辑的问题

---

## 问题陈述

### 现状
- 双击可以编辑格子，但**空字符串无法点击**（显示为完全空白）
- 编辑方式单一，用户无法快速操作（复制、过滤、设置默认值等）
- 没有对 JSON 数据的友好编辑体验

### 用户需求
- 能够编辑空字符串
- 右键快速访问 15+ 种操作（复制、过滤、删除等）
- JSON 数据支持树形查看和编辑（类似 DBeaver）
- 支持多种格式复制（JSON、TSV、Markdown、Insert statement）

---

## 菜单项设计

### 最终菜单结构

| 组别 | 菜单项 | 快捷键 | 功能说明 |
|------|--------|--------|---------|
| **查看/编辑** | Quick Look Editor | Ctrl Enter | 打开树形查看/编辑器（JSON 显示左树+右编辑） |
| | Edit in modal | | 打开编辑 modal |
| | Set Value → | | 子菜单：EMPTY / NULL / DEFAULT |
| **行操作** | Refresh | Ctrl Alt R | 刷新数据 |
| | Paste | Ctrl V | 粘贴剪贴板内容到当前格子 |
| | Add row | Ctrl I | 新增空行 |
| | Duplicate | Ctrl D | 复制当前行 |
| **复制操作** | Copy | Ctrl C | 复制整行（tab 分隔格式） |
| | Copy Cell Value | | 只复制该格子值 |
| | Copy All Column Values | | 复制整列（tab 分隔格式） |
| | Copy As → | | 子菜单：JSON / TSV for Excel / Markdown / Insert statement |
| **筛选/删除** | Quick Filter → | | 子菜单：各种过滤条件 |
| | Delete row | Delete | 删除当前行 |

### Set Value 子菜单

- **EMPTY** — 设置为空字符串 `""`
- **NULL** — 设置为 `NULL`
- **DEFAULT** — 设置为列定义的默认值（从 CREATE TABLE 获取）

### Copy As 子菜单

- **JSON** — 输出 JSON 格式
- **TSV for Excel** — Tab 分隔，适合 Excel 粘贴
- **Markdown** — Markdown 代码块格式
- **Insert statement** — 完整的 INSERT 语句（包含列名和值）

### Quick Filter 子菜单

基于用户提供的图，支持：
- `=` (精确匹配)
- `Contains` (包含)
- `Not contains` (不包含)
- `Has prefix` (前缀)
- `Has suffix` (后缀)
- `IS NULL`
- `IS NOT NULL`

---

## 编辑 Modal 设计

### A. 简单值编辑模式

适用于：字符串、数字、日期、布尔值等标量类型

```
┌──────────────────────────┐
│ Edit Cell                │
├──────────────────────────┤
│ [多行文本输入框]         │
│ (自动扩展高度)           │
├──────────────────────────┤
│ [Cancel] [Copy] [Apply]  │
└──────────────────────────┘
```

**行为**：
- 自动聚焦输入框
- `Enter` 提交，`Escape` 取消
- `Copy` 按钮复制当前内容到剪贴板

### B. JSON 树形编辑模式

适用于：JSON 类型的列

```
┌─────────────────────────────────────┐
│ Edit Cell (JSON)                    │
├────────────────────┬────────────────┤
│ 左：树形结构       │ 右：值编辑     │
│                    │                │
│ ▼ ROOT             │ type: string   │
│ ├─ url         →   │ ["http://..."] │
│ ├─ method          │         ▼      │
│ ├─ payload         │                │
│ │ ├─ amount        │ type: number   │
│ │ ├─ api_id        │ [0.0]      ▼   │
│ │ └─ ...           │                │
│ ├─ headers         │                │
│ └─ mock_fake       │ type: null     │
│                    │ [NULL]     ▼   │
├─────────────────────────────────────┤
│ [Cancel] [Copy] [Raw/Format] [Apply]│
└─────────────────────────────────────┘
```

**行为**：
- **左树**：显示 JSON 结构树，支持展开/折叠
  - 点击节点时，右侧显示该节点的值
  - 自动缩进，清晰显示层级
- **右侧编辑区**：
  - 显示选中节点的类型和当前值
  - 对于标量值（string、number、boolean）：文本框或下拉
  - 对于 `null`：显示为 `NULL`（不可编辑，除非选 Set Value）
  - 对于对象/数组：显示 `[object]` / `[array]`，不直接编辑
- **底部按钮**：
  - `Raw` — 切换到原始 JSON 文本编辑（全文本框）
  - `Format` — 切换回树形视图
  - `Copy` — 复制当前 JSON 到剪贴板
  - `Apply` — 保存修改

---

## Bug 修复

### 空字符串无法编辑的问题

**原因**：当前代码中，空字符串被渲染为空白 `<span></span>`，无法点击触发 `onDoubleClick`。

**修复方案**：
- 所有值（包括空字符串、NULL）都渲染为可点击的 `<span>` 或占位符
- 空字符串显示为 `<span style={{color: '#ccc'}}>·</span>` 或类似的可见占位符
- NULL 继续显示为灰色 `NULL`

---

## 组件架构

### 新增组件

```
web/src/components/
├── DataGrid.tsx (修改)
│   ├── 集成右键菜单、编辑 modal
│   └── 修复空字符串编辑问题
│
├── CellContextMenu.tsx (新建)
│   ├── 右键菜单容器
│   ├── 菜单项列表
│   ├── 子菜单处理（Set Value、Copy As、Quick Filter）
│   └── 事件处理（onClick 回调）
│
├── EditCellModal.tsx (新建)
│   ├── Modal 容器和生命周期
│   ├── 简单值 vs JSON 模式判断
│   ├── 模式路由：
│   │   ├── SimpleCellEditor（简单值）
│   │   └── JsonTreeEditor（JSON）
│   └── 提交/取消处理
│
├── SimpleCellEditor.tsx (新建)
│   └── 多行文本框 + Cancel/Copy/Apply 按钮
│
└── JsonTreeEditor.tsx (新建)
    ├── 左树：JSON 树形渲染
    ├── 右侧：值编辑面板
    ├── Raw/Format 切换
    └── 修改提交逻辑
```

### 数据流

```
用户右键点击格子
  ↓
CellContextMenu 打开，获取 (row, col) 和单元格值
  ↓
用户选择菜单项
  ↓
回调函数执行：
  - Set Value → API PATCH
  - Copy → 复制到剪贴板
  - Delete → API DELETE
  - Edit in modal → 打开 EditCellModal
  - Quick Filter → 应用过滤条件（API 参数）
```

---

## 实现细节

### 右键菜单实现

- 使用 HTML5 `onContextMenu` 事件
- 菜单定位于鼠标坐标（带边界检测）
- 点击菜单外或按 Escape 关闭菜单
- 子菜单：悬停显示（不需要另外点击）

### API 交互

新增或修改的 API 调用：

| 操作 | 现有 API | 备注 |
|------|---------|------|
| 编辑格子 | PATCH `/api/db/.../rows` | 已实现 |
| Set Value (NULL) | PATCH `/api/db/.../rows` + `new_value: null` | 现有逻辑支持 |
| Set Value (DEFAULT) | 需要新 API 或特殊值标记 | TBD |
| 快速过滤 | 修改 GET `/api/db/.../data` 查询参数 | TBD |
| 删除行 | DELETE `/api/db/.../rows` | 已实现 |

### Copy As 实现

- **Copy as JSON** — `JSON.stringify(value)`
- **Copy as TSV** — 整行所有列用 `\t` 连接
- **Copy as Markdown** — 用 ` ``` json\n ... \n ``` ` 包裹
- **Copy as Insert statement** — 生成 `INSERT INTO table (col1, col2, ...) VALUES (...)`

---

## 测试点

| 场景 | 预期行为 |
|------|---------|
| 双击空字符串 | 进入编辑模式 |
| 右键编辑空字符串 | 打开 modal，可编辑 |
| 右键复制包含 `NULL` 的行 | 正确处理 NULL（不引用） |
| 右键复制 JSON 列 | Copy Cell Value 复制 JSON 字符串；Copy as JSON 输出格式化 JSON |
| 编辑 JSON → Apply | 验证 JSON 有效性，保存修改 |
| Quick Filter 后排序 | 过滤条件保留，分页数据正确 |

---

## 范围和限制

### 包含在本设计中
- 右键菜单和所有子菜单
- 编辑 modal（简单值和 JSON）
- Bug 修复（空字符串）
- Copy As 多种格式

### 不包含（后续优化）
- XML、YAML 等其他结构化格式编辑
- 批量编辑多个格子
- 撤销/重做（Undo/Redo）
- 自定义过滤条件保存

---

## 优先级和时序

| 阶段 | 内容 |
|------|------|
| P0 (必须) | 右键菜单基础；Edit modal (简单值)；Set Value；Copy/Copy Cell Value；Delete row |
| P1 (重要) | JSON 树形编辑；Copy As；Quick Filter |
| P2 (可选) | 高级过滤；批量操作 |

