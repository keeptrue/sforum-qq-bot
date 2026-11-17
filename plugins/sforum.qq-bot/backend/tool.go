package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
)

const (
	// aiToolTimeoutMS 与清单 aiTools[].timeoutMs 逐字一致（宿主派发前按它校验，
	// 声明值不是预算：真正的等待由设置里的发送超时决定）。
	aiToolTimeoutMS = 10_000
	// statusMaxChars 是状态输出的**字节**预算，让结果稳稳落在宿主的 4096 字节
	// 截断线以内（中文一词三字节，按 rune 计数会超）。
	statusMaxChars = 3500
)

const testPushToolDescription = "仅供管理员在插件设置页执行「测试推送」使用，模型不应调用它。" +
	"它用设置页当前草稿值渲染一条样例主题并发送到白名单里的第一个群，" +
	"不发真实帖子、不查论坛数据；失败时给出可读原因。"

const statusToolDescription = "仅供管理员在插件设置页查看「推送状态」使用，模型不应调用它。" +
	"它返回配置概览、长连接状态、最近投递记录与最近识别到的群 ID。"

const previewToolDescription = "仅供管理员在插件设置页查看「模板预览」使用，模型不应调用它。" +
	"它用设置页当前草稿值渲染一条样例消息并原样返回，只做渲染：不发送、不保存、不查论坛数据。"

// testPushParameters 与清单 aiTools[].parameters 的字段集合逐字一致。草稿值全部
// 以字符串形态存在，因此这里把数值项也声明为 string，避免宿主表单序列化差异。
//
// 声明里有 app_id / app_secret / send_timeout_seconds，但本工具不使用它们：
// AppSecret 绝不再出现在第二个出口（凭据只进设置项，只在完成绑定的那次
// /bind/poll 响应里出现一次），发送超时也只认进程启动时注入的那一份。
var testPushParameters = mustCompactJSON(map[string]any{
	"type": "object",
	"properties": map[string]any{
		"app_id":               map[string]any{"type": "string", "description": "设置页草稿中的 AppID（本工具不读取，实际凭据来自已保存设置）。"},
		"app_secret":           map[string]any{"type": "string", "description": "设置页草稿中的 AppSecret（本工具不读取，避免密钥多一个出口）。"},
		"send_timeout_seconds": map[string]any{"type": "string", "description": "设置页草稿中的单次发送超时秒数（本工具不读取，实际超时来自已保存设置）。"},
		"message_template":     map[string]any{"type": "string", "description": "设置页草稿中的推送模板（含 {{title}} 等变量）。"},
		"message_format":       map[string]any{"type": "string", "description": "设置页草稿中的消息形态（text / markdown）。"},
		"signature":            map[string]any{"type": "string", "description": "设置页草稿中的消息签名。"},
		"excerpt_limit":        map[string]any{"type": "string", "description": "设置页草稿中的摘要长度上限。"},
		"max_message_chars":    map[string]any{"type": "string", "description": "设置页草稿中的消息总长度上限。"},
		"group_ids":            map[string]any{"type": "string", "description": "设置页草稿中的群 ID 列表，换行或逗号分隔。"},
	},
})

// statusParameters 与清单同名同义，且同样只做声明：状态查询对参数完全宽容
// （handler 忽略它们），补齐字段是为了让清单与代码的声明逐字一致。
var statusParameters = mustCompactJSON(map[string]any{
	"type": "object",
	"properties": map[string]any{
		"app_id":                map[string]any{"type": "string", "description": "设置页草稿中的 AppID（本工具不读取）。"},
		"app_secret":            map[string]any{"type": "string", "description": "设置页草稿中的 AppSecret（本工具不读取，也不会回显）。"},
		"enable_group_id_claim": map[string]any{"type": "string", "description": "设置页草稿中的群 ID 自动识别开关（本工具不读取）。"},
		"group_ids":             map[string]any{"type": "string", "description": "设置页草稿中的群 ID 列表（本工具不读取）。"},
		"message_format":        map[string]any{"type": "string", "description": "设置页草稿中的消息形态（本工具不读取）。"},
		"paused":                map[string]any{"type": "string", "description": "设置页草稿中的暂停开关（本工具不读取）。"},
	},
})

// previewParameters 与预览 action 的 fields 同名同义；草稿值全部以字符串形态
// 存在，因此数值项也声明为 string。
var previewParameters = mustCompactJSON(map[string]any{
	"type": "object",
	"properties": map[string]any{
		"message_template":  map[string]any{"type": "string", "description": "设置页草稿中的推送模板（含 {{title}} 等变量）。"},
		"message_format":    map[string]any{"type": "string", "description": "设置页草稿中的消息形态（text / markdown）。"},
		"signature":         map[string]any{"type": "string", "description": "设置页草稿中的消息签名。"},
		"excerpt_limit":     map[string]any{"type": "string", "description": "设置页草稿中的摘要长度上限，0 表示不显示摘要。"},
		"max_message_chars": map[string]any{"type": "string", "description": "设置页草稿中的消息总长度上限。"},
	},
})

