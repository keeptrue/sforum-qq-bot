package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// env 构造一个只认识给定键的 getenv。
func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigDefaults(t *testing.T) {
	config := LoadConfig(env(nil))
	if config.AppID != "" || config.AppSecret != "" {
		t.Fatalf("默认凭据应为空，得到 %q/%q", config.AppID, config.AppSecret)
	}
	if len(config.GroupIDs) != 0 || len(config.CategoryFilter) != 0 {
		t.Fatalf("默认白名单应为空，得到 %v / %v", config.GroupIDs, config.CategoryFilter)
	}
	if config.ExcerptLimit != 120 {
		t.Fatalf("excerpt 默认 120，得到 %d", config.ExcerptLimit)
	}
	if config.MaxMessageChars != 1000 {
		t.Fatalf("maxChars 默认 1000，得到 %d", config.MaxMessageChars)
	}
	if config.Signature != "来自 SForum 论坛" {
		t.Fatalf("签名默认值不对：%q", config.Signature)
	}
	if config.DailyPushCapPerGroup != 800 || config.MaxPushesPerMinute != 20 || config.MaxRetries != 2 {
		t.Fatalf("限流默认值不对：%d/%d/%d",
			config.DailyPushCapPerGroup, config.MaxPushesPerMinute, config.MaxRetries)
	}
	if config.SendTimeoutSeconds != 10 {
		t.Fatalf("超时默认 10 秒，得到 %d", config.SendTimeoutSeconds)
	}
	if !config.EnableGroupIDClaim {
		t.Fatal("群 ID 识别默认应为开启")
	}
	if config.MessageFormat != messageFormatText {
		t.Fatalf("消息形态默认 text，得到 %q", config.MessageFormat)
	}
	if !strings.Contains(config.MessageTemplate, "{{title}}") || !strings.Contains(config.MessageTemplate, "{{url}}") {
		t.Fatalf("默认模板缺少核心变量：%q", config.MessageTemplate)
	}
	if config.Paused {
		t.Fatal("默认不应暂停")
	}
	if len(config.Warnings) != 0 {
		t.Fatalf("干净环境不应有解析告警：%v", config.Warnings)
	}
	if config.CredentialsConfigured() || config.GroupIDClaimEnabled() {
		t.Fatal("零配置下不应认为凭据齐全或该建立长连接")
	}
}

func TestSplitSettingList(t *testing.T) {
	raw := "GROUP_A\nGROUP_B, GROUP_C\n  GROUP_A  \n# 这一行是注释\nGROUP_D, #尾注释\n,,"
	values := splitSettingList(raw)
	want := []string{"GROUP_A", "GROUP_B", "GROUP_C", "GROUP_D"}
	if len(values) != len(want) {
		t.Fatalf("期望 %v，得到 %v", want, values)
	}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("第 %d 项期望 %q，得到 %q（全量 %v）", index, want[index], values[index], values)
		}
	}
	if got := splitSettingList(""); len(got) != 0 {
		t.Fatalf("空串应解析出空列表，得到 %v", got)
	}
}

func TestLoadConfigTextareas(t *testing.T) {
	config := LoadConfig(env(map[string]string{
		settingPrefix + "GROUP_IDS":       " g1 \n g2,g1\n#c,g3",
		settingPrefix + "CATEGORY_FILTER": "news\nannounce,news",
	}))
	if strings.Join(config.GroupIDs, "|") != "g1|g2|g3" {
		t.Fatalf("群白名单解析不对：%v", config.GroupIDs)
	}
	if strings.Join(config.CategoryFilter, "|") != "news|announce" {
		t.Fatalf("分类过滤解析不对：%v", config.CategoryFilter)
	}
}

func TestLoadConfigInvalidNumbersFallBack(t *testing.T) {
	config := LoadConfig(env(map[string]string{
		settingPrefix + "EXCERPT_LIMIT":     "abc",
		settingPrefix + "MAX_RETRIES":       "-3",
		settingPrefix + "MAX_MESSAGE_CHARS": "2.5",
	}))
	if config.ExcerptLimit != defaultExcerptLimit {
		t.Fatalf("非法数字应回落默认值，得到 %d", config.ExcerptLimit)
	}
	if config.MaxRetries != defaultMaxRetries {
		t.Fatalf("负数应回落默认值，得到 %d", config.MaxRetries)
	}
	if config.MaxMessageChars != defaultMaxMessageChars {
		t.Fatalf("小数应回落默认值，得到 %d", config.MaxMessageChars)
	}
	if len(config.Warnings) < 3 {
		t.Fatalf("三个非法数字应各记一条 warn，得到 %v", config.Warnings)
	}
}

