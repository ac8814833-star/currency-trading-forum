# 货币交易论坛 (Currency Trading Forum)

一个面向货币交易爱好者的技术论坛，支持文章发布、点赞排行、评论互动、汇率查询，并集成了基于 RAG 技术的 AI 智能问答助手。

---

## 技术栈

| 层级 | 技术 |
|---|---|
| **前端** | Vue 3 + TypeScript + Vite + Pinia + Element Plus |
| **后端** | Go 1.24 + Gin + GORM |
| **数据库** | MySQL 8.0 |
| **缓存** | Redis 3+（Docker 环境使用 Redis 7） |
| **消息队列** | RabbitMQ 3 |
| **检索引擎** | Elasticsearch 8（BM25）+ MySQL FULLTEXT 降级 |
| **向量数据库** | ChromaDB |
| **AI 框架** | Eino (ByteDance) + DashScope (通义千问) |
| **容器化** | Docker + Docker Compose |

## 架构总览

```
┌─────────────────┐     ┌──────────────────────────────────────────────┐
│   Browser (Vue)  │────▶│  Go Backend (Gin, :3000)                     │
└─────────────────┘     │                                              │
                        │  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
                        │  │  MySQL    │  │  Redis   │  │ RabbitMQ   │  │
                        │  │  8.0      │  │  3+      │  │ 3          │  │
                        │  └────▲─────┘  └────┬─────┘  └─────┬──────┘  │
                        │       │             │              │         │
                        │       │   点赞 Lua + List Outbox    │         │
                        │       └──── 点赞消费者 ◀────────────┘         │
                        │                     │                        │
                        │  ┌──────────┐              ┌───────▼──────┐  │
                        │  │ ChromaDB │◀─────────────│ RAG Indexer │  │
                        │  │ (向量库)  │              │ (异步索引)   │  │
                        │  └────┬─────┘              └──────────────┘  │
                        │       │                                      │
                        │  ┌────▼─────┐                                │
                        │  │ DashScope │  (Embedding + LLM 问答)       │
                        │  └──────────┘                                │
                        └──────────────────────────────────────────────┘
```

## 功能列表

### 用户系统
- [x] 注册 / 登录（图形验证码 + 密码 bcrypt 加密）
- [x] JWT 双 Token 认证（Access Token 120min + Refresh Token 7天）
- [x] Token 自动刷新（前端 axios 拦截器静默续期）
- [x] 登出（Refresh Token 吊销 + Access Token 黑名单）
- [x] 接口限流（基于 Redis 令牌桶算法）

### 文章管理
- [x] 发布 / 编辑 / 删除文章
- [x] 文章列表分页 + 详情查看
- [x] 文章详情缓存（Redis）；列表直接按页查询数据库

### 互动功能
- [x] 点赞 / 取消点赞（Redis Lua 原子更新状态、计数、排行榜和 List outbox）
- [x] RabbitMQ 异步持久化点赞（Publisher Confirm、版本幂等、多消费者）
- [x] 热门文章排行榜（Redis ZSet 实时排序；投影恢复或 Redis 运行异常时返回短暂不可用，避免展示陈旧数据库排名）
- [x] 顶级评论分页、两层回复和删除回复子树

### 汇率模块
- [x] 系统示例汇率查询与交叉换算（按最新记录读取；不宣称实时行情）

### AI 智能问答 (RAG)
- [x] 文章生命周期自动索引：发布/编辑后通过 RabbitMQ 异步向量化，失败由全量/增量同步补偿
- [x] Eino Recursive Splitter 递归分块 + ChromaDB 向量检索，支持父子块上下文回填
- [x] 混合检索：向量召回与 Elasticsearch BM25 通过 RRF 融合；ES 不可用时回退到 MySQL FULLTEXT/LIKE 候选召回
- [x] 基于检索片段生成答案；无召回时直接返回“无法确定”，不调用模型生成
- [x] RAG 问答、手动索引均按用户限流；手动索引仅允许文章作者调用
- [x] 布隆过滤器（Redis Bitmap，缺失/未就绪时回源数据库，避免误判）

---

## 环境要求

