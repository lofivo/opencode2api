# 上游访问

描述网关访问 OpenCode 上游时使用的通道与会话概念。

## Language

**匿名通道（Anonymous lane）**：
使用共享公共凭证访问免费模型的上游通道；不需要用户提供上游 Key。
_Avoid_: 无认证通道

**密钥通道（Key tier）**：
使用用户配置的 Zen 或 Go Key 访问模型的上游通道。

**请求形状通道（v1 / v2 lane）**：
按上游要求的 OpenCode 客户端代际区分的请求约定；与是否使用匿名凭证是两个不同维度。
_Avoid_: API v1 / API v2

**会话亲和性（Session affinity）**：
同一对话持续关联同一上游会话及优先节点的关系；子会话可以拥有独立身份并共享父会话的亲和性。

**匿名会话恢复（Anonymous session recovery）**：
匿名访问被拒绝后，以新的上游会话身份重试原请求的恢复过程；它不等于切换模型。
