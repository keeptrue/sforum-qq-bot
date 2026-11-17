package main

import (
	"context"
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
	// 成功结果缓存一小时（群名不常改）；失败只缓存十分钟——白名单批下来之后
	// 十分钟内就能自愈，而不是把「没权限」钉死一整天。
	groupsNameSuccessTTL = time.Hour
	groupsNameFailureTTL = 10 * time.Minute
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

// store 记下结果：name 为空时按失败 TTL 缓存，避免每次刷新都重打一遍 11253。
func (c *groupNameCache) store(openID, name string) {
	if c == nil || openID == "" {
		return
	}
	ttl := groupsNameSuccessTTL
	if name == "" {
		ttl = groupsNameFailureTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[openID] = groupNameEntry{name: name, expiresAt: c.now().Add(ttl)}
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
					// 失败也进缓存：非白名单应用每次都会拿到同一个 11253。
					name = ""
					namesMu.Lock()
					failures++
					lastReason = adminReason(err)
					namesMu.Unlock()
				}
				cache.store(openID, name)
				if name == "" {
					continue
				}
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