| 软件 | 最低版本 | 说明 |
|---|---|---|
| Go | 1.24+ | 后端运行环境 |
| Node.js | 20+ | 前端构建 |
| MySQL | 8.0 | 关系数据库 |
| Redis | 3+ | 文章详情缓存 / 布隆过滤器 / 限流 / 点赞状态与 outbox |
| RabbitMQ | 3 | RAG 索引与点赞异步持久化队列 |
| ChromaDB | latest | 向量数据库（RAG 检索） |
| Docker | 20+ | 可选，容器化部署 |

---

## 快速开始

### 1. 克隆项目

```bash
git clone <repo-url>
cd "Currency trading forum"
```

### 2. 启动基础设施（Docker）

```bash
docker compose -f docker-compose.infra.yml up -d
```

这会启动 MySQL、Redis、RabbitMQ、ChromaDB 四个服务。

### 3. 配置环境变量

复制 `.env.example` 为 `.env`，填入阿里云 DashScope API Key 与 JWT 密钥。后端本地启动时会读取项目根目录的 `.env`；生产环境请通过部署平台的环境变量注入。

```bash
cp .env.example .env
```

```
RAG_DASHSCOPE_KEY="your-dashscope-api-key"
JWT_SECRET="replace-with-a-random-secret"
TRUSTED_PROXIES="" # 仅在明确知道反向代理 IP/CIDR 时设置
```

### 4. 启动后端

```bash
cd back

# 安装依赖
go mod tidy

# 启动（默认监听 :3000）
go run main.go
```

数据库表会在首次启动时自动创建（GORM AutoMigrate）。MySQL 全文索引也会自动检查并创建。

### 5. 启动前端

```bash
cd front

# 安装依赖
npm install

# 启动开发服务器（默认 http://localhost:5173）
npm run dev
```

前端开发服务器已配置 API 代理，请求自动转发到后端 `:3000`。

### 6. 访问

- 前端页面：http://localhost:5173
- 后端 API：http://localhost:3000/api
- RabbitMQ 管理界面：http://localhost:15672（guest/guest）

---

## Docker 一键部署（生产模式）

```bash
# 确保 .env 文件已配置 RAG_DASHSCOPE_KEY 与 JWT_SECRET；生产 Compose 会拒绝空 JWT_SECRET
docker compose -f docker-compose.prod.yml up -d
```

生产模式下前端通过 Nginx 在 `:80` 端口提供服务，后端在内部网络运行。

---

## 项目结构

```
├── back/                        # Go 后端
│   ├── config/                  # 配置（数据库、Redis、RabbitMQ、JWT、RAG）
│   │   ├── config.go            # 配置加载（viper）
│   │   ├── config.yml           # 配置文件
│   │   ├── db.go                # MySQL 连接
│   │   ├── redis.go             # Redis 连接
│   │   ├── rabbitmq.go          # RabbitMQ 连接与队列声明
│   │   └── logger.go            # 日志初始化
│   ├── controllers/             # 控制器层
│   │   ├── auth_controllers.go  # 注册 / 登录 / 刷新 / 登出
│   │   ├── article_controller.go# 文章 CRUD
│   │   ├── comment_controller.go# 评论管理
│   │   ├── like_controller.go   # 点赞 / 排行榜
│   │   ├── exchange_rate_controller.go # 汇率
│   │   └── rag_controller.go    # RAG 索引 / 问答
│   ├── middlewares/             # 中间件
│   │   ├── auth_middlewares.go  # JWT 鉴权 + 黑名单检查
│   │   └── rate_limit.go        # Redis 令牌桶限流
│   ├── models/                  # 数据模型（GORM）
│   ├── router/router.go         # 路由定义
│   ├── utils/                   # 工具层
│   │   ├── utils.go             # JWT 生成 / 解析 / 黑名单
│   │   ├── captcha.go           # 图形验证码
│   │   ├── bloom.go             # 布隆过滤器（Redis Bitmap）
│   │   ├── like_atomic.go       # Redis Lua 点赞状态、ZSet 排行和 outbox
│   │   ├── like_store.go        # 未配置 Redis 时的同步事务降级
│   │   ├── like_relay.go        # Redis List outbox → RabbitMQ Confirm
│   │   ├── like_queue.go        # 点赞消息定义与 Confirm 发布器
│   │   ├── like_worker.go       # 多消费者幂等持久化点赞关联
│   │   ├── like_stats_worker.go # 批量同步 articles.likes_count
│   │   ├── rag_service.go       # RAG：文本切割 / 向量化 / 检索 / LLM 问答
│   │   ├── rag_indexer.go       # RAG：文章索引
│   │   ├── rag_hybrid.go        # RAG：混合检索（向量 + BM25）
│   │   ├── rag_sync_worker.go   # RAG：全量 / 增量同步
│   │   └── redis_utils.go       # Redis 工具
│   ├── global/global.go         # 全局变量（DB、Redis、RabbitMQ 连接）
│   ├── main.go                  # 入口
│   ├── Dockerfile
│   └── go.mod / go.sum
├── front/                       # Vue 3 前端
│   ├── src/
│   │   ├── components/          # 登录 / 注册组件
│   │   ├── views/               # 页面视图
│   │   │   ├── HomeView.vue     # 首页导航
│   │   │   ├── NewsView.vue     # 文章列表与搜索
│   │   │   ├── NewsDetailView.vue       # 文章详情
│   │   │   ├── CreateArticleView.vue    # 发布文章
│   │   │   ├── EditArticleView.vue      # 编辑文章
│   │   │   ├── CurrencyExchangeView.vue # 汇率参考与换算
│   │   │   └── RAGAssistantView.vue     # AI 问答助手
│   │   ├── router/              # 路由
│   │   ├── store/auth.ts        # 认证状态（Pinia）
│   │   ├── axios.ts             # HTTP 客户端（含 Token 自动刷新）
│   │   ├── App.vue
│   │   └── main.ts
│   ├── Dockerfile
│   ├── nginx.conf
│   └── package.json
├── deploy/back/config.yml       # 生产环境配置
├── docker-compose.infra.yml     # 开发基础设施
├── docker-compose.prod.yml      # 生产环境全栈部署
└── .env.example                 # 环境变量模板
```

