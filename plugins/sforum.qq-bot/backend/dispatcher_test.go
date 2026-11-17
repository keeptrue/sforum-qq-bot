package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAdmitSkipReasons(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"暂停", func(c *Config) { c.Paused = true }, skipPaused},
		{"缺凭据", func(c *Config) { c.AppSecret = "" }, skipCredentialsMissing},
		{"白名单为空", func(c *Config) { c.GroupIDs = nil }, skipWhitelistEmpty},
		{"分类不匹配", func(c *Config) { c.CategoryFilter = []string{"news", "tech"} }, skipCategoryFiltered},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			config := testConfig()
			testCase.mutate(&config)
			dispatcher := NewDispatcher(config, &fakeSender{}, NewState(), testLogger())
			admitted, reason := dispatcher.Admit("delivery-1", "12345", "announce")
			if admitted {
				t.Fatalf("应被过滤，reason=%q", reason)
			}
			if reason != testCase.want {
				t.Fatalf("过滤原因期望 %q，得到 %q", testCase.want, reason)
			}
		})
	}
}

func TestAdmitCategoryMatchAndEmptyFilter(t *testing.T) {
	config := testConfig()
	config.CategoryFilter = []string{"announce"}
	dispatcher := NewDispatcher(config, &fakeSender{}, NewState(), testLogger())
	if admitted, reason := dispatcher.Admit("d-1", "12345", "announce"); !admitted {
		t.Fatalf("分类在白名单内应放行，reason=%q", reason)
	}
	config.CategoryFilter = nil
	dispatcher = NewDispatcher(config, &fakeSender{}, NewState(), testLogger())
	if admitted, reason := dispatcher.Admit("d-2", "12345", "anything"); !admitted {
		t.Fatalf("分类过滤为空应放行全部，reason=%q", reason)
	}
}

func TestAdmitDeduplicatesWithinTTL(t *testing.T) {
	dispatcher := NewDispatcher(testConfig(), &fakeSender{}, NewState(), testLogger())
	base := time.Date(2026, 11, 14, 23, 0, 0, 0, time.UTC)
	current := base
	dispatcher.now = func() time.Time { return current }

	if admitted, _ := dispatcher.Admit("delivery-1", "12345", "announce"); !admitted {
		t.Fatal("首次投递应放行")
	}
	if admitted, reason := dispatcher.Admit("delivery-1", "12345", "announce"); admitted || reason != skipDuplicate {
		t.Fatalf("同投递键应去重，admitted=%t reason=%q", admitted, reason)
	}
	// 不同投递 id 同一主题：不是重复事件（编辑后再触发属于新投递）。
	if admitted, reason := dispatcher.Admit("delivery-2", "12345", "announce"); !admitted {
		t.Fatalf("不同投递 id 应放行，reason=%q", reason)
	}
	current = base.Add(dedupeTTL + time.Minute)
	if admitted, reason := dispatcher.Admit("delivery-1", "12345", "announce"); !admitted {
		t.Fatalf("超过 TTL 后应重新放行，reason=%q", reason)
	}
}

func TestAdmitFallsBackToTopicIDAsKey(t *testing.T) {
	dispatcher := NewDispatcher(testConfig(), &fakeSender{}, NewState(), testLogger())
	if admitted, _ := dispatcher.Admit("", "12345", "announce"); !admitted {
		t.Fatal("首次投递应放行")
	}
	if admitted, reason := dispatcher.Admit("", "12345", "announce"); admitted || reason != skipDuplicate {
		t.Fatalf("投递 id 缺失时应退回主题 id 去重，admitted=%t reason=%q", admitted, reason)
	}
}

func TestAdmitPausedDoesNotConsumeDedupeWindow(t *testing.T) {
	config := testConfig()
	config.Paused = true
	dispatcher := NewDispatcher(config, &fakeSender{}, NewState(), testLogger())
	if admitted, _ := dispatcher.Admit("delivery-1", "12345", "announce"); admitted {
		t.Fatal("暂停时不应放行")
	}
	config.Paused = false
	dispatcher.cfg = config
	if admitted, reason := dispatcher.Admit("delivery-1", "12345", "announce"); !admitted {
		t.Fatalf("恢复后同一投递键应放行，reason=%q", reason)
	}
}

func TestTokenBucketRefills(t *testing.T) {
	start := time.Date(2026, 11, 14, 23, 0, 0, 0, time.UTC)
	bucket := newTokenBucket(2, start)
	if wait := bucket.take(start); wait != 0 {
		t.Fatalf("第一个令牌应立即可用，wait=%v", wait)
	}
	if wait := bucket.take(start); wait != 0 {
		t.Fatalf("第二个令牌应立即可用，wait=%v", wait)
	}
	if wait := bucket.take(start); wait <= 0 {
		t.Fatalf("令牌耗尽后应等待，wait=%v", wait)
	}
	// 容量 2/分钟 = 每 30 秒回填一个令牌。
	if wait := bucket.take(start.Add(31 * time.Second)); wait != 0 {
		t.Fatalf("回填后应可立即取用，wait=%v", wait)
	}
}