func aiToolDefinitions(p *plugin) []pluginv2.AIToolDefinition {
	return []pluginv2.AIToolDefinition{
		{
			ID: toolTestPushID, ContractVersion: toolTestPushID + "@1",
			Name: "qq-bot-test-push", Description: testPushToolDescription,
			Parameters: testPushParameters, TimeoutMS: aiToolTimeoutMS,
			Execute: p.testPushHandler,
		},
		{
			ID: toolStatusID, ContractVersion: toolStatusID + "@1",
			Name: "qq-bot-status", Description: statusToolDescription,
			Parameters: statusParameters, TimeoutMS: aiToolTimeoutMS,
			Execute: p.statusHandler,
		},
		{
			ID: toolPreviewID, ContractVersion: toolPreviewID + "@1",
			Name: "qq-bot-preview", Description: previewToolDescription,
			Parameters: previewParameters, TimeoutMS: aiToolTimeoutMS,
			Execute: p.previewHandler,
		},
	}
}

// testPushHandler 是设置页「发一条测试消息」按钮的入口。探测 runtime 是被宿主用
// 草稿设置临时拉起来的，所以本进程环境里就是运营者正在编辑的那份配置；参数只是
// 同一份事实的冗余副本。它**不**调用 HostQuery：探测 runtime 没有 host token。
func (p *plugin) testPushHandler(ctx context.Context, call *pluginv2.AIToolCall) (*pluginv2.AIToolResult, error) {
	p.enterProbeMode()
	draft, err := parseDraftArguments(call.Arguments)
	if err != nil {
		return testPushFailure("参数无法解析：" + err.Error() +
			"。这个工具只接受表单字段（message_template / message_format / signature / excerpt_limit / max_message_chars / group_ids），不接受自由文本。"), nil
	}

	config := p.cfg
	// test_push 保持「空串 = 沿用已保存值」：发送路径不引入「发一条空消息」这个
	// 新失败面（M1.2 决策 D3）。只有预览按 presence-aware 渲染空串。
	if draft.MessageTemplate != nil && *draft.MessageTemplate != "" {
		config.MessageTemplate = *draft.MessageTemplate
	}
	if draft.MessageFormat != "" {
		config.MessageFormat = normalizeFormat(draft.MessageFormat)
	}
	if draft.Signature != nil && *draft.Signature != "" {
		config.Signature = *draft.Signature
	}
	if draft.ExcerptLimit != nil {
		config.ExcerptLimit = *draft.ExcerptLimit
	}
	if draft.MaxMessageChars != nil {
		config.MaxMessageChars = *draft.MaxMessageChars
	}
	groups := config.GroupIDs
	if len(draft.GroupIDs) > 0 {
		groups = draft.GroupIDs
	}

	rendered := Render(config.MessageTemplate, sampleTemplateData(config.Signature), RenderOptions{
		Format: config.MessageFormat, MaxChars: config.MaxMessageChars, Warnf: p.log.Warnf,
	})

	var report strings.Builder
	report.WriteString(rendered.Text)
	report.WriteString("\n\n—— 发送结果 ——\n")
	if len(groups) == 0 {
		report.WriteString("未发送：群白名单是空的，请先填写至少一个群 ID。")
		return testPushFailure(report.String()), nil
	}
	if !config.CredentialsConfigured() {
		report.WriteString("未发送：尚未配置 AppID / AppSecret。")
		return testPushFailure(report.String()), nil
	}
	target := groups[0]
	if p.client == nil {
		report.WriteString("未发送：QQ 客户端未初始化。")
		return testPushFailure(report.String()), nil
	}
	result, sendErr := p.client.SendGroupMessage(ctx, target, rendered.Text, config.MessageFormat)
	if sendErr != nil {
		fmt.Fprintf(&report, "发送失败：%s", adminReason(sendErr))
		return testPushFailure(report.String()), nil
	}
	fmt.Fprintf(&report, "已发送到群 %s（message id %s…）", maskGroupID(target), messageIDPrefix(result.MessageID))
	return &pluginv2.AIToolResult{Content: report.String()}, nil
}

// statusHandler 是设置页「推送状态」按钮的入口。它对参数完全宽容：状态查询不该
// 因为模型或前端塞了奇怪参数而失败。
func (p *plugin) statusHandler(_ context.Context, _ *pluginv2.AIToolCall) (*pluginv2.AIToolResult, error) {
	p.enterProbeMode()
	return &pluginv2.AIToolResult{Content: p.renderStatus()}, nil
}

