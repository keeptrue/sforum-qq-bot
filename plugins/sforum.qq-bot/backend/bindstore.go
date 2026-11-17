package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// bindTaskTTL 与二维码有效期一致：超过 10 分钟的绑定任务直接当作过期，
	// 不留在内存里等下一次轮询。
	bindTaskTTL = 10 * time.Minute
	// bindStartPerMinute 是每进程每分钟可新建的绑定任务数。它挡的是刷新风暴与
	// 脚本刷二维码，不是正常操作：一次扫码只需要一个任务。
	bindStartPerMinute = 5
	// bindStoreMaxEntries 是任务表硬上限。TTL + 限流下正常不会摸到，存在只是为了
	// 让一张被灌满的表也能收敛。
	bindStoreMaxEntries = 64
)

// bindTaskEntry 是一条绑定任务：key 是解密密钥的 base64，只在内存里存在。
type bindTaskEntry struct {
	key       string
	createdAt time.Time
}

// BindStore 是扫码绑定的任务表。它是进程内状态：重启即清空，不落盘、不进日志。
//
// 语义上分两步——轮询可以反复读（前端每 1.5 秒一次），但凭据只允许成功交付一次，
// 所以读用 Key、成功用 Take（取走即删）。
type BindStore struct {
	mu    sync.Mutex
	tasks map[string]bindTaskEntry
	ttl   time.Duration
	now   func() time.Time
	start tokenBucket
}

func NewBindStore(now func() time.Time) *BindStore {
	if now == nil {
		now = time.Now
	}
	return &BindStore{
		tasks: make(map[string]bindTaskEntry, 8),
		ttl:   bindTaskTTL,
		now:   now,
		start: newTokenBucket(bindStartPerMinute, now()),
	}
}

// TTL 是任务有效期，路由用它回报 expiresInSeconds。
func (s *BindStore) TTL() time.Duration {
	if s == nil || s.ttl <= 0 {
		return bindTaskTTL
	}
	return s.ttl
}

// AllowStart 领取一次新建任务的配额；返回 0 表示放行，否则是需要等待的时长。
func (s *BindStore) AllowStart() time.Duration {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	now := s.now()
	s.mu.Unlock()
	return s.start.take(now)
}

// RefundStart 归还一次没用上的配额：限流挡的是「建任务」这个动作，上游失败
// （网络、retcode、限额）不该消耗运营者的额度。
func (s *BindStore) RefundStart() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start.refund()
}

// Put 记下一次绑定任务，并顺手清掉过期项。
func (s *BindStore) Put(taskID, key string) {
	taskID = strings.TrimSpace(taskID)
	if s == nil || taskID == "" || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.pruneLocked(now)
	s.tasks[taskID] = bindTaskEntry{key: key, createdAt: now}
	if len(s.tasks) > bindStoreMaxEntries {
		s.trimOldestLocked(len(s.tasks) - bindStoreMaxEntries)
	}
}

// Key 只读地取任务密钥，供反复轮询使用；任务不存在或已过期返回 false。
func (s *BindStore) Key(taskID string) (string, bool) {
	taskID = strings.TrimSpace(taskID)
	if s == nil || taskID == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tasks[taskID]
	if !ok {
		return "", false
	}
	if s.expiredLocked(entry, s.now()) {
		delete(s.tasks, taskID)
		return "", false
	}
	return entry.key, true
}

// Take 取走任务密钥：凭据只允许交付一次，成功后立即删除。
func (s *BindStore) Take(taskID string) (string, bool) {
	taskID = strings.TrimSpace(taskID)
	if s == nil || taskID == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tasks[taskID]
	if !ok {
		return "", false
	}
	delete(s.tasks, taskID)
	if s.expiredLocked(entry, s.now()) {
		return "", false
	}
	return entry.key, true
}

// Delete 丢弃一个任务（轮询判定过期时用）。
func (s *BindStore) Delete(taskID string) {
	taskID = strings.TrimSpace(taskID)
	if s == nil || taskID == "" {
		return
	}
	s.mu.Lock()
	delete(s.tasks, taskID)
	s.mu.Unlock()
}

// Len 返回当前任务数，供测试与状态排查使用。
func (s *BindStore) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

func (s *BindStore) expiredLocked(entry bindTaskEntry, now time.Time) bool {
	ttl := s.ttl
	if ttl <= 0 {
		ttl = bindTaskTTL
	}
	return now.Sub(entry.createdAt) > ttl
}

// pruneLocked 清掉全部过期任务。调用方必须持有 s.mu。
func (s *BindStore) pruneLocked(now time.Time) {
	for taskID, entry := range s.tasks {
		if s.expiredLocked(entry, now) {
			delete(s.tasks, taskID)
		}
	}
}

// trimOldestLocked 在超出硬上限时按创建时间丢最旧的若干条。
func (s *BindStore) trimOldestLocked(extra int) {
	if extra <= 0 {
		return
	}
	type candidate struct {
		taskID    string
		createdAt time.Time
	}
	entries := make([]candidate, 0, len(s.tasks))
	for taskID, entry := range s.tasks {
		entries = append(entries, candidate{taskID: taskID, createdAt: entry.createdAt})
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].createdAt.Before(entries[right].createdAt)
	})
	for _, entry := range entries {
		if extra <= 0 {
			return
		}
		delete(s.tasks, entry.taskID)
		extra--
	}
}
