package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	// queueCapacity 是内存队列上限：满了丢最旧的一条，宁可丢一条旧主题也不阻塞
	// 论坛侧的事件投递。
	queueCapacity = 200
	// dedupeTTL 是同一投递键的抑制窗口。
	dedupeTTL = time.Hour
	// dedupePurgeThreshold 触发一次过期清理，避免长跑进程里 map 无界增长。
	dedupePurgeThreshold = 512
)

// 过滤/跳过原因。它们会出现在日志与投递记录里，因此是稳定的小写标识。
const (
	skipPaused             = "paused"
	skipCredentialsMissing = "credentials_missing"
	skipWhitelistEmpty     = "whitelist_empty"
	skipCategoryFiltered   = "category_filtered"
	skipDuplicate          = "duplicate"
	reasonQuotaExceeded    = "quota_exceeded"
)

// Sender 是一次投递所需的最小能力面。生产实现是 *QQClient，测试注入假实现。
type Sender interface {
	SendGroupMessage(ctx context.Context, groupOpenID, content, format string) (SendResult, error)
}

// DeliveryJob 是队列里的一次推送：一条已渲染的消息发给若干群。
type DeliveryJob struct {
	TopicTitle string
	Content    string
	Groups     []string
	Truncated  bool
}

// Dispatcher 负责过滤、去重、限速、配额、排队与重试。它只有一个 worker：QQ 侧
// 的推送天然是串行的，多开只会更容易撞频控。
type Dispatcher struct {
	cfg    Config
	sender Sender
	state  *State
	log    *logger

	queue     chan DeliveryJob
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	enqueueMu sync.Mutex

	mu     sync.Mutex
	dedupe map[string]time.Time
	daily  map[string]dayCounter

	bucket tokenBucket

	now         func() time.Time
	sleep       func(ctx context.Context, duration time.Duration) bool
	backoffBase time.Duration
}

type dayCounter struct {
	day   string
	count int
}

// NewDispatcher 构造调度器。worker 由 Start 启动，测试也可以只调 Admit/Enqueue。
func NewDispatcher(cfg Config, sender Sender, state *State, log *logger) *Dispatcher {
	now := time.Now
	dispatcher := &Dispatcher{
		cfg:         cfg,
		sender:      sender,
		state:       state,
		log:         log,
		queue:       make(chan DeliveryJob, queueCapacity),
		done:        make(chan struct{}),
		dedupe:      make(map[string]time.Time),
		daily:       make(map[string]dayCounter),
		bucket:      newTokenBucket(cfg.MaxPushesPerMinute, now()),
		now:         now,
		backoffBase: time.Second,
	}
	dispatcher.sleep = dispatcher.defaultSleep
	return dispatcher
}

// Start 启动唯一的 worker goroutine。
func (d *Dispatcher) Start() {
	if d == nil {
		return
	}
	d.startOnce.Do(func() { go d.run() })
}

// Stop 让 worker 退出。已入队的任务不再投递（对应「暂停推送」的语义）。
func (d *Dispatcher) Stop() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() { close(d.done) })
}

// Pending 返回当前队列长度，供状态工具显示。
func (d *Dispatcher) Pending() int {
	if d == nil {
		return 0
	}
	return len(d.queue)
}

// Admit 是 topic.created 的过滤闸门，顺序固定：暂停 → 凭据缺失 → 白名单为空 →
// 分类不匹配 → 去重。命中任一条件返回 (false, reason)；去重键只在放行时写入，
// 因此「暂停期间的事件」不会占用之后真实投递的去重窗口。
func (d *Dispatcher) Admit(deliveryKey, topicID, categorySlug string) (bool, string) {
	if d == nil {
		return false, skipCredentialsMissing
	}
	if d.cfg.Paused {
		return false, skipPaused
	}
	if !d.cfg.CredentialsConfigured() {
		return false, skipCredentialsMissing
	}
	if len(d.cfg.GroupIDs) == 0 {
		return false, skipWhitelistEmpty
	}
	if len(d.cfg.CategoryFilter) > 0 && !containsString(d.cfg.CategoryFilter, categorySlug) {
		return false, skipCategoryFiltered
	}
	key := dedupeKey(deliveryKey, topicID)
	if key == "" {
		return true, ""
	}
	if d.seenRecently(key) {
		return false, skipDuplicate
	}
	d.remember(key)
	return true, ""
}

// Enqueue 把一次投递放进队列。队列满时丢最旧的一条并记 warn，返回值是入队后的长度。
func (d *Dispatcher) Enqueue(job DeliveryJob) int {
	if d == nil || len(job.Groups) == 0 {
		return d.Pending()
	}
	d.enqueueMu.Lock()
	defer d.enqueueMu.Unlock()
	for attempt := 0; attempt < queueCapacity+2; attempt++ {
		select {
		case d.queue <- job:
			return len(d.queue)
		default:
		}
		select {
		case dropped := <-d.queue:
			d.log.Warnf("qq-bot: 队列已满，丢弃最旧的一条投递（《%s》）", dropped.TopicTitle)
		default:
			// worker 刚好取空了队列，重试入队即可。
		}
	}
	d.log.Warnf("qq-bot: 队列持续拥塞，本次投递被丢弃（《%s》）", job.TopicTitle)
	return len(d.queue)
}

func (d *Dispatcher) run() {
	for {
		select {
		case <-d.done:
			return
		case job := <-d.queue:
			d.deliver(job)
		}
	}
}