// previewHandler 是 schema 回退路径上的「预览（不发送）」。组件模式走
// /preview 路由，两条入口共用 renderPreview，输出逐字一致。
//
// IsError 只在参数不可解析时为真：模板里写了未知变量、消息被截断都是可预期的
// 编辑中间态，要在正文里提示，而不是把整次预览标成失败。
func (p *plugin) previewHandler(_ context.Context, call *pluginv2.AIToolCall) (*pluginv2.AIToolResult, error) {
	p.enterProbeMode()
	draft, err := parseDraftArguments(call.Arguments)
	if err != nil {
		return &pluginv2.AIToolResult{
			Content: "参数无法解析：" + err.Error() +
				"。这个工具只接受表单字段（message_template / message_format / signature / excerpt_limit / max_message_chars），不接受自由文本。",
			IsError: true,
		}, nil
	}

	config := p.cfg
	draftConfig := previewDraft{
		Template:        config.MessageTemplate,
		Format:          config.MessageFormat,
		Signature:       config.Signature,
		ExcerptLimit:    config.ExcerptLimit,
		MaxMessageChars: config.MaxMessageChars,
	}
	// 与 /preview 同一约定：键在就按给的值渲染（空串渲染空），缺键才回落。
	if draft.MessageTemplate != nil {
		draftConfig.Template = *draft.MessageTemplate
	}
	if draft.MessageFormat != "" {
		draftConfig.Format = normalizeFormat(draft.MessageFormat)
	}
	if draft.Signature != nil {
		draftConfig.Signature = *draft.Signature
	}
	if draft.ExcerptLimit != nil {
		draftConfig.ExcerptLimit = *draft.ExcerptLimit
	}
	// 与预览路由同一约定：0 表示沿用已保存的默认值，而不是不限长。
	if draft.MaxMessageChars != nil && *draft.MaxMessageChars > 0 {
		draftConfig.MaxMessageChars = *draft.MaxMessageChars
	}

	rendered := p.renderPreview(draftConfig)
	var report strings.Builder
	report.WriteString(rendered.Text)
	if rendered.Truncated {
		fmt.Fprintf(&report, "\n\n（已按 %d 字截断）", draftConfig.MaxMessageChars)
	}
	if len(rendered.Unknown) > 0 {
		placeholders := make([]string, 0, len(rendered.Unknown))
		for _, name := range rendered.Unknown {
			placeholders = append(placeholders, "{{"+name+"}}")
		}
		fmt.Fprintf(&report, "\n\n未知变量：%s（已原样保留，可对照上方变量清单修正）",
			strings.Join(placeholders, "、"))
	}
	return &pluginv2.AIToolResult{Content: report.String()}, nil
}

func (p *plugin) renderStatus() string {
	var report strings.Builder
	report.WriteString("QQ 推送状态\n")

	credentials := "未配置"
	if p.cfg.CredentialsConfigured() {
		credentials = "已配置"
	}
	paused := "否"
	if p.cfg.Paused {
		paused = "是"
	}
	claim := "开"
	if !p.cfg.EnableGroupIDClaim {
		claim = "关"
	}
	queue := 0
	if p.dispatcher != nil {
		queue = p.dispatcher.Pending()
	}
	fmt.Fprintf(&report, "凭据：%s｜目标群：%s 个｜暂停：%s｜群 ID 识别：%s｜队列：%s 条\n",
		credentials, strconv.Itoa(len(p.cfg.GroupIDs)), paused, claim, strconv.Itoa(queue))

	status := p.gateway.Status()
	report.WriteString("长连接：" + status.State)
	if status.ConnectedFor > 0 {
		fmt.Fprintf(&report, "（已连接 %s）", formatDuration(status.ConnectedFor))
	}
	if status.LastError != "" {
		report.WriteString("，最近错误：" + status.LastError)
	}
	report.WriteString("\n")

	report.WriteString("最近投递：\n")
	deliveries := p.state.RecentDeliveries(10)
	if len(deliveries) == 0 {
		report.WriteString("（暂无记录）\n")
	}
	for _, delivery := range deliveries {
		outcome := "成功"
		if !delivery.OK {
			outcome = "失败"
		}
		fmt.Fprintf(&report, "%s %s %s", delivery.At.Format("01-02 15:04"), outcome, delivery.GroupID)
		if delivery.TopicTitle != "" {
			fmt.Fprintf(&report, " 《%s》", delivery.TopicTitle)
		}
		if delivery.Truncated {
			report.WriteString("（已截断）")
		}
		if delivery.Reason != "" {
			report.WriteString("：" + delivery.Reason)
		}
		report.WriteString("\n")
	}

	report.WriteString("最近识别的群 ID（完整，可复制到白名单）：\n")
	claims := p.state.RecentClaims(10)
	if len(claims) == 0 {
		report.WriteString("（暂无记录：机器人被拉进群或群里 @机器人 后会出现在这里）\n")
	}
	for _, claim := range claims {
		fmt.Fprintf(&report, "%s（%s，%s，%d 次）\n",
			claim.GroupID, claim.EventType, claim.FirstSeen.Format("01-02 15:04"), claim.Count)
	}
	return clampText(report.String(), statusMaxChars)
}

