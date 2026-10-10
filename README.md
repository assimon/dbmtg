# dbmtg

**D**on't **B**other **M**y **T**ele**G**ram —— 别打扰我的 Telegram。

Telegram 私聊里的广告、诈骗和骚扰越来越多，而图片验证码、算术题在 AI 面前已经形同虚设。dbmtg 是一个自托管的双向沟通机器人：把机器人作为陌生人联系你的**唯一入口**，对方先通过 Cloudflare Turnstile 或 [Cap](https://github.com/tiagozip/cap) 人机验证，消息才会送到你的管理群；每个访客一个独立话题，你在话题里直接回复，对方看不到你的真实账号。

```
陌生人 ──私聊──▶ 机器人 ──(须知 / 人机验证 / 防刷屏)──▶ 管理群
                                                    └─ 每个访客一个话题，置顶资料卡
你在话题里回复 ──▶ 机器人 ──▶ 陌生人
```

## 功能

**访客侧**
- 全消息类型双向转发（`copyMessage`，不暴露"转发自"），引用回复双向对齐，编辑消息双向同步
- 人机验证：Turnstile / Cap / 关闭，通过 Telegram Mini App 完成，验证与 TG 账号绑定（initData 签名校验），无法代验
- 须知：`/start` 时发送管理员配置的须知（支持格式、图片），可要求点击「我已阅读」后才能发消息
- 验证 / 确认前发送的消息会暂存，完成后自动送达
- 送达回执：消息转发成功后在原消息上加 👌

**管理侧**
- 每个访客一个话题，置顶资料卡：头像、名字、@username、ID、语言、简介、生日、个人频道、验证状态；
  按钮一键查看资料页、刷新、拉黑 / 解除、放行；访客改名后话题标题和资料卡自动更新
- 未回复上限：管理员回复前访客最多发 N 条（相册算一条），超出直接丢弃并提示双方
- 撤回：在话题里回复已发出的消息并发送 `/del`
- 离开模式：自动回复一段说明，同一访客按间隔只回复一次
- 多管理员：管理群成员都能回复；`admin_ids` 中的超级管理员可私聊机器人修改设置
- 长轮询 / Webhook 两种模式，SQLite 单文件存储，单二进制 / Docker 部署

## 部署

### 1. 准备机器人和管理群

1. 找 [@BotFather](https://t.me/BotFather) 创建机器人，拿到 token。
2. 新建一个群，在群设置里开启 **话题（Topics）**，把机器人拉进群并设为管理员，勾选 **管理话题** 和 **置顶消息** 权限。

### 2. 启动

**Docker Compose（推荐）**

```bash
git clone https://github.com/assimon/dbmtg.git && cd dbmtg
cp config.example.yaml config.yaml   # 先只填 bot.token
docker compose up -d
docker compose logs -f
```

> 一定要先创建 `config.yaml` 再启动，否则 Docker 会把它当成目录挂载。数据库保存在 `./data`。
>
> compose 默认不占用宿主机端口：长轮询模式只需要出站访问 Telegram，验证页由同一网络里的 Cloudflare Tunnel 访问。

**源码运行**（Go 1.26+）

```bash
cp config.example.yaml config.yaml
go build -o dbmtg ./cmd/dbmtg && ./dbmtg -c config.yaml
```

### 3. 填写管理群和管理员 ID

只填了 token 时机器人处于**初始化模式**：

- 私聊机器人发任意消息，得到你的用户 ID，填入 `bot.admin_ids`；
- 在管理群里发 `/id`，得到群 ID（形如 `-100xxxxxxxxxx`），填入 `bot.admin_group_id`。

然后重启（`docker compose restart`）。到这里已经可以正常收发消息了，最后把机器人链接放到你的个人简介里，作为陌生人联系你的入口。

### 4. 开启人机验证（可选，推荐）

验证页由内置 Web 服务提供，需要 **HTTPS 公网地址**（Telegram Mini App 的要求），填到 `web.public_url`。

- **Turnstile**：在 Cloudflare 控制台创建 Widget，主机名填 `public_url` 的域名，把 site key / secret 填入配置。
- **Cap**：自建 Cap Standalone，创建站点密钥，填入 `captcha.cap.url / site_key / secret`。

重启后私聊机器人发 `/settings`，点"切换验证方式"即可启用。

**HTTPS 从哪来？**

- 有服务器和域名：在 `docker-compose.yml` 里取消 `ports` 的注释（端口冲突时改冒号左边的端口），再用 Caddy / Nginx 反代过去。
- 没有公网 IP（家里的 NAS、旧电脑、树莓派）：用 **Cloudflare Tunnel**，不需要公网 IP，也不用配证书。
  1. 在 Cloudflare Zero Trust 创建一个 Tunnel，复制 token；
  2. 在 Tunnel 里添加 Public Hostname，例如 `bot.example.com`，服务地址填 `http://dbmtg:8080`（容器间内网访问，不需要暴露端口）；
  3. 把 token 写进项目目录的 `.env`，带上 tunnel 启动：
     ```bash
     echo "TUNNEL_TOKEN=你的token" > .env
     docker compose --profile tunnel up -d
     ```
  4. `public_url` 设为 `https://bot.example.com`。

  不用 Docker 时也可以直接运行 `cloudflared tunnel run --token <token>`，服务地址改为 `http://localhost:8080`。

## 命令

| 位置 | 命令 | 说明 |
|---|---|---|
| 话题内（群成员） | `/info` | 用户资料与统计 |
| | `/ban` / `/unban` | 拉黑 / 解除 |
| | `/del` | 回复一条已发出的消息，撤回它 |
| 私聊（超级管理员） | `/settings` | 设置面板：验证方式、须知确认、送达回执、回复署名、离开模式 |
| | `/stats` | 统计 |
| | `/welcome` | 预览须知 |
| | `/setwelcome` | 设置须知：回复一条消息发送（保留格式、图片），或直接跟文字 |
| | `/setaway` | 设置离开回复，用法同上 |
| | `/setawayhours <小时>` | 离开回复对同一访客的间隔 |
| | `/setmaxunreplied <条数>` | 管理员回复前访客最多可发几条，0 为不限 |
| | `/setttl <天数>` | 验证有效期，0 为永久 |
| | `/setmaxfail <次数>` | 验证失败多少次自动拉黑 |
| | `/help` | 命令帮助 |

> 用"回复一条消息"设置的须知 / 离开回复会引用原消息，请勿删除，否则会退回为纯文字。

## 配置

完整配置见 [`config.example.yaml`](config.example.yaml)。所有配置项都可以用环境变量覆盖，前缀 `DBMTG_`，层级用下划线连接，如 `DBMTG_BOT_TOKEN`、`DBMTG_WEB_PUBLIC_URL`。

运行时的开关（验证方式、须知、未回复上限、离开模式等）存在数据库里，通过机器人的 `/settings` 和命令修改，无需重启。

## 其它说明

- 机器人被陌生人拉进管理群以外的群会自动退出。
- 更换 `admin_group_id` 后，旧的话题绑定会被清空，访客下次发消息时在新群自动建话题。
- 管理员关闭话题后，访客再发消息时会自动重新打开。

## 已知限制

- 相册（多图）会被拆成单条消息转发
- 编辑同步只同步文字和说明文字，替换图片 / 文件本身不同步
- 资料卡头像只在创建话题时获取
- 超过 48 小时的消息无法撤回（Telegram 限制）

## License

[MIT](LICENSE)