func TestTokenBucketUnlimited(t *testing.T) {
	bucket := newTokenBucket(0, time.Now())
	for index := 0; index < 5; index++ {
		if wait := bucket.take(time.Now()); wait != 0 {
			t.Fatalf("0 表示不限制，wait=%v", wait)
		}
	}
}

func TestDailyQuotaPerGroup(t *testing.T) {
	config := testConfig()
	config.DailyPushCapPerGroup = 2
	dispatcher := NewDispatcher(config, &fakeSender{}, NewState(), testLogger())
	day := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	current := day
	dispatcher.now = func() time.Time { return current }

	for index := 0; index < 2; index++ {
		if !dispatcher.allowDaily("GROUP_A") {
			t.Fatalf("第 %d 次应计入配额", index+1)
		}
	}
	if dispatcher.allowDaily("GROUP_A") {
		t.Fatal("触达上限后应拒绝")
	}
	if !dispatcher.allowDaily("GROUP_B") {
		t.Fatal("配额按群独立计数")
	}
	current = day.Add(24 * time.Hour)
	if !dispatcher.allowDaily("GROUP_A") {
		t.Fatal("跨天后应重置配额")
	}
}

func TestDeliverRetriesTransientAndRecordsSuccess(t *testing.T) {
	config := testConfig()
	config.MaxRetries = 2
	sender := &fakeSender{}
	sender.behavior = func(index int, _ string) (SendResult, error) {
		if index < 2 {
			return SendResult{}, &QQError{Reason: "主动消息已超出频控", Code: 40034100, Transient: true}
		}
		return SendResult{MessageID: "msg-ok"}, nil
	}
	state := NewState()
	dispatcher := NewDispatcher(config, sender, state, testLogger())
	dispatcher.backoffBase = time.Millisecond

	dispatcher.deliver(DeliveryJob{TopicTitle: "新版本发布", Content: "hello", Groups: []string{"GROUP_A"}})

	if calls := len(sender.Calls()); calls != 3 {
		t.Fatalf("瞬时错误应重试到成功，实际调用 %d 次", calls)
	}
	records := state.RecentDeliveries(10)
	if len(records) != 1 || !records[0].OK {
		t.Fatalf("应记录一条成功投递：%+v", records)
	}
	if records[0].GroupID != maskGroupID("GROUP_A") {
		t.Fatalf("记录里的群 ID 应脱敏：%q", records[0].GroupID)
	}
}

func TestDeliverDoesNotRetryBusinessError(t *testing.T) {
	config := testConfig()
	config.MaxRetries = 3
	sender := &fakeSender{}
	sender.behavior = func(int, string) (SendResult, error) {
		return SendResult{}, &QQError{Reason: "凭据无效", Code: 100016}
	}
	state := NewState()
	dispatcher := NewDispatcher(config, sender, state, testLogger())
	dispatcher.backoffBase = time.Millisecond

	dispatcher.deliver(DeliveryJob{TopicTitle: "标题", Content: "hello", Groups: []string{"GROUP_A"}})

	if calls := len(sender.Calls()); calls != 1 {
		t.Fatalf("业务错误不应重试，实际调用 %d 次", calls)
	}
	records := state.RecentDeliveries(10)
	if len(records) != 1 || records[0].OK {
		t.Fatalf("应记录一次失败：%+v", records)
	}
	if !strings.Contains(records[0].Reason, "凭据无效") {
		t.Fatalf("失败原因应是翻译后的文案：%q", records[0].Reason)
	}
}

func TestDeliverGivesUpAfterMaxRetries(t *testing.T) {
	config := testConfig()
	config.MaxRetries = 1
	sender := &fakeSender{}
	sender.behavior = func(int, string) (SendResult, error) {
		return SendResult{}, &QQError{Reason: "连不上 QQ 接口", Transient: true}
	}
	state := NewState()
	dispatcher := NewDispatcher(config, sender, state, testLogger())
	dispatcher.backoffBase = time.Millisecond

	dispatcher.deliver(DeliveryJob{TopicTitle: "标题", Content: "hello", Groups: []string{"GROUP_A"}})

	if calls := len(sender.Calls()); calls != 2 {
		t.Fatalf("1 次重试意味着共 2 次调用，实际 %d", calls)
	}
	records := state.RecentDeliveries(10)
	if len(records) != 1 || records[0].OK {
		t.Fatalf("应记录失败：%+v", records)
	}
}

