package main

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	// botProfileTTL 是机器人自身信息（GET /users/@me）的缓存时长。昵称与头像
	// 几乎不变，10 分钟足够让设置页「刷新一次就准」，又不会让每次打开卡片都打
	// 一次平台接口。
	botProfileTTL = 10 * time.Minute
	// botProfileFailureTTL 只缓存 1 分钟：凭据刚填对、或平台刚恢复时，运营者
	// 点一次「刷新」就该看到变化，而不是等十分钟。
	botProfileFailureTTL = time.Minute

	messageAccountCredentialsMissing = "尚未配置凭据（AppID / AppSecret）"
)

// BotProfile 是 GET /users/@me 的可读投影。AvatarURL 只保留平台返回的**完整**
// URL（官方文档与实测示例都是 https://thirdqq.qlogo.cn/…）；相对路径一律丢弃，
// 不去猜一个基址拼。
type BotProfile struct {
	ID        string
	Name      string
	AvatarURL string
}

// botProfileCache 是进程内的机器人信息缓存：成功缓存久、失败缓存短。
//
// 它只存公开信息（昵称/头像），不碰任何凭据；进程重启即清空。
type botProfileCache struct {
	// fetchMu 保证同一时刻只有一个调用方去问平台，避免设置页连点几次就打几次。
	fetchMu sync.Mutex

	mu        sync.Mutex
	profile   BotProfile
	reason    string
	expiresAt time.Time
	loaded    bool
	now       func() time.Time
}

func newBotProfileCache(now func() time.Time) *botProfileCache {
	if now == nil {
		now = time.Now
	}
	return &botProfileCache{now: now}
}

// lookup 返回缓存里的结论：profile / reason / 是否仍在有效期内。
// reason 非空表示上次查询失败，此时 profile 为零值。
func (c *botProfileCache) lookup() (BotProfile, string, bool) {
	if c == nil {
		return BotProfile{}, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loaded || !c.now().Before(c.expiresAt) {
		return BotProfile{}, "", false
	}
	return c.profile, c.reason, true
}

func (c *botProfileCache) store(profile BotProfile, reason string) {
	if c == nil {
		return
	}
	ttl := botProfileTTL
	if reason != "" {
		ttl = botProfileFailureTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profile = profile
	c.reason = reason
	c.expiresAt = c.now().Add(ttl)
	c.loaded = true
}

// accountBot 返回机器人自身信息（可能来自缓存）。reason 非空表示拿不到，
// 调用方据此把 available 置 false——它永远不返回 error，状态卡片不该因为
// 平台抖动而整块失败。
func (p *plugin) accountBot(ctx context.Context) (BotProfile, string) {
	if p == nil || p.client == nil || !p.client.configured() {
		return BotProfile{}, messageAccountCredentialsMissing
	}
	cache := p.botProfile
	if cache == nil {
		// 没有缓存实例（理论上不会发生）：直接查一次，不写缓存。
		profile, err := p.client.BotProfile(ctx)
		if err != nil {
			return BotProfile{}, adminReason(err)
		}
		return profile, ""
	}
	if profile, reason, ok := cache.lookup(); ok {
		return profile, reason
	}

	cache.fetchMu.Lock()
	defer cache.fetchMu.Unlock()
	// 等锁期间别的调用方可能已经查完了。
	if profile, reason, ok := cache.lookup(); ok {
		return profile, reason
	}
	profile, err := p.client.BotProfile(ctx)
	if err != nil {
		reason := adminReason(err)
		cache.store(BotProfile{}, reason)
		return BotProfile{}, reason
	}
	cache.store(profile, "")
	return profile, ""
}

// accountGateway 把网关的进程内状态摊成卡片字段，**不发任何请求**。
// since 由 ConnectedFor 反推（连接中/失败态没有起点，给 null）。
func accountGateway(status GatewayStatus, now time.Time) map[string]any {
	var since any
	if status.State == gatewayStateConnected && status.ConnectedFor > 0 {
		since = now.Add(-status.ConnectedFor).UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"state":  status.State,
		"reason": status.LastError,
		"since":  since,
	}
}

// countUniqueGroups 数群白名单里的有效条目（去空白、去重），与设置页
// 「目标群 N 个」同口径。
func countUniqueGroups(groups []string) int {
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		seen[group] = struct{}{}
	}
	return len(seen)
}