// enterProbeMode 是探测隔离：设置页会用短命 runtime 调这两个工具，它没有 Host
// token，绝不能跟正式 runtime 抢长连接 session。第一次调用即关闭并禁止重连。
func (p *plugin) enterProbeMode() {
	if p == nil {
		return
	}
	p.probeOnce.Do(func() {
		if p.gateway != nil {
			p.gateway.MarkProbeOnly()
		}
	})
}

func testPushFailure(content string) *pluginv2.AIToolResult {
	return &pluginv2.AIToolResult{Content: content, IsError: true}
}

func sampleTemplateData(signature string) TemplateData {
	return TemplateData{
		Title:     "【测试】这是一条来自 SForum 的推送样例",
		Author:    "示例作者",
		Category:  "示例分类",
		Tags:      []string{"测试", "推送"},
		Excerpt:   "这里是摘要占位：真实推送时这一段会换成主题正文的开头。",
		URL:       "https://forum.example.com/t/10001",
		TopicID:   "10001",
		Signature: signature,
	}
}

// draftArguments 是设置页草稿值的冗余副本。
//
// MessageTemplate 与 Signature 用指针区分「没给这个键」与「给了空串」：预览里
// 清空模板要渲染空，缺键才回落已保存配置。其余字段沿用「零值 = 没给」。
type draftArguments struct {
	MessageTemplate *string
	MessageFormat   string
	Signature       *string
	ExcerptLimit    *int
	MaxMessageChars *int
	GroupIDs        []string
}

func parseDraftArguments(raw string) (draftArguments, error) {
	var draft draftArguments
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return draft, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return draft, fmt.Errorf("不是合法的 JSON 对象")
	}
	draft.MessageTemplate = rawOptionalString(payload, "message_template")
	draft.MessageFormat = rawString(payload, "message_format")
	draft.Signature = rawOptionalString(payload, "signature")
	draft.ExcerptLimit = rawInt(payload, "excerpt_limit")
	draft.MaxMessageChars = rawInt(payload, "max_message_chars")
	draft.GroupIDs = splitSettingList(rawString(payload, "group_ids"))
	return draft, nil
}

func rawString(payload map[string]json.RawMessage, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// rawOptionalString 读一个字符串字段，并保留「键在但为空」这个事实——预览要按
// 空串渲染而不是回落。键不存在、值是 null、或不是字符串都返回 nil（= 没给）。
func rawOptionalString(payload map[string]json.RawMessage, key string) *string {
	raw, ok := payload[key]
	if !ok {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func rawInt(payload map[string]json.RawMessage, key string) *int {
	raw, ok := payload[key]
	if !ok {
		return nil
	}
	var numeric float64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		value := int(numeric)
		return &value
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || value < 0 {
		return nil
	}
	return &value
}

func normalizeFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case messageFormatMarkdown, "md":
		return messageFormatMarkdown
	default:
		return messageFormatText
	}
}

func formatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%d 秒", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%d 分 %d 秒", int(duration.Minutes()), int(duration.Seconds())%60)
	}
	return fmt.Sprintf("%d 时 %d 分", int(duration.Hours()), int(duration.Minutes())%60)
}

// clampText 把文本按**字节**安全截断到 limit 以内。
//
// 宿主按字节切 4096，按 rune 截断（limit 个字）会让中文输出到三倍长度后仍被腰斩，
// 还可能切出半个 rune。这里在字节预算内回退到最近的 rune 边界，并留出省略号的
// 空间，保证「返回值 ≤ limit 字节」且永远是合法 UTF-8。
func clampText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	const suffix = "\n…（输出已截断）"
	// 预算装不下省略号时只切正文：宁可没有提示，也不能超过字节上限。
	if limit <= len(suffix) {
		return value[:runeSafeCut(value, limit)]
	}
	budget := limit - len(suffix)
	if budget > len(value) {
		budget = len(value)
	}
	return value[:runeSafeCut(value, budget)] + suffix
}

// runeSafeCut 返回不超过 budget 字节的最大 rune 边界偏移。
func runeSafeCut(value string, budget int) int {
	if budget >= len(value) {
		return len(value)
	}
	if budget < 0 {
		return 0
	}
	cut := budget
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return cut
}

func mustCompactJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode AI tool parameters: %v", err))
	}
	return string(encoded)
}