func TestDeliverIsolatesGroups(t *testing.T) {
	config := testConfig()
	config.MaxRetries = 0
	sender := &fakeSender{}
	sender.behavior = func(_ int, group string) (SendResult, error) {
		if group == "GROUP_A" {
			return SendResult{}, &QQError{Reason: "该接口需要平台白名单", Code: 11253}
		}
		return SendResult{MessageID: "msg-b"}, nil
	}
	state := NewState()
	dispatcher := NewDispatcher(config, sender, state, testLogger())

	dispatcher.deliver(DeliveryJob{TopicTitle: "标题", Content: "hello", Groups: []string{"GROUP_A", "GROUP_B"}})

	records := state.RecentDeliveries(10)
	if len(records) != 2 {
		t.Fatalf("两个群应各有一条记录：%+v", records)
	}
	if records[0].OK || !records[1].OK {
		t.Fatalf("前一个群失败不应影响后一个：%+v", records)
	}
	if len(sender.Calls()) != 2 {
		t.Fatalf("应各尝试一次，实际 %d", len(sender.Calls()))
	}
}

func TestDeliverStopsAtDailyQuota(t *testing.T) {
	config := testConfig()
	config.DailyPushCapPerGroup = 1
	sender := &fakeSender{}
	state := NewState()
	dispatcher := NewDispatcher(config, sender, state, testLogger())
	dispatcher.now = func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }

	dispatcher.deliver(DeliveryJob{TopicTitle: "第一次", Content: "hello", Groups: []string{"GROUP_A"}})
	dispatcher.deliver(DeliveryJob{TopicTitle: "第二次", Content: "hello", Groups: []string{"GROUP_A"}})

	if calls := len(sender.Calls()); calls != 1 {
		t.Fatalf("触达日配额后不应再发送，实际 %d 次", calls)
	}
	records := state.RecentDeliveries(10)
	if len(records) != 2 {
		t.Fatalf("应记录成功与超配额各一条：%+v", records)
	}
	if records[1].OK || records[1].Reason != reasonQuotaExceeded {
		t.Fatalf("第二条应记为 quota_exceeded：%+v", records[1])
	}
}

func TestEnqueueDropsOldestWhenFull(t *testing.T) {
	var buffer bytes.Buffer
	config := testConfig()
	dispatcher := NewDispatcher(config, &fakeSender{}, NewState(), newLogger(&buffer, false))
	// 不入队 worker：队列保持满态。
	for index := 0; index < queueCapacity; index++ {
		dispatcher.Enqueue(DeliveryJob{TopicTitle: "填充", Content: "x", Groups: []string{"GROUP_A"}})
	}
	if pending := dispatcher.Pending(); pending != queueCapacity {
		t.Fatalf("队列应满，实际 %d", pending)
	}
	dispatcher.Enqueue(DeliveryJob{TopicTitle: "最新", Content: "x", Groups: []string{"GROUP_A"}})
	if pending := dispatcher.Pending(); pending != queueCapacity {
		t.Fatalf("丢弃最旧后长度仍应是上限，实际 %d", pending)
	}
	if !strings.Contains(buffer.String(), "队列已满") {
		t.Fatalf("应记录队列丢弃告警：%q", buffer.String())
	}
	// 队列里最后一条应是最新入队的任务。
	drained := make([]DeliveryJob, 0, queueCapacity)
	for index := 0; index < queueCapacity; index++ {
		drained = append(drained, <-dispatcher.queue)
	}
	if drained[len(drained)-1].TopicTitle != "最新" {
		t.Fatalf("队尾应是最新任务，得到 %q", drained[len(drained)-1].TopicTitle)
	}
	if drained[0].TopicTitle != "填充" {
		t.Fatalf("队首应是第二个填充任务，得到 %q", drained[0].TopicTitle)
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	dispatcher := NewDispatcher(testConfig(), &fakeSender{}, NewState(), testLogger())
	dispatcher.backoffBase = time.Second
	if got := dispatcher.backoff(0); got != time.Second {
		t.Fatalf("第一次退避应为 1s，得到 %v", got)
	}
	if got := dispatcher.backoff(1); got != 4*time.Second {
		t.Fatalf("第二次退避应为 4s，得到 %v", got)
	}
	if got := dispatcher.backoff(10); got != time.Minute {
		t.Fatalf("退避应封顶 60s，得到 %v", got)
	}
}

func TestTransientClassification(t *testing.T) {
	if transientError(errors.New("plain error")) {
		t.Fatal("非 QQError 不应视为可重试")
	}
	if !transientError(&QQError{Transient: true}) {
		t.Fatal("Transient=true 应可重试")
	}
	if transientError(&QQError{Code: 100016}) {
		t.Fatal("凭据错误不应可重试")
	}
}
