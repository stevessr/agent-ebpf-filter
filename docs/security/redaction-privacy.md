# 脱敏与隐私

数据脱敏是项目安全模型的一部分，不是后处理补丁。

## | 级别 | 用途 |
| --- | --- |
| None | 开发环境，不推荐生产 |
| Basic | 明显密码 / token |
| Standard | 默认推荐，覆盖常见敏感字段 |
| Strict | 高安全要求 / 合规审计 |

## - path：用户目录、配置目录；
- command args：password、token、api_key、bearer；
- network：私网 IP、内部域名；
- HTTP headers：Authorization、Cookie、X-API-KEY；
- URL query：token/key/secret/password；
- JSON / form / text body：常见密钥模式；
- PEM / SSH key / AWS credential / JWT / Bearer token。

## `pb.Event` 中的隐私相关字段：

- `argv_digest`
- `redaction_level`
- `sanitized_fields`

TLS / Codex capture payload 还应携带：

- redaction state；
- body truncated 标志；
- digest；
- length。

## TLS capture 边界

TLS 明文捕获默认关闭。启用时也必须：

- 经过 auth；
- 经过 runtime gate；
- 统一 redaction；
- 限制 body 大小；
- 普通事件只带 metadata / digest。

## - `backend/redaction/README.md`
- `docs/security/sanitization.md`
- `docs/security/sanitization-user-guide.md`
- `docs/_archive/SANITIZATION_IMPLEMENTATION_SUMMARY.md`

---

## - [安全模型](model.md)
- [Runtime Gates 与 Auth](runtime-gates-auth.md)
- [Sanitization 完整文档](sanitization.md)
- [Sanitization 中文指南](sanitization-user-guide.md)
- [事件管线](../backend/event-pipeline.md)
