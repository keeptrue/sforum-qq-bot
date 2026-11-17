package main

import (
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 11, 14, 23, 1, 0, 0, time.Local)
}

func sampleData() TemplateData {
	return TemplateData{
		Title:     "新版本发布",
		Author:    "老后",
		Category:  "公告",
		Tags:      []string{"发布", "版本"},
		Excerpt:   "这一版修了很多东西",
		URL:       "https://forum.example.com/t/42",
		TopicID:   "42",
		Signature: "来自 SForum 论坛",
	}
}

func TestRenderAllVariables(t *testing.T) {
	template := "{{title}}|{{author}}|{{category}}|{{tags}}|{{excerpt}}|{{url}}|{{topicId}}|{{date}}|{{signature}}"
	rendered := Render(template, sampleData(), RenderOptions{Now: fixedNow})
	want := "新版本发布|老后|公告|发布 · 版本|这一版修了很多东西|https://forum.example.com/t/42|42|2026-11-14 23:01|来自 SForum 论坛"
	if rendered.Text != want {
		t.Fatalf("渲染结果不对：\nwant %q\ngot  %q", want, rendered.Text)
	}
	if len(rendered.Unknown) != 0 || rendered.Truncated {
		t.Fatalf("不应有未知变量或截断：%+v", rendered)
	}
}

func TestRenderChineseAliases(t *testing.T) {
	template := "标题={{标题}} 作者={{作者}} 分类={{分类}} 标签={{标签}} 摘要={{摘要}} 链接={{链接}} 主题ID={{主题ID}} 日期={{日期}} 签名={{签名}}"
	rendered := Render(template, sampleData(), RenderOptions{Now: fixedNow})
	for _, want := range []string{"标题=新版本发布", "作者=老后", "分类=公告", "标签=发布 · 版本",
		"摘要=这一版修了很多东西", "链接=https://forum.example.com/t/42", "主题ID=42",
		"日期=2026-11-14 23:01", "签名=来自 SForum 论坛"} {
		if !strings.Contains(rendered.Text, want) {
			t.Fatalf("中文别名渲染缺少 %q：%q", want, rendered.Text)
		}
	}
	if len(rendered.Unknown) != 0 {
		t.Fatalf("中文别名不应算未知变量：%v", rendered.Unknown)
	}
}

func TestRenderUnknownVariablesKeptAndWarnedOnce(t *testing.T) {
	var warnings []string
	rendered := Render("{{title}} {{txet}} {{txet}} {{nope}}", sampleData(), RenderOptions{
		Now: fixedNow,
		Warnf: func(format string, args ...any) {
			warnings = append(warnings, format)
		},
	})
	if !strings.Contains(rendered.Text, "{{txet}}") || !strings.Contains(rendered.Text, "{{nope}}") {
		t.Fatalf("未知变量应原样保留：%q", rendered.Text)
	}
	if len(rendered.Unknown) != 2 || rendered.Unknown[0] != "txet" || rendered.Unknown[1] != "nope" {
		t.Fatalf("未知变量清单不对：%v", rendered.Unknown)
	}
	if len(warnings) != 2 {
		t.Fatalf("每个未知变量应只 warn 一次，得到 %d 条：%v", len(warnings), warnings)
	}
}

func TestRenderMarkdownEscaping(t *testing.T) {
	data := sampleData()
	data.Title = "a*b_c[d](e)#f+g-h.i!j>k~l|m`n\\o"
	rendered := Render("{{title}}", data, RenderOptions{Format: messageFormatMarkdown, Now: fixedNow})
	want := "a\\*b\\_c\\[d\\]\\(e\\)\\#f\\+g\\-h\\.i\\!j\\>k\\~l\\|m\\`n\\\\o"
	if rendered.Text != want {
		t.Fatalf("Markdown 转义不对：\nwant %q\ngot  %q", want, rendered.Text)
	}

	plain := Render("{{title}}", data, RenderOptions{Format: messageFormatText, Now: fixedNow})
	if plain.Text != data.Title {
		t.Fatalf("纯文本不应转义：%q", plain.Text)
	}
}

func TestRenderCompressesNewlinesInValues(t *testing.T) {
	data := sampleData()
	data.Excerpt = "第一行\n第二行\r\n\n第三行"
	rendered := Render("摘要：{{excerpt}}", data, RenderOptions{Now: fixedNow})
	if rendered.Text != "摘要：第一行 第二行 第三行" {
		t.Fatalf("变量值内换行应压成单空格：%q", rendered.Text)
	}
	if strings.Contains(rendered.Text, "\n") {
		t.Fatalf("渲染结果不应残留换行：%q", rendered.Text)
	}
}

func TestRenderTruncatesLongMessage(t *testing.T) {
	data := sampleData()
	data.Title = strings.Repeat("长", 30)
	rendered := Render("{{title}}", data, RenderOptions{MaxChars: 10, Now: fixedNow})
	if !rendered.Truncated {
		t.Fatal("超长消息应标记截断")
	}
	if want := strings.Repeat("长", 10) + "…"; rendered.Text != want {
		t.Fatalf("截断结果不对：%q", rendered.Text)
	}
}

func TestRenderKeepsShortMessage(t *testing.T) {
	rendered := Render("短消息", sampleData(), RenderOptions{MaxChars: 10, Now: fixedNow})
	if rendered.Truncated || rendered.Text != "短消息" {
		t.Fatalf("短消息不应被改动：%+v", rendered)
	}
}

func TestBuildExcerpt(t *testing.T) {
	if got := BuildExcerpt("  多余   空白\n\n与换行  ", 10); got != "多余 空白 与换行" {
		t.Fatalf("摘要空白压缩不对：%q", got)
	}
	if got := BuildExcerpt("一二三四五", 3); got != "一二三…" {
		t.Fatalf("摘要截断应按 rune：%q", got)
	}
	if got := BuildExcerpt("一二三", 3); got != "一二三" {
		t.Fatalf("恰好等于上限不应加省略号：%q", got)
	}
	if got := BuildExcerpt("一二三", 0); got != "" {
		t.Fatalf("limit=0 应视为空摘要：%q", got)
	}
	if got := BuildExcerpt("", 10); got != "" {
		t.Fatalf("空正文应得到空摘要：%q", got)
	}
}

func TestTemplateMentions(t *testing.T) {
	if !TemplateMentions("摘要：{{excerpt}}", templateVarExcerpt, "摘要") {
		t.Fatal("应识别英文变量")
	}
	if !TemplateMentions("摘要：{{ 摘要 }}", templateVarExcerpt, "摘要") {
		t.Fatal("应识别中文别名与两侧空白")
	}
	if TemplateMentions("只有 {{title}}", templateVarExcerpt, "摘要") {
		t.Fatal("不应把 title 当成摘要变量")
	}
}