---

## API 概览

### 公开接口

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/register` | 注册 |
| POST | `/api/auth/login` | 登录（含验证码） |
| POST | `/api/auth/refresh` | 刷新 Token |
| GET | `/api/auth/captcha` | 获取图形验证码 |
| GET | `/api/currency/rates` | 查询系统汇率参考表 |
| GET | `/api/currency/convert` | 汇率换算 |
| GET | `/api/articles/:id/comments` | 查看评论 |
| GET | `/api/articles/:id/comments/count` | 评论计数 |

### 需认证接口（Header: `Authorization: Bearer <token>`）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/logout` | 登出 |
| GET | `/api/user/me` | 当前用户信息 |
| GET | `/api/articles?page=1&pageSize=20` | 文章分页列表 |
| GET | `/api/articles/hot` | 热门排行榜 |
| GET | `/api/articles/:id` | 文章详情 |
| POST | `/api/articles` | 发布文章 |
| PUT | `/api/articles/:id` | 编辑文章 |
| DELETE | `/api/articles/:id` | 删除文章 |
| POST | `/api/articles/:id/like` | 点赞 |
| POST | `/api/articles/:id/unlike` | 取消点赞 |
| GET | `/api/articles/:id/like` | 点赞状态 |
| POST | `/api/articles/:id/comments` | 发表评论 |
| DELETE | `/api/comments/:id` | 删除评论 |
| POST | `/api/rag/index/:id` | 索引文章到 RAG |
| POST | `/api/rag/ask` | RAG 智能问答 |

---

## 核心设计

### JWT 双 Token 机制

- **Access Token**：2 小时有效，用于 API 鉴权，存在前端 localStorage
- **Refresh Token**：7 天有效，用于静默续期，存在 localStorage
- 前端 axios 拦截器在收到 401 时自动调用 `/auth/refresh` 刷新
- 登出时将 Refresh Token 吊销并将当前 Access Token 的 JTI 加入 Redis 黑名单

### 点赞一致性

```
用户点赞 / 取消
    │
    ▼
Redis Lua：Bitmap 状态校验 + 计数器 + ZSet 排行 + List outbox（同一原子脚本）
    │
    ▼
Relay：BRPOPLPUSH 转入处理中列表 → RabbitMQ Publisher Confirm
    │                                                   │
    └──────── Confirm 后 LREM 确认 ─────────────────────┘
                                                        ▼
RabbitMQ 多消费者：按 (user_id, article_id) 唯一索引和 StateVersion 幂等写入 MySQL
                                                        │
                                                        ▼
定时批量同步 articles.likes_count，并失效文章详情缓存
```

