package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 宿主把插件设置项按 `SFORUM_SETTING_<KEY>` 注入子进程（大写、非字母数字转下划线）。
// 设置只在进程启动时读一次：宿主保存设置会重启插件进程，因此没有热更新路径。
const settingPrefix = "SFORUM_SETTING_"

// 缺省值必须与清单 settings[].default 逐字一致；改了清单就要同步改这里。
const (
	defaultExcerptLimit         = 120
	defaultMaxMessageChars      = 1000
	defaultSignature            = "来自 SForum 论坛"
	defaultDailyPushCapPerGroup = 800
	defaultMaxPushesPerMinute   = 20
	defaultMaxRetries           = 2
	defaultSendTimeoutSeconds   = 10
	defaultEnableGroupIDClaim   = true
	defaultReplyOnMention       = true
	defaultMessageFormat        = messageFormatText
)

// defaultMessageTemplate 与 manifest/settings.json 的 message_template.default
// 逐字一致（manifest_contract_test.go 会比对，改一处必须改另一处）。
const defaultMessageTemplate = "【新帖】{{title}}\n" +
	"作者：{{author}}｜分类：{{category}}\n" +
	"{{excerpt}}\n" +
	"{{url}}\n" +
	"{{signature}}"

const (
	messageFormatText     = "text"
	messageFormatMarkdown = "markdown"
)

// Config 是本进程看到的配置快照。
type Config struct {
	AppID     string
	AppSecret string

	GroupIDs       []string
	CategoryFilter []string

	Paused             bool
	EnableGroupIDClaim bool
	// ReplyOnMention 打开时，被 @ 会用事件里的 msg_id 回一句固定说明（被动回复，
	// 不占主动消息频控）。它只影响「回不回」，不影响群 ID 认领。
	ReplyOnMention bool

	MessageFormat   string
	MessageTemplate string
	Signature       string

	ExcerptLimit    int
	MaxMessageChars int

	DailyPushCapPerGroup int
	MaxPushesPerMinute   int
	MaxRetries           int
	SendTimeoutSeconds   int
	LogVerbose           bool

	// Warnings 收集解析期发现的问题（非法数字/布尔/未知消息形态），由 main 打一次
	// warn 日志。它不含任何敏感值：只回显数字类设置项的原始输入。
	Warnings []string
}

// SendTimeout 是单次 QQ 接口调用的超时。
func (c Config) SendTimeout() time.Duration {
	seconds := c.SendTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultSendTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// CredentialsConfigured 报告凭据是否齐全。零配置时插件不建立任何外部连接。
func (c Config) CredentialsConfigured() bool {
	return strings.TrimSpace(c.AppID) != "" && strings.TrimSpace(c.AppSecret) != ""
}

// GroupIDClaimEnabled 报告是否应该建立群 ID 识别长连接。
func (c Config) GroupIDClaimEnabled() bool {
	return c.EnableGroupIDClaim && c.CredentialsConfigured()
}

// settingLookup 是设置读取的注入点。第二个返回值区分「键不存在」与「显式空串」：
// 宿主只注入保存过的键，所以「已保存为空」必须保留空值，而「从未保存」才回落默认值。
type settingLookup func(key string) (string, bool)

// LoadConfig 是 getenv 形态的入口（测试与调用方沿用）：返回非空即视为已设置，
// 因此它无法表达「保存为空」。生产路径用 LoadConfigLookup。
func LoadConfig(getenv func(string) string) Config {
	return LoadConfigLookup(func(key string) (string, bool) {
		if getenv == nil {
			return "", false
		}
		value := getenv(key)
		return value, value != ""
	})
}

// LoadConfigLookup 读取全部设置，并能区分「键不存在」与「显式空串」。生产入口传
// os.LookupEnv：空签名是合法配置，不该被默认值顶掉。
func LoadConfigLookup(lookup settingLookup) Config {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	// getenv 抹平存在性，供只关心「值是否为空」的解析器使用。
	getenv := func(key string) string {
		value, ok := lookup(key)
		if !ok {
			return ""
		}
		return value
	}
	config := Config{
		AppID:                strings.TrimSpace(getenv(settingPrefix + "APP_ID")),
		AppSecret:            strings.TrimSpace(getenv(settingPrefix + "APP_SECRET")),
		GroupIDs:             splitSettingList(getenv(settingPrefix + "GROUP_IDS")),
		CategoryFilter:       splitSettingList(getenv(settingPrefix + "CATEGORY_FILTER")),
		MessageFormat:        defaultMessageFormat,
		MessageTemplate:      defaultMessageTemplate,
		Signature:            defaultSignature,
		ExcerptLimit:         defaultExcerptLimit,
		MaxMessageChars:      defaultMaxMessageChars,
		DailyPushCapPerGroup: defaultDailyPushCapPerGroup,
		MaxPushesPerMinute:   defaultMaxPushesPerMinute,
		MaxRetries:           defaultMaxRetries,
		SendTimeoutSeconds:   defaultSendTimeoutSeconds,
		EnableGroupIDClaim:   defaultEnableGroupIDClaim,
		ReplyOnMention:       defaultReplyOnMention,
	}
	if raw := getenv(settingPrefix + "MESSAGE_TEMPLATE"); strings.TrimSpace(raw) != "" {
		config.MessageTemplate = raw
	}
	// 签名与清单描述一致：留空就渲染成空串。只有「键不存在」才用默认值——
	// 用 getenv 无法区分这两者，所以这里直接看 lookup 的存在性。
	if raw, ok := lookup(settingPrefix + "SIGNATURE"); ok {
		config.Signature = strings.TrimSpace(raw)
	}
	config.MessageFormat = parseFormat(getenv(settingPrefix+"MESSAGE_FORMAT"), &config)
	config.Paused = parseBoolSetting(getenv, "PAUSED", false, &config)
	config.EnableGroupIDClaim = parseBoolSetting(getenv, "ENABLE_GROUP_ID_CLAIM", defaultEnableGroupIDClaim, &config)
	config.ReplyOnMention = parseBoolSetting(getenv, "REPLY_ON_MENTION", defaultReplyOnMention, &config)
	config.LogVerbose = parseBoolSetting(getenv, "LOG_VERBOSE", false, &config)
	config.ExcerptLimit = parseIntSetting(getenv, "EXCERPT_LIMIT", defaultExcerptLimit, &config)
	config.MaxMessageChars = parseIntSetting(getenv, "MAX_MESSAGE_CHARS", defaultMaxMessageChars, &config)
	config.DailyPushCapPerGroup = parseIntSetting(getenv, "DAILY_PUSH_CAP_PER_GROUP", defaultDailyPushCapPerGroup, &config)
	config.MaxPushesPerMinute = parseIntSetting(getenv, "MAX_PUSHES_PER_MINUTE", defaultMaxPushesPerMinute, &config)
	config.MaxRetries = parseIntSetting(getenv, "MAX_RETRIES", defaultMaxRetries, &config)
	config.SendTimeoutSeconds = parseIntSetting(getenv, "SEND_TIMEOUT_SECONDS", defaultSendTimeoutSeconds, &config)
	return config
}

// splitSettingList 解析 textarea 形态的多值设置：按换行与英文逗号切分，`#` 开头的
// 片段是注释，去空白、去重并保持出现顺序。
func splitSettingList(raw string) []string {
	values := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, line := range strings.Split(raw, "\n") {
		for _, piece := range strings.Split(line, ",") {
			item := strings.TrimSpace(piece)
			if item == "" || strings.HasPrefix(item, "#") {
				continue
			}
			if _, duplicate := seen[item]; duplicate {
				continue
			}
			seen[item] = struct{}{}
			values = append(values, item)
		}
	}
	return values
}

func parseFormat(raw string, config *Config) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", messageFormatText, "plain", "plaintext":
		return messageFormatText
	case messageFormatMarkdown, "md":
		return messageFormatMarkdown
	default:
		config.Warnings = append(config.Warnings,
			fmt.Sprintf("设置项 %sMESSAGE_FORMAT 的值 %q 不是 text/markdown，已回落 %s",
				settingPrefix, raw, messageFormatText))
		return messageFormatText
	}
}