func (d *Dispatcher) deliver(job DeliveryJob) {
	for _, group := range job.Groups {
		select {
		case <-d.done:
			return
		default:
		}
		if !d.acquire() {
			return
		}
		if !d.allowDaily(group) {
			d.state.RecordDelivery(DeliveryRecord{
				GroupID: maskGroupID(group), TopicTitle: job.TopicTitle,
				OK: false, Reason: reasonQuotaExceeded, Truncated: job.Truncated,
			})
			d.log.Warnf("qq-bot: 群 %s 触达单日推送上限，本条不再投递", maskGroupID(group))
			continue
		}
		d.deliverOne(job, group)
	}
}

func (d *Dispatcher) deliverOne(job DeliveryJob, group string) {
	maxRetries := d.cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; ; attempt++ {
		result, err := d.sender.SendGroupMessage(context.Background(), group, job.Content, d.cfg.MessageFormat)
		if err == nil {
			d.state.RecordDelivery(DeliveryRecord{
				GroupID: maskGroupID(group), TopicTitle: job.TopicTitle,
				OK: true, Truncated: job.Truncated,
			})
			d.log.Infof("qq-bot: 已推送到群 %s（message %s）", maskGroupID(group), messageIDPrefix(result.MessageID))
			return
		}
		if !transientError(err) || attempt >= maxRetries {
			d.state.RecordDelivery(DeliveryRecord{
				GroupID: maskGroupID(group), TopicTitle: job.TopicTitle,
				OK: false, Reason: adminReason(err), Truncated: job.Truncated,
			})
			d.log.Warnf("qq-bot: 推送到群 %s 失败：%s", maskGroupID(group), adminReason(err))
			return
		}
		// 瞬时错误：指数退避 1s / 4s / …，最多 max_retries 次重试。
		delay := d.backoff(attempt)
		d.log.Warnf("qq-bot: 推送到群 %s 遇到瞬时错误，%s 后重试（第 %d 次）：%s",
			maskGroupID(group), delay, attempt+1, adminReason(err))
		if !d.sleep(context.Background(), delay) {
			return
		}
	}
}

// acquire 取一个全局每分钟令牌；没有令牌就等到有为止。
func (d *Dispatcher) acquire() bool {
	for {
		wait := d.bucket.take(d.now())
		if wait <= 0 {
			return true
		}
		if !d.sleep(context.Background(), wait) {
			return false
		}
	}
}

func (d *Dispatcher) allowDaily(group string) bool {
	if d == nil || d.cfg.DailyPushCapPerGroup <= 0 {
		return true
	}
	day := d.now().Format("2006-01-02")
	d.mu.Lock()
	defer d.mu.Unlock()
	counter := d.daily[group]
	if counter.day != day {
		counter = dayCounter{day: day}
	}
	if counter.count >= d.cfg.DailyPushCapPerGroup {
		d.daily[group] = counter
		return false
	}
	counter.count++
	d.daily[group] = counter
	return true
}

func (d *Dispatcher) backoff(attempt int) time.Duration {
	base := d.backoffBase
	if base <= 0 {
		base = time.Second
	}
	delay := base
	for index := 0; index < attempt; index++ {
		delay *= 4
		if delay > time.Minute {
			return time.Minute
		}
	}
	return delay
}

func (d *Dispatcher) defaultSleep(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-d.done:
		return false
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (d *Dispatcher) seenRecently(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	seenAt, ok := d.dedupe[key]
	if !ok {
		return false
	}
	if d.now().Sub(seenAt) > dedupeTTL {
		delete(d.dedupe, key)
		return false
	}
	return true
}

func (d *Dispatcher) remember(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	if len(d.dedupe) >= dedupePurgeThreshold {
		for existing, seenAt := range d.dedupe {
			if now.Sub(seenAt) > dedupeTTL {
				delete(d.dedupe, existing)
			}
		}
	}
	d.dedupe[key] = now
}

// dedupeKey 优先用宿主投递 id（同一事件重投时它不变）；缺失时退到主题 id。
func dedupeKey(deliveryID, topicID string) string {
	if key := trimKey(deliveryID); key != "" {
		return "delivery:" + key
	}
	if key := trimKey(topicID); key != "" {
		return "topic:" + key
	}
	return ""
}

func trimKey(value string) string {
	return strings.TrimSpace(value)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// adminReason 把一次发送失败翻译成管理员可读的单行原因。
func adminReason(err error) string {
	var qqError *QQError
	if errors.As(err, &qqError) {
		return qqError.AdminMessage()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// tokenBucket 是全局每分钟限速器：容量 = perMinute，匀速回填 perMinute/60 每秒。
type tokenBucket struct {
	mu              sync.Mutex
	capacity        float64
	refillPerSecond float64
	tokens          float64
	last            time.Time
}

func newTokenBucket(perMinute int, now time.Time) tokenBucket {
	if perMinute <= 0 {
		// 0 表示不限制：容量与回填都为 0，take 永远返回 0。
		return tokenBucket{last: now}
	}
	return tokenBucket{
		capacity:        float64(perMinute),
		refillPerSecond: float64(perMinute) / 60,
		tokens:          float64(perMinute),
		last:            now,
	}
}

// take 尝试取一个令牌；成功返回 0，否则返回需要等待的时长。
func (b *tokenBucket) take(now time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.capacity <= 0 || b.refillPerSecond <= 0 {
		return 0
	}
	if !now.Before(b.last) {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens = minFloat(b.capacity, b.tokens+elapsed*b.refillPerSecond)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	missing := 1 - b.tokens
	wait := time.Duration(missing / b.refillPerSecond * float64(time.Second))
	if wait <= 0 {
		wait = time.Millisecond
	}
	return wait
}

// refund 归还一个未使用的令牌（上限 capacity）。用于「先占位、失败回滚」的限流：
// 失败的那次不该消耗配额。
func (b *tokenBucket) refund() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.capacity <= 0 || b.refillPerSecond <= 0 {
		return
	}
	b.tokens = minFloat(b.capacity, b.tokens+1)
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
