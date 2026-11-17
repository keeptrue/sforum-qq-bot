package main

import (
	"context"
	"strings"
	"testing"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
)

// 预览工具必须已声明，且 id / 契约版本与清单 aiTools[] 逐字一致。
func TestPreviewToolDeclared(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	definitions := aiToolDefinitions(instance)
	var found *pluginv2.AIToolDefinition
	for index := range definitions {
		if definitions[index].ID == toolPreviewID {
			found = &definitions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("未声明预览工具 %s", toolPreviewID)
	}
	if found.ContractVersion != toolPreviewID+"@1" {
		t.Fatalf("契约版本不对：%q", found.ContractVersion)
	}
	if found.Name != "qq-bot-preview" {
		t.Fatalf("工具名不对：%q", found.Name)
	}
	if found.Execute == nil {
		t.Fatal("预览工具没有执行函数")
	}
	if !strings.Contains(found.Parameters, "message_template") ||
		!strings.Contains(found.Parameters, "max_message_chars") {
		t.Fatalf("参数声明缺字段：%s", found.Parameters)
	}
}

// 预览工具只渲染：正文里是渲染结果，IsError 保持 false。
func TestPreviewToolRenders(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{
		Arguments: `{"message_template":"【{{title}}】{{signature}}","signature":"—— 来自测试"}`,
	})
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	if result.IsError {
		t.Fatalf("渲染成功不该标 IsError：%s", result.Content)
	}
	if !strings.Contains(result.Content, "【测试】这是一条来自 SForum 的推送样例") {
		t.Fatalf("样例标题没渲染出来：%s", result.Content)
	}
	if !strings.Contains(result.Content, "—— 来自测试") {
		t.Fatalf("草稿签名没生效：%s", result.Content)
	}
}

// 未知变量与截断要在正文里提示，但不改变 IsError。
func TestPreviewToolReportsUnknownAndTruncation(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{
		Arguments: `{"message_template":"{{title}}{{txet}}","max_message_chars":"6"}`,
	})
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	if result.IsError {
		t.Fatalf("未知变量是编辑中间态，不该标 IsError：%s", result.Content)
	}
	if !strings.Contains(result.Content, "{{txet}}") {
		t.Fatalf("应提示未知变量：%s", result.Content)
	}
	if !strings.Contains(result.Content, "已按 6 字截断") {
		t.Fatalf("应提示截断：%s", result.Content)
	}
}

// 参数不可解析才置 IsError。
func TestPreviewToolBadArguments(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{Arguments: "not json"})
	if err != nil {
		t.Fatalf("参数错误不该返回 error：%v", err)
	}
	if !result.IsError {
		t.Fatal("参数不可解析必须标 IsError")
	}
	if !strings.Contains(result.Content, "参数无法解析") {
		t.Fatalf("文案不对：%s", result.Content)
	}
}

// 空模板按空串渲染：正文为空，且不带「未知变量」「已按 N 字截断」后缀。
func TestPreviewToolEmptyTemplateRendersEmpty(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{
		Arguments: `{"message_template":""}`,
	})
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	if result.IsError {
		t.Fatalf("空模板是编辑中间态，不该标 IsError：%s", result.Content)
	}
	if result.Content != "" {
		t.Fatalf("空模板应渲染空串，得到 %q", result.Content)
	}
}

// 缺键才回落：参数里没有 message_template 时仍用已保存模板渲染样例。
func TestPreviewToolFallsBackWhenTemplateAbsent(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{Arguments: `{}`})
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	if result.IsError {
		t.Fatalf("回落不该标 IsError：%s", result.Content)
	}
	if !strings.Contains(result.Content, sampleTemplateData("").Title) {
		t.Fatalf("缺键应回落已保存模板并渲染样例标题：%s", result.Content)
	}
	if !strings.Contains(result.Content, "【新主题】") {
		t.Fatalf("回落的应是已保存模板：%s", result.Content)
	}
}

// 清空签名同样按空串渲染，而不是回落到已保存签名。
func TestPreviewToolEmptySignatureRendersEmpty(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{
		Arguments: `{"message_template":"{{signature}}","signature":""}`,
	})
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	if result.IsError {
		t.Fatalf("空签名不该标 IsError：%s", result.Content)
	}
	if result.Content != "" {
		t.Fatalf("空签名应渲染空串，得到 %q", result.Content)
	}
}

// 摘要上限 0 表示不显示摘要，与真实推送语义一致。
func TestPreviewToolExcerptLimitZero(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	result, err := instance.previewHandler(context.Background(), &pluginv2.AIToolCall{
		Arguments: `{"message_template":"摘要：{{excerpt}}","excerpt_limit":"0"}`,
	})
	if err != nil {
		t.Fatalf("预览失败：%v", err)
	}
	if strings.TrimSpace(result.Content) != "摘要：" {
		t.Fatalf("摘要上限为 0 时应为空，得到 %q", result.Content)
	}
}