func parseBoolSetting(getenv func(string) string, key string, fallback bool, config *Config) bool {
	raw := strings.TrimSpace(getenv(settingPrefix + key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "true", "1", "on":
		return true
	case "false", "0", "off":
		return false
	default:
		config.Warnings = append(config.Warnings,
			fmt.Sprintf("设置项 %s%s 的值 %q 不是布尔值，已回落 %t", settingPrefix, key, raw, fallback))
		return fallback
	}
}

func parseIntSetting(getenv func(string) string, key string, fallback int, config *Config) int {
	raw := strings.TrimSpace(getenv(settingPrefix + key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		// 数字类设置的回显是安全的；解析错误串本身不打印（避免堆栈），只记一条 warn。
		config.Warnings = append(config.Warnings,
			fmt.Sprintf("设置项 %s%s 的值 %q 不是非负整数，已回落 %d", settingPrefix, key, raw, fallback))
		return fallback
	}
	return value
}

// logger 是插件的诊断输出。它同时负责脱敏：注册过的密钥在任何一行日志里都会被
// 替换成 ***，这是「app_secret 不出现在任何输出」的兜底，而不是唯一防线。
type logger struct {
	mu      sync.Mutex
	out     io.Writer
	verbose bool
	secrets []string
}

func newLogger(out io.Writer, verbose bool) *logger {
	return &logger{out: out, verbose: verbose}
}

// addSecret 登记一个需要在日志里抹掉的敏感值。
func (l *logger) addSecret(secret string) {
	if l == nil || len(strings.TrimSpace(secret)) < 4 {
		return
	}
	l.mu.Lock()
	l.secrets = append(l.secrets, secret)
	l.mu.Unlock()
}

func (l *logger) Warnf(format string, args ...any) {
	l.logf("warn", format, args...)
}

func (l *logger) Infof(format string, args ...any) {
	if l == nil || !l.verbose {
		return
	}
	l.logf("info", format, args...)
}

func (l *logger) Errorf(format string, args ...any) {
	l.logf("error", format, args...)
}

func (l *logger) logf(level string, format string, args ...any) {
	if l == nil || l.out == nil {
		return
	}
	line := fmt.Sprintf(format, args...)
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, secret := range l.secrets {
		line = strings.ReplaceAll(line, secret, "***")
	}
	fmt.Fprintf(l.out, "%s [%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), level, line)
}
