package main

import (
	"sync"
	"testing"
	"time"
)

// fakeClock 是任务表的可控时钟：TTL 与限流都按它走，测试不睡觉。
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

// 任务表：写入后可反复读，Take 取走即删。
func TestBindStoreOneTimeTake(t *testing.T) {
	store := NewBindStore(newFakeClock().Now)
	store.Put("task-1", "key-1")

	if key, ok := store.Key("task-1"); !ok || key != "key-1" {
		t.Fatalf("读任务失败：%q / %v", key, ok)
	}
	// 轮询可以反复读，凭据还没交付。
	if key, ok := store.Key("task-1"); !ok || key != "key-1" {
		t.Fatalf("重复读取失败：%q / %v", key, ok)
	}
	if key, ok := store.Take("task-1"); !ok || key != "key-1" {
		t.Fatalf("取走任务失败：%q / %v", key, ok)
	}
	// secret 只交付一次：再取就是过期。
	if _, ok := store.Take("task-1"); ok {
		t.Fatal("任务被取走两次")
	}
	if _, ok := store.Key("task-1"); ok {
		t.Fatal("取走后仍能读到任务")
	}
	if store.Len() != 0 {
		t.Fatalf("任务表应已清空，实际 %d 条", store.Len())
	}
}

// TTL：超过 10 分钟即过期，读与取都应落空。
func TestBindStoreTTL(t *testing.T) {
	clock := newFakeClock()
	store := NewBindStore(clock.Now)
	if store.TTL() != bindTaskTTL {
		t.Fatalf("默认 TTL 期望 %s，得到 %s", bindTaskTTL, store.TTL())
	}
	store.Put("task-1", "key-1")
	clock.Advance(bindTaskTTL + time.Second)

	if _, ok := store.Key("task-1"); ok {
		t.Fatal("过期任务不该读得到")
	}
	if store.Len() != 0 {
		t.Fatalf("过期任务应被顺手清掉，实际 %d 条", store.Len())
	}

	store.Put("task-2", "key-2")
	clock.Advance(bindTaskTTL + time.Second)
	if _, ok := store.Take("task-2"); ok {
		t.Fatal("过期任务不该取得到")
	}
}

// 清理：Put 会顺带清掉过期项，任务表不会无界增长。
func TestBindStorePruneOnPut(t *testing.T) {
	clock := newFakeClock()
	store := NewBindStore(clock.Now)
	for index := 0; index < 5; index++ {
		store.Put("old-"+string(rune('a'+index)), "key")
	}
	clock.Advance(bindTaskTTL + time.Second)
	store.Put("fresh", "key")
	if store.Len() != 1 {
		t.Fatalf("清理后应只剩 1 条，实际 %d 条", store.Len())
	}
	if _, ok := store.Key("fresh"); !ok {
		t.Fatal("新任务不该被清掉")
	}
}

// 硬上限：被灌满时丢最旧的，而不是无限膨胀。
func TestBindStoreTrimOldest(t *testing.T) {
	clock := newFakeClock()
	store := NewBindStore(clock.Now)
	for index := 0; index < bindStoreMaxEntries+5; index++ {
		store.Put("task-"+string(rune('a'+index%26))+string(rune('a'+index/26)), "key")
		clock.Advance(time.Millisecond)
	}
	if store.Len() > bindStoreMaxEntries {
		t.Fatalf("任务表超出上限：%d > %d", store.Len(), bindStoreMaxEntries)
	}
}

// 限流：每分钟 5 次，第 6 次给等待时长；时间推进后恢复。
func TestBindStoreRateLimit(t *testing.T) {
	clock := newFakeClock()
	store := NewBindStore(clock.Now)
	for attempt := 0; attempt < bindStartPerMinute; attempt++ {
		if wait := store.AllowStart(); wait != 0 {
			t.Fatalf("第 %d 次不该被限流，等待 %s", attempt+1, wait)
		}
	}
	wait := store.AllowStart()
	if wait <= 0 {
		t.Fatal("超过每分钟配额必须给出等待时长")
	}
	if wait > time.Minute {
		t.Fatalf("等待时长不合理：%s", wait)
	}
	// 回填一个令牌（5/分钟 → 12 秒一个）。
	clock.Advance(12 * time.Second)
	if wait := store.AllowStart(); wait != 0 {
		t.Fatalf("回填后应放行，等待 %s", wait)
	}
}

// Delete：轮询判定过期时直接把任务丢掉。
func TestBindStoreDelete(t *testing.T) {
	store := NewBindStore(newFakeClock().Now)
	store.Put("task-1", "key-1")
	store.Delete("task-1")
	if _, ok := store.Key("task-1"); ok {
		t.Fatal("删除后仍能读到任务")
	}
	// 空 id 与 nil 表都不该 panic。
	store.Put("", "key")
	store.Delete("")
	var nilStore *BindStore
	nilStore.Put("task-1", "key")
	if _, ok := nilStore.Key("task-1"); ok {
		t.Fatal("nil 任务表不该有数据")
	}
	if nilStore.Len() != 0 {
		t.Fatal("nil 任务表的长度应为 0")
	}
	if wait := nilStore.AllowStart(); wait != 0 {
		t.Fatalf("nil 任务表不该限流，得到 %s", wait)
	}
}

// 并发读写任务表不应有数据竞争（配合 -race 更有价值）。
func TestBindStoreConcurrentAccess(t *testing.T) {
	store := NewBindStore(newFakeClock().Now)
	var waitGroup sync.WaitGroup
	for index := 0; index < 16; index++ {
		waitGroup.Add(1)
		go func(worker int) {
			defer waitGroup.Done()
			taskID := "task-" + string(rune('a'+worker))
			store.Put(taskID, "key")
			store.Key(taskID)
			store.AllowStart()
			store.Take(taskID)
			store.Len()
		}(index)
	}
	waitGroup.Wait()
	if store.Len() != 0 {
		t.Fatalf("并发读写后任务表应为空，实际 %d 条", store.Len())
	}
}

// 失败回滚：先占位、成功后扣。上游失败不该消耗运营者的配额。
func TestBindStoreRefundStart(t *testing.T) {
	store := NewBindStore(newFakeClock().Now)
	for attempt := 0; attempt < bindStartPerMinute*3; attempt++ {
		if wait := store.AllowStart(); wait != 0 {
			t.Fatalf("第 %d 次不该被限流，等待 %s", attempt+1, wait)
		}
		store.RefundStart()
	}
	// 回滚不该把令牌加爆：连续取 5 次之后第 6 次仍应被限流。
	for attempt := 0; attempt < bindStartPerMinute; attempt++ {
		if wait := store.AllowStart(); wait != 0 {
			t.Fatalf("第 %d 次不该被限流，等待 %s", attempt+1, wait)
		}
	}
	if wait := store.AllowStart(); wait <= 0 {
		t.Fatal("回滚后配额仍应封顶在每分钟 5 次")
	}
	var nilStore *BindStore
	nilStore.RefundStart()
}
