# dbmtg

自托管的 Telegram 双向沟通机器人。陌生人只能通过 Bot 联系你，每个用户的消息自动分流到管理群里的独立话题（Topic），在话题里直接回复即可。支持 Cloudflare Turnstile / [Cap](https://github.com/tiagozip/cap) 人机验证，挡住骚扰和广告机器人。

## 功能

- 全消息类型双向转发（`copyMessage`，不暴露"转发自"），引用回复双向对齐
- 每个用户一个话题，话题被删后自动重建
- 人机验证：Turnstile / Cap / 关闭，通过 Telegram Mini App 完成，验证与 TG 账号绑定（initData 签名校验），无法代验
- 验证前发送的消息会暂存，验证通过后自动送达
- 多管理员：管理群成员都能回复；`admin_ids` 中的超级管理员可私聊机器人修改设置
- 长轮询 / Webhook 两种模式，SQLite 单文件存储，单二进制部署

## 快速开始

1. 找 [@BotFather](https://t.me/BotFather) 创建机器人，拿到 token。
2. 新建一个群，在群设置里开启 **话题（Topics）**，把机器人拉进群并设为管理员，勾选 **管理话题** 权限。
3. 获取群 ID（形如 `-100xxxxxxxxxx`）和你自己的用户 ID。
4. 复制配置并填写：
   ```bash
   cp config.example.yaml config.yaml
   ```
5. 运行：
   ```bash
   go build -o dbmtg ./cmd/dbmtg && ./dbmtg -c config.yaml
   ```
   或 Docker：
   ```bash
   docker build -t dbmtg .
   docker run -d --name dbmtg --restart unless-stopped \
     -v $PWD/config.yaml:/app/config.yaml -v $PWD/data:/app/data \
     -p 8080:8080 dbmtg
   ```

## 人机验证

验证页由内置 Web 服务提供，需要 **HTTPS 公网地址**（Telegram Mini App 的要求），填到 `web.public_url`。

- **Turnstile**：在 Cloudflare 控制台创建 Widget，主机名填 `public_url` 的域名，把 site key / secret 填入配置。
- **Cap**：自建 Cap Standalone，创建站点密钥，填入 `captcha.cap.url / site_key / secret`。

配置好后私聊机器人发 `/settings`，点"切换验证方式"即可启用。

### 没有公网服务器？用 Cloudflare Tunnel

家里的 NAS、旧电脑、树莓派都能跑。域名托管在 Cloudflare 后：

```bash
cloudflared tunnel login
cloudflared tunnel create dbmtg
cloudflared tunnel route dns dbmtg bot.example.com
cloudflared tunnel run --url http://localhost:8080 dbmtg
```

然后把 `public_url` 设为 `https://bot.example.com`。不需要公网 IP，也不用配证书。

## 命令

| 位置 | 命令 | 说明 |
|---|---|---|
| 话题内（群成员） | `/info` | 用户资料 |
| | `/ban` / `/unban` | 拉黑 / 解除 |
| 私聊（超级管理员） | `/settings` | 查看设置，切换验证方式、回复署名 |
| | `/stats` | 统计 |
| | `/setwelcome <内容>` | 欢迎语 |
| | `/setttl <天数>` | 验证有效期，0 为永久 |
| | `/setmaxfail <次数>` | 验证失败多少次自动拉黑 |
| | `/help` | 命令帮助 |

## 已知限制

- 相册（多图）会被拆成单条消息转发
- 编辑消息暂不同步

## License

MIT
