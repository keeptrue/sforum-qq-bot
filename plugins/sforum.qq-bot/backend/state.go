package main

import (
	"strings"
	"sync"
	"time"
)

const (
	deliveryRingSize = 50
	claimRingSize    = 20
)

// DeliveryRecord 是一条投递结果。GroupID 在写入时就是脱敏后的形态，原始群 ID
// 不会被记录到投递列表里——排障需要原值时看「已识别群 ID」。
type DeliveryRecord struct {
	At         time.Time
	GroupID    string
	TopicTitle string
	OK         bool
	Reason     string
	Truncated  bool
}

// ClaimRecord 是一个通过长连接识别到的目标群。
type ClaimRecord struct {
	GroupID   string
	EventType string
	FirstSeen time.Time
	LastSeen  time.Time
	Count     int
}

// State 是插件的内存投影：投递记录与群 ID 认领。只有内存，进程重启即清空。
type State struct {
	mu         sync.Mutex
	deliveries []DeliveryRecord
	claims     []ClaimRecord
}

func NewState() *State { return &State{} }

// RecordDelivery 追加一条投递记录，超出 ring 容量时丢最旧的一条。
func (s *State) RecordDelivery(record DeliveryRecord) {
	if s == nil {
		return
	}
	if record.At.IsZero() {
		record.At = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliveries = append(s.deliveries, record)
	if len(s.deliveries) > deliveryRingSize {
		s.deliveries = append([]DeliveryRecord(nil), s.deliveries[len(s.deliveries)-deliveryRingSize:]...)
	}
}

// RecordClaim 记录一次群事件：同一个群只保留一条，累加次数并更新最近时间。
func (s *State) RecordClaim(groupOpenID, eventType string) {
	groupOpenID = strings.TrimSpace(groupOpenID)
	if s == nil || groupOpenID == "" {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.claims {
		if s.claims[index].GroupID != groupOpenID {
			continue
		}
		s.claims[index].LastSeen = now
		s.claims[index].Count++
		s.claims[index].EventType = eventType
		return
	}
	s.claims = append(s.claims, ClaimRecord{
		GroupID: groupOpenID, EventType: eventType, FirstSeen: now, LastSeen: now, Count: 1,
	})
	if len(s.claims) > claimRingSize {
		s.claims = append([]ClaimRecord(nil), s.claims[len(s.claims)-claimRingSize:]...)
	}
}

// RecentDeliveries 返回最近 limit 条投递记录（旧的在前）。
func (s *State) RecentDeliveries(limit int) []DeliveryRecord {
	if s == nil || limit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	start := len(s.deliveries) - limit
	if start < 0 {
		start = 0
	}
	return append([]DeliveryRecord(nil), s.deliveries[start:]...)
}

// RecentClaims 返回最近 limit 个认领到的群（完整群 ID，供管理员复制到白名单）。
//
// 这里**故意**不脱敏：运营者要把群 ID 复制进白名单，脱敏就等于让它不可用。
// 插件里也没有「脱敏的状态出口」——投递记录里的群 ID 在写入时就已经是
// maskGroupID 的形态（见 DeliveryRecord），需要原值时看这里。
func (s *State) RecentClaims(limit int) []ClaimRecord {
	if s == nil || limit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	start := len(s.claims) - limit
	if start < 0 {
		start = 0
	}
	return append([]ClaimRecord(nil), s.claims[start:]...)
}