点赞请求不再锁定热点文章行：Redis 是点赞状态、实时计数和排行榜的快速路径；RabbitMQ 负责削峰并异步持久化用户-文章关联。消费者只锁定同一个用户与同一篇文章的唯一关联，且只接受更大的 `StateVersion`，因此重复投递、消费者并发和乱序消息都不会改变最终状态。

Redis outbox 中的事件只有在 RabbitMQ 返回 Publisher Confirm 后才移出处理中列表；进程在这两步之间崩溃会造成重复投递，但版本幂等会将其处理为无副作用。该链路是“至少一次投递”，不是“恰好一次投递”。

每篇文章还维护待落库事件集合：当 Bitmap 或计数键局部丢失、但该文章仍有未落库事件时，接口会短暂返回恢复中，而不是使用落后的 MySQL 数据回填缓存。

RabbitMQ 在后端启动较晚或发生重启时，Relay、点赞消费者和 RAG 消费者会重试获取连接并重新声明所需队列；Redis outbox 中尚未确认的点赞事件会在连接恢复后继续投递。

生产部署需要为 Redis 开启 AOF 持久化并避免对 `like:v2:*` 键使用淘汰策略；否则 Redis 崩溃时，尚未投递 RabbitMQ 的 outbox 事件无法恢复。Redis 已配置但运行中不可用时，点赞与实时榜返回短暂不可用，避免把尚未落库的事件误显示为旧 MySQL 数据；只有明确未配置 Redis 时，系统才使用同步 MySQL 事务降级。

点赞队列使用 `likes_sync_queue_v2`，消息包含事件 ID 与状态版本。升级前不要把旧版 `likes_sync_queue` 的无版本消息直接转入 v2 队列；应先核对旧队列积压与 `likes` 表，再按有效关联重算历史 `articles.likes_count`。

### RAG 检索流程

```
用户提问
    │
    ▼
1. 宽召回：向量 chunk + BM25 文章（ES 故障时 MySQL FULLTEXT/LIKE）
    │
    ▼
2. RRF 融合为文章候选集
    │
    ▼
3. 在候选文章内二次向量检索精确 child chunk
    │
    ▼
4. 真实父子块回填、可选 rerank、去重与引用
    │
    ▼
5. 仅基于带文章编号和更新时间的上下文调用通义千问；无命中则直接返回无法确定
```

Elasticsearch 不可用时，系统保留向量召回，并以 MySQL FULLTEXT（不可用时 LIKE）生成关键词候选，不中断问答服务。
Embedding/Chroma 故障且关键词有命中时仍可使用文章原文片段；两路均不可用时返回 503，不伪造回答。LLM 故障同样返回 503。RAG 完整链路仍需配置可用的 DashScope Key、ChromaDB 与模型服务。

父块默认约 1,200 字、子块默认约 400 字。每个子块携带稳定的 `parent_id`，命中后仅把相应父段送入模型一次，避免重复上下文。`rag.indexSchemaVersion` 用于安全滚动升级索引格式；修改分块结构或嵌入元数据后应递增该值，后台同步任务会重新索引旧文章。

### RAG 评测

`back/testdata/rag_eval_cases.json` 提供 35 条人工标注问题：30 条可回答问题与 5 条应拒答的实时/未覆盖问题。评测会输出 Recall@K、引用正确率、拒答准确率、p50/p95 延迟，以及基于可选价格参数估算的成本。

在 Chroma、可选 ES 和 DashScope 配置均可用后执行：

```bash
cd back
go run ./cmd/rag-eval -out ../rag-eval-report.json
```

若需要成本估算，可传入当前模型的“每百万 token 人民币价格”；报告会明确标记为估算值：

```bash
go run ./cmd/rag-eval \
  -input-price-per-million-tokens 0 \
  -output-price-per-million-tokens 0 \
  -out ../rag-eval-report.json
```

评测样本的文章编号来自当前本地数据集；更换种子数据后，请先核对并更新 `expected_article_ids`，再将结果用于版本对比。

### 限流

基于 Redis 令牌桶算法，对登录、发文章、点赞和 RAG 问答/索引等敏感接口限流。默认不信任 `X-Forwarded-For`；部署在反向代理后需显式设置 `TRUSTED_PROXIES` 为代理的 IP/CIDR。未配置时登录限流会按代理地址聚合。

---

## License

ISC
