package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
)

// 宿主按字节截断 4096：clamp 必须保证「输出 ≤ 预算」且永远是合法 UTF-8，
// 不能像按 rune 计数那样在中文上超三倍，也不能切出半个字符。
func TestClampTextRespectsByteBudget(t *testing.T) {
	long := strings.Repeat("状态输出", 100) // 每个汉字 3 字节，共 1200 字节
	for _, limit := range []int{1, 5, 25, 26, 40, 100, 1200, 3500} {
		output := clampText(long, limit)
		if len(output) > limit {
			t.Fatalf("limit=%d 时输出 %d 字节，超出预算", limit, len(output))
		}
		if !utf8.ValidString(output) {
			t.Fatalf("limit=%d 时切出了非法 UTF-8：%q", limit, output)
		}
	}
	if output := clampText(long, 100); !strings.HasSuffix(output, "已截断）") {
		t.Fatalf("预算够时应带截断提示：%q", output)
	}
}

func TestClampTextKeepsShortAndUnlimited(t *testing.T) {
	if got := clampText("短消息", 100); got != "短消息" {
		t.Fatalf("未超预算应原样返回：%q", got)
	}
	long := strings.Repeat("字", 50)
	if got := clampText(long, 0); got != long {
		t.Fatal("limit<=0 表示不限制")
	}
	// 预算连省略号都装不下：只切正文，仍然不超预算。
	if got := clampText(long, 4); len(got) > 4 || !utf8.ValidString(got) {
		t.Fatalf("极小预算也应安全：%q（%d 字节）", got, len(got))
	}
}

// 声明必须能真的注册成功：main.go 在注册失败时只记一条 error 然后跳过全部工具，
// 那会让设置页按钮静默失效。这条测试把「声明合法」变成门禁。
func TestAIToolDefinitionsRegister(t *testing.T) {
	instance := newRouteTestPlugin(t, testConfig(), nil)
	registry, err := pluginv2.NewAIToolRegistry(aiToolDefinitions(instance)...)
	if err != nil {
		t.Fatalf("AI 工具声明注册失败：%v", err)
	}
	registered := registry.Definitions()
	if len(registered) != 3 {
		t.Fatalf("应注册 3 个工具，实际 %d 个", len(registered))
	}
	for _, definition := range registered {
		if definition.TimeoutMS != aiToolTimeoutMS {
			t.Fatalf("工具 %s 的超时与应用声明不一致：%d", definition.ID, definition.TimeoutMS)
		}
	}
}