func TestLoadConfigNumbersAndBooleans(t *testing.T) {
	config := LoadConfig(env(map[string]string{
		settingPrefix + "EXCERPT_LIMIT":            "0",
		settingPrefix + "MAX_MESSAGE_CHARS":        "750",
		settingPrefix + "DAILY_PUSH_CAP_PER_GROUP": "10",
		settingPrefix + "MAX_PUSHES_PER_MINUTE":    "5",
		settingPrefix + "MAX_RETRIES":              "0",
		settingPrefix + "SEND_TIMEOUT_SECONDS":     "3",
		settingPrefix + "PAUSED":                   "ON",
		settingPrefix + "ENABLE_GROUP_ID_CLAIM":    "0",
		settingPrefix + "LOG_VERBOSE":              "True",
		settingPrefix + "MESSAGE_FORMAT":           "Markdown",
	}))
	if config.ExcerptLimit != 0 || config.MaxMessageChars != 750 || config.MaxRetries != 0 {
		t.Fatalf("数字设置解析不对：%d/%d/%d", config.ExcerptLimit, config.MaxMessageChars, config.MaxRetries)
	}
	if !config.Paused {
		t.Fatal("PAUSED=ON 应解析为 true")
	}
	if config.EnableGroupIDClaim {
		t.Fatal("ENABLE_GROUP_ID_CLAIM=0 应解析为 false")
	}
	if !config.LogVerbose {
		t.Fatal("LOG_VERBOSE=True 应解析为 true")
	}
	if config.MessageFormat != messageFormatMarkdown {
		t.Fatalf("消息形态应解析为 markdown，得到 %q", config.MessageFormat)
	}
	if config.SendTimeout() != 3e9 {
		t.Fatalf("发送超时应是 3 秒，得到 %v", config.SendTimeout())
	}
}

func TestLoadConfigInvalidBooleanFallsBackWithWarning(t *testing.T) {
	config := LoadConfig(env(map[string]string{
		settingPrefix + "PAUSED":         "yes",
		settingPrefix + "MESSAGE_FORMAT": "html",
		settingPrefix + "APP_SECRET":     "super-secret-value",
	}))
	if config.Paused {
		t.Fatal("非法布尔值应回落 false")
	}
	if len(config.Warnings) != 2 {
		t.Fatalf("应记两条 warn（布尔 + 形态），得到 %v", config.Warnings)
	}
	for _, warning := range config.Warnings {
		if strings.Contains(warning, "super-secret-value") {
			t.Fatalf("告警里泄露了密钥：%q", warning)
		}
	}
}

func TestLoggerRedactsSecrets(t *testing.T) {
	var buffer bytes.Buffer
	log := newLogger(&buffer, true)
	log.addSecret("super-secret-value")
	log.Warnf("请求失败：secret=%s", "super-secret-value")
	log.Infof("另一行 %s", "super-secret-value")
	output := buffer.String()
	if strings.Contains(output, "super-secret-value") {
		t.Fatalf("日志泄露密钥：%q", output)
	}
	if strings.Count(output, "***") != 2 {
		t.Fatalf("两行都应被脱敏：%q", output)
	}
}

func TestLoggerVerboseGatesInfo(t *testing.T) {
	var buffer bytes.Buffer
	log := newLogger(&buffer, false)
	log.Infof("不该出现")
	if buffer.Len() != 0 {
		t.Fatalf("非 verbose 模式不应输出 info：%q", buffer.String())
	}
	log.Warnf("该出现")
	if !strings.Contains(buffer.String(), "该出现") {
		t.Fatalf("warn 应始终输出：%q", buffer.String())
	}
}

// signature 语义与清单描述一致：留空就是空串，只有「键不存在」才用默认值。
func TestLoadConfigSignatureSemantics(t *testing.T) {
	signatureLookup := func(value string, present bool) settingLookup {
		return func(key string) (string, bool) {
			if key != settingPrefix+"SIGNATURE" {
				return "", false
			}
			return value, present
		}
	}

	if got := LoadConfigLookup(signatureLookup("", true)).Signature; got != "" {
		t.Fatalf("显式空签名应渲染成空串，得到 %q", got)
	}
	if got := LoadConfigLookup(signatureLookup("   ", true)).Signature; got != "" {
		t.Fatalf("只有空白的签名也应压成空串，得到 %q", got)
	}
	if got := LoadConfigLookup(signatureLookup("", false)).Signature; got != defaultSignature {
		t.Fatalf("键不存在时应回落默认签名，得到 %q", got)
	}
	if got := LoadConfigLookup(signatureLookup("—— 来自老后", true)).Signature; got != "—— 来自老后" {
		t.Fatalf("自定义签名应生效，得到 %q", got)
	}
	// getenv 形态无法表达「存在但为空」，空串按未设置处理。
	if got := LoadConfig(env(map[string]string{settingPrefix + "SIGNATURE": ""})).Signature; got != defaultSignature {
		t.Fatalf("getenv 形态下空串按未设置处理，得到 %q", got)
	}
}

// 生产入口用的是 os.LookupEnv：宿主注入的空值必须被当成「显式空串」。
func TestLoadConfigProductionLookupHonorsEmptySignature(t *testing.T) {
	t.Setenv(settingPrefix+"SIGNATURE", "")
	if got := LoadConfigLookup(os.LookupEnv).Signature; got != "" {
		t.Fatalf("宿主注入空签名时必须保留空串，得到 %q", got)
	}
	t.Setenv(settingPrefix+"MESSAGE_TEMPLATE", "只有标题：{{title}}")
	if got := LoadConfigLookup(os.LookupEnv).MessageTemplate; got != "只有标题：{{title}}" {
		t.Fatalf("模板应沿用注入值，得到 %q", got)
	}
}
