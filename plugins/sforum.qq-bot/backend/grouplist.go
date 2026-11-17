package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	// groupsListLookupLimit 是单次列表请求最多查多少个群名。官方群信息接口只有
	// 30 QPM，一次刷新不该把配额吃干；其余群先给 openid + nameUnavailable。
	groupsListLookupLimit = 10
	// groupsListLookupWorkers 是并发度。串行查 10 个群名会把路由拖到超过宿主
	// 超时（15 秒），并发 3 既压住瞬时 QPS，也能在预算内查完。
	groupsListLookupWorkers = 3
	// groupsNameBudget 是整次列表请求花在群名上的总预算：超时就放弃剩余查询，
	// 让接口先返回「有 openid、暂时没名字」的列表，而不是整体超时。
	groupsNameBudget = 4 * time.Second
	// 成功结果缓存一小时：群名不常改。
	groupsNameSuccessTTL = time.Hour
	// 11253（应用无接口访问权限）与其它确定性业务错误缓存十分钟：白名单批下来
	// 之前问多少次都是同一句「没有权限」，别把 30 QPM 花在重复确认上。
	groupsNameDeniedTTL = 10 * time.Minute
	// 瞬时失败（本地预算超时、ctx 取消、5xx、传输层）只缓存几十秒：一次慢请求
	// 不该让这些群名十分钟不再重查。
	groupsNameTransientTTL = 45 * time.Second
	// groupsNameLookupPerMinute 与官方 30 QPM 对齐，进程内共享一份配额。
	groupsNameLookupPerMinute = 30
)

// groupNameEntry 是一条群名缓存：name 为空表示「上次问过，没拿到」。
type groupNameEntry struct {
	name      string
	expiresAt time.Time
}

// groupNameCache 是群名的尽力而为缓存。它只存群名，不存任何凭据；进程重启即清空。
//
// 两层保护：缓存挡掉重复查询，令牌桶挡掉刷新风暴。两者都失效时也只会让名字
// 暂时缺失，不会让列表接口失败。
//
// 缓存键只有 openid：同一进程里 AppID 不会变（设置只在启动时读一次，保存即重启
// 插件），所以不必带 AppID。将来若做设置热更新，键必须改成 AppID+openid，否则
// 换机器人之后会读到上一个机器人的群名。
type groupNameCache struct {
	mu      sync.Mutex
	entries map[string]groupNameEntry
	budget  tokenBucket
	now     func() time.Time
}

func newGroupNameCache(now func() time.Time) *groupNameCache {
	if now == nil {
		now = time.Now
	}
	return &groupNameCache{
		entries: make(map[string]groupNameEntry, 8),
		budget:  newTokenBucket(groupsNameLookupPerMinute, now()),
		now:     now,
	}
}

// lookup 返回缓存里的群名，以及「这个答案是否还在有效期内」。cached 为 true 而
// name 为空，表示上次查询失败且还在失败 TTL 内——不要再问一次。
func (c *groupNameCache) lookup(openID string) (name string, cached bool) {
	if c == nil || openID == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[openID]
	if !ok {
		return "", false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, openID)
		return "", false
	}
	return entry.name, true
}

// storeName 记下一次成功查询。
func (c *groupNameCache) storeName(openID, name string) {
	c.store(openID, name, groupsNameSuccessTTL)
}

// storeFailure 记下一次失败，TTL 由失败性质决定（见 groupsNameFailureTTL）：
// 确定性的「没权限」缓存久一点，瞬时失败只缓存几十秒，下一轮刷新就能自愈。
func (c *groupNameCache) storeFailure(openID string, err error) {
	c.store(openID, "", groupsNameFailureTTL(err))
}

func (c *groupNameCache) store(openID, name string, ttl time.Duration) {
	if c == nil || openID == "" {
		return
	}
	if ttl <= 0 {
		ttl = groupsNameTransientTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[openID] = groupNameEntry{name: name, expiresAt: c.now().Add(ttl)}
}

// groupsNameFailureTTL 按失败性质给缓存时长：
//   - 11253 是确定性的「应用无接口访问权限」，批下来之前结果不会变 → 缓存久；
//   - 瞬时失败（Code==0 的传输层/超时/取消、Transient）→ 只缓存几十秒；
//   - 其余业务错误（凭据无效、群不存在等）同样是确定性结论 → 缓存久。
func groupsNameFailureTTL(err error) time.Duration {
	var qqError *QQError
	if errors.As(err, &qqError) {
		if qqError.Code == 11253 {
			return groupsNameDeniedTTL
		}
		if qqError.Code == 0 || qqError.Transient {
			return groupsNameTransientTTL
		}
	}
	return groupsNameDeniedTTL
}

// allow 领一次查询配额；返回 0 表示放行，否则表示还要等多久。
func (c *groupNameCache) allow() time.Duration {
	if c == nil {
		return 0
	}
	return c.budget.take(c.now())
}

// resolveGroupNames 尽力而为地补群名：缓存命中直接用，未命中且还有配额与预算
// 的才真的去问平台。任何失败都只意味着「这个名字暂时拿不到」，绝不冒泡成路由失败。
func (p *plugin) resolveGroupNames(ctx context.Context, openIDs []string) map[string]string {
	names := make(map[string]string, len(openIDs))
	cache := p.groupNames
	client := p.client
	if cache == nil || client == nil || !client.configured() || len(openIDs) == 0 {
		return names
	}

	pending := make([]string, 0, groupsListLookupLimit)
	for _, openID := range openIDs {
		if name, cached := cache.lookup(openID); cached {
			if name != "" {
				names[openID] = name
			}
			continue
		}
		if len(pending) >= groupsListLookupLimit {
			continue
		}
		if wait := cache.allow(); wait > 0 {
			continue
		}
		pending = append(pending, openID)
	}
	if len(pending) == 0 {
		return names
	}

	if ctx == nil {
		ctx = context.Background()
	}
	budgetCtx, cancel := context.WithTimeout(ctx, groupsNameBudget)
	defer cancel()

	var (
		namesMu    sync.Mutex
		failures   int
		lastReason string
	)
	jobs := make(chan string)
	var waitGroup sync.WaitGroup
	workers := groupsListLookupWorkers
	if workers > len(pending) {
		workers = len(pending)
	}
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for openID := range jobs {
				name, err := client.GroupName(budgetCtx, openID)
				if err != nil {
					namesMu.Lock()
					failures++
					lastReason = adminReason(err)
					namesMu.Unlock()
					cache.storeFailure(openID, err)
					continue
				}
				cache.storeName(openID, name)
				namesMu.Lock()
				names[openID] = name
				namesMu.Unlock()
			}
		}()
	}
	for _, openID := range pending {
		select {
		case jobs <- openID:
		case <-budgetCtx.Done():
		}
	}
	close(jobs)
	waitGroup.Wait()

	if failures > 0 && p.log != nil {
		// 只打一行汇总：群名缺失是预期状态（非白名单应用必然如此），不是事故。
		p.log.Warnf("qq-bot: %d 个群名查询失败，列表按无名字返回：%s", failures, lastReason)
	}
	return names
}
