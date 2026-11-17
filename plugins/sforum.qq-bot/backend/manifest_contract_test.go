package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// 清单是契约真源。这组测试直接读包清单与设置文档，比对 backend 里的常量：
// 任何一侧漂移都让 `go test` 立刻变红。
//
// 它存在的理由很具体：HookId 取错字段（拿 handler 比事件 id）曾让 topic.created
// 全部被拒，而当时的 vet/test/宿主 validate 全绿——门禁不覆盖这类跨文件一致性。
const (
	manifestPath = "../sforum.extension.json"
	settingsPath = "../manifest/settings.json"
)

type manifestEvent struct {
	ID              string `json:"id"`
	Handler         string `json:"handler"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	ContractVersion string `json:"contractVersion"`
	InputSchema     string `json:"inputSchema"`
	ResultSchema    string `json:"resultSchema"`
}

type manifestRoute struct {
	ID              string   `json:"id"`
	Handler         string   `json:"handler"`
	Path            string   `json:"path"`
	Method          string   `json:"method"`
	ContractVersion string   `json:"contractVersion"`
	RequestSchema   string   `json:"requestSchema"`
	ResponseSchema  string   `json:"responseSchema"`
	Methods         []string `json:"methods"`
	Access          string   `json:"access"`
	Fallback        string   `json:"fallback"`
	Guard           string   `json:"guard"`
	Permission      string   `json:"permission"`
}

type manifestToolParameterSet struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

type manifestTool struct {
	ID              string                   `json:"id"`
	Handler         string                   `json:"handler"`
	Name            string                   `json:"name"`
	ContractVersion string                   `json:"contractVersion"`
	TimeoutMS       int                      `json:"timeoutMs"`
	Parameters      manifestToolParameterSet `json:"parameters"`
}

type extensionManifest struct {
	ID      string          `json:"id"`
	Version string          `json:"version"`
	Events  []manifestEvent `json:"events"`
	Routes  []manifestRoute `json:"routes"`
	AITools []manifestTool  `json:"aiTools"`
}

type settingField struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Default string `json:"default"`
}

type settingsDocument struct {
	Fields []settingField `json:"fields"`
}

func loadExtensionManifest(t *testing.T) extensionManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		t.Fatalf("读不到扩展清单 %s：%v", manifestPath, err)
	}
	var manifest extensionManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("清单不是合法 JSON：%v", err)
	}
	if manifest.ID == "" || len(manifest.Events) == 0 || len(manifest.Routes) == 0 || len(manifest.AITools) == 0 {
		t.Fatalf("清单缺少 id/events/routes/aiTools，解析路径可能不对：%+v", manifest.ID)
	}
	return manifest
}

func loadSettingsDocument(t *testing.T) settingsDocument {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(settingsPath))
	if err != nil {
		t.Fatalf("读不到设置文档 %s：%v", settingsPath, err)
	}
	var document settingsDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("设置文档不是合法 JSON：%v", err)
	}
	if len(document.Fields) == 0 {
		t.Fatalf("设置文档没有 fields：%s", settingsPath)
	}
	return document
}

// TestManifestContract 是清单与代码的逐字比对入口。
func TestManifestContract(t *testing.T) {
	manifest := loadExtensionManifest(t)
	settings := loadSettingsDocument(t)

	t.Run("extension id", func(t *testing.T) {
		if manifest.ID != extensionID {
			t.Fatalf("扩展 id 漂移：清单 %q，backend %q", manifest.ID, extensionID)
		}
	})

	t.Run("events", func(t *testing.T) {
		if len(manifest.Events) != 1 {
			t.Fatalf("清单应声明 1 个事件，实际 %d 个", len(manifest.Events))
		}
		event := manifest.Events[0]
		// HookId 是事件 id；handler 只用于宿主侧派发登记。
		if event.ID != observeEventID {
			t.Fatalf("事件 id 漂移：清单 %q，backend observeEventID %q", event.ID, observeEventID)
		}
		if event.Handler != observeHandlerID {
			t.Fatalf("事件 handler 漂移：清单 %q，backend %q", event.Handler, observeHandlerID)
		}
		if event.Name != observeEventName {
			t.Fatalf("事件名漂移：清单 %q，backend %q", event.Name, observeEventName)
		}
		if event.Kind != "observe" {
			t.Fatalf("事件类型应为 observe，清单写的是 %q", event.Kind)
		}
		if event.InputSchema != observeInputSchemaRef {
			t.Fatalf("事件 inputSchema 漂移：清单 %q，backend %q", event.InputSchema, observeInputSchemaRef)
		}
		if event.ResultSchema != observeResultSchemaRef {
			t.Fatalf("事件 resultSchema 漂移：清单 %q，backend %q", event.ResultSchema, observeResultSchemaRef)
		}
		if event.ContractVersion != observeEventID+"@1" {
			t.Fatalf("事件 contractVersion 应为 %s@1，清单写的是 %q", observeEventID, event.ContractVersion)
		}
	})

	t.Run("routes", func(t *testing.T) {
		// 期望值全部来自 backend 常量：清单里少一条、多一条、改一个字符都会红。
		expected := []struct {
			ID             string
			RequestSchema  string
			ResponseSchema string
			Path           string
		}{
			{routeBindStartID, bindStartInputSchema, bindStartOutputSchema, "/bind/start"},
			{routeBindPollID, bindPollInputSchema, bindPollOutputSchema, "/bind/poll"},
			{routePreviewID, previewInputSchema, previewOutputSchema, "/preview"},
			{routeTestPushID, testPushInputSchema, testPushOutputSchema, "/test-push"},
			{routeStatusID, statusInputSchema, statusOutputSchema, "/status"},
			{routeGroupsListID, groupsListInputSchema, groupsListOutputSchema, "/groups/list"},
			{routeAccountStatusID, accountStatusInputSchema, accountStatusOutputSchema, "/account/status"},
		}
		if len(manifest.Routes) != len(expected) {
			t.Fatalf("清单应声明 %d 条路由，实际 %d 条", len(expected), len(manifest.Routes))
		}
		for _, want := range expected {
			route, found := findManifestRoute(manifest.Routes, want.ID)
			if !found {
				t.Fatalf("清单缺少路由 %s", want.ID)
			}
			// 清单要求 handler 与 id 相同；宿主按 handler 登记、按 id 下发 RouteId。
			if route.Handler != route.ID {
				t.Fatalf("路由 %s 的 handler 必须等于 id，清单写的是 %q", want.ID, route.Handler)
			}
			if route.RequestSchema != want.RequestSchema {
				t.Fatalf("路由 %s 的 requestSchema 漂移：清单 %q，backend %q",
					want.ID, route.RequestSchema, want.RequestSchema)
			}
			if route.ResponseSchema != want.ResponseSchema {
				t.Fatalf("路由 %s 的 responseSchema 漂移：清单 %q，backend %q",
					want.ID, route.ResponseSchema, want.ResponseSchema)
			}
			if route.Path != want.Path {
				t.Fatalf("路由 %s 的 path 漂移：清单 %q，backend 约定 %q", want.ID, route.Path, want.Path)
			}
			if route.ContractVersion != want.ID+"@1" {
				t.Fatalf("路由 %s 的 contractVersion 应为 %s@1，清单写的是 %q",
					want.ID, want.ID, route.ContractVersion)
			}
		}
	})

	t.Run("ai tools", func(t *testing.T) {
		definitions := aiToolDefinitions(newRouteTestPlugin(t, testConfig(), nil))
		if len(definitions) != len(manifest.AITools) {
			t.Fatalf("清单与 backend 的工具数量不一致：清单 %d 个，backend %d 个",
				len(manifest.AITools), len(definitions))
		}
		for _, definition := range definitions {
			tool, found := findManifestTool(manifest.AITools, definition.ID)
			if !found {
				t.Fatalf("清单缺少 AI 工具 %s", definition.ID)
			}
			if definition.ContractVersion != tool.ContractVersion {
				t.Fatalf("工具 %s 的 contractVersion 漂移：清单 %q，backend %q",
					definition.ID, tool.ContractVersion, definition.ContractVersion)
			}
			if definition.ContractVersion != definition.ID+"@1" {
				t.Fatalf("工具 %s 的 contractVersion 应为 id@1，backend 写的是 %q",
					definition.ID, definition.ContractVersion)
			}
			if tool.Handler != tool.ID {
				t.Fatalf("工具 %s 的 handler 必须等于 id，清单写的是 %q", tool.ID, tool.Handler)
			}
			if definition.Name != tool.Name {
				t.Fatalf("工具 %s 的 name 漂移：清单 %q，backend %q",
					definition.ID, tool.Name, definition.Name)
			}
			if definition.TimeoutMS != tool.TimeoutMS {
				t.Fatalf("工具 %s 的 timeoutMs 漂移：清单 %d，backend %d",
					definition.ID, tool.TimeoutMS, definition.TimeoutMS)
			}
			if got, want := parameterPropertyNames(t, definition.Parameters), sortedKeys(tool.Parameters.Properties); !equalStringSlices(got, want) {
				t.Fatalf("工具 %s 的参数集合漂移：\nbackend %v\n清单    %v", definition.ID, got, want)
			}
		}
	})

	t.Run("settings defaults", func(t *testing.T) {
		// 缺省值必须与清单 settings[].default 逐字一致：宿主只注入保存过的键，
		// 未保存时用的就是这里的默认值。
		expected := map[string]string{
			"app_id":                   "",
			"app_secret":               "",
			"message_format":           defaultMessageFormat,
			"message_template":         defaultMessageTemplate,
			"signature":                defaultSignature,
			"excerpt_limit":            strconv.Itoa(defaultExcerptLimit),
			"max_message_chars":        strconv.Itoa(defaultMaxMessageChars),
			"daily_push_cap_per_group": strconv.Itoa(defaultDailyPushCapPerGroup),
			"max_pushes_per_minute":    strconv.Itoa(defaultMaxPushesPerMinute),
			"max_retries":              strconv.Itoa(defaultMaxRetries),
			"send_timeout_seconds":     strconv.Itoa(defaultSendTimeoutSeconds),
			"enable_group_id_claim":    strconv.FormatBool(defaultEnableGroupIDClaim),
			"reply_on_mention":         strconv.FormatBool(defaultReplyOnMention),
			"paused":                   "false",
			"log_verbose":              "false",
		}
		for key, want := range expected {
			field, found := findSettingField(settings.Fields, key)
			if !found {
				t.Fatalf("设置文档缺少字段 %s", key)
			}
			if field.Default != want {
				t.Fatalf("设置项 %s 的默认值与 backend 不一致：\n清单   %q\nbackend %q",
					key, field.Default, want)
			}
		}
	})
}

func findManifestRoute(routes []manifestRoute, id string) (manifestRoute, bool) {
	for _, route := range routes {
		if route.ID == id {
			return route, true
		}
	}
	return manifestRoute{}, false
}

func findManifestTool(tools []manifestTool, id string) (manifestTool, bool) {
	for _, tool := range tools {
		if tool.ID == id {
			return tool, true
		}
	}
	return manifestTool{}, false
}

func findSettingField(fields []settingField, key string) (settingField, bool) {
	for _, field := range fields {
		if field.Key == key {
			return field, true
		}
	}
	return settingField{}, false
}

// parameterPropertyNames 解析 backend 声明里的参数名集合（已排序）。
func parameterPropertyNames(t *testing.T, parameters string) []string {
	t.Helper()
	var declared struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(parameters), &declared); err != nil {
		t.Fatalf("backend 工具参数不是合法 JSON：%v", err)
	}
	names := make([]string, 0, len(declared.Properties))
	for name := range declared.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
