package main

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// 模板变量：中文名只是英文名的别名，两者指向同一个值。
const (
	templateVarTitle     = "title"
	templateVarAuthor    = "author"
	templateVarCategory  = "category"
	templateVarTags      = "tags"
	templateVarExcerpt   = "excerpt"
	templateVarURL       = "url"
	templateVarTopicID   = "topicId"
	templateVarDate      = "date"
	templateVarSignature = "signature"
)

const templateTagSeparator = " · "

// markdownEscapeChars 是需要逐字符加反斜杠的 Markdown 元字符集合。
//
// 刻意不转义空格：`\ ` 在 Markdown 里表示「转义空格」，渲染结果不变，却会让正文
// 出现一串反斜杠。也不转义中文标点：它们不是 Markdown 语法。
const markdownEscapeChars = "\\`*_[]()#+-.!>~|"

// templateVariablePattern 匹配 {{name}}，允许两侧空白。
var templateVariablePattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// TemplateData 是渲染输入。日期留空时取渲染时刻（进程时区），测试可显式给定。
type TemplateData struct {
	Title     string
	Author    string
	Category  string
	Tags      []string
	Excerpt   string
	URL       string
	TopicID   string
	Signature string
	Date      string
}

// RenderOptions 汇集渲染期策略。
type RenderOptions struct {
	Format   string
	MaxChars int
	Now      func() time.Time
	Warnf    func(format string, args ...any)
}

// Rendered 是渲染结果。Unknown 按出现顺序去重，Truncated 表示触发了总长度截断。
type Rendered struct {
	Text      string
	Unknown   []string
	Truncated bool
}

// Render 替换模板变量：未知变量原样保留并各记一次 warn，Markdown 形态下对变量值
// 转义元字符，最后按 MaxChars 截断。
func Render(template string, data TemplateData, opts RenderOptions) Rendered {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	values := templateValues(data, now())
	markdown := strings.EqualFold(strings.TrimSpace(opts.Format), messageFormatMarkdown)

	unknown := make([]string, 0, 2)
	seenUnknown := make(map[string]struct{}, 2)
	rendered := templateVariablePattern.ReplaceAllStringFunc(template, func(match string) string {
		name := strings.TrimSpace(match[2 : len(match)-2])
		value, ok := values[name]
		if !ok {
			if _, duplicate := seenUnknown[name]; !duplicate {
				seenUnknown[name] = struct{}{}
				unknown = append(unknown, name)
			}
			// 未知变量原样保留：模板拼错时管理员能在面板里看见 {{txet}} 而不是静默消失。
			return match
		}
		value = collapseWhitespace(value)
		if markdown {
			value = escapeMarkdown(value)
		}
		return value
	})
	if opts.Warnf != nil {
		for _, name := range unknown {
			opts.Warnf("qq-bot: 模板变量 {{%s}} 未知，已原样保留", name)
		}
	}
	text, truncated := truncateRunes(rendered, opts.MaxChars)
	return Rendered{Text: text, Unknown: unknown, Truncated: truncated}
}

// TemplateMentions 报告模板是否引用了任一变量名（含中文别名）。
func TemplateMentions(template string, names ...string) bool {
	for _, match := range templateVariablePattern.FindAllStringSubmatch(template, -1) {
		name := strings.TrimSpace(match[1])
		for _, candidate := range names {
			if name == candidate {
				return true
			}
		}
	}
	return false
}

// BuildExcerpt 把正文压成单行摘要并按 rune 截断；limit<=0 表示不需要摘要。
func BuildExcerpt(plainText string, limit int) string {
	if limit <= 0 {
		return ""
	}
	squeezed := collapseWhitespace(plainText)
	if squeezed == "" {
		return ""
	}
	if utf8.RuneCountInString(squeezed) <= limit {
		return squeezed
	}
	return string([]rune(squeezed)[:limit]) + "…"
}

func templateValues(data TemplateData, now time.Time) map[string]string {
	date := strings.TrimSpace(data.Date)
	if date == "" {
		date = now.Format("2006-01-02 15:04")
	}
	tags := strings.Join(data.Tags, templateTagSeparator)
	canonical := map[string]string{
		templateVarTitle:     data.Title,
		templateVarAuthor:    data.Author,
		templateVarCategory:  data.Category,
		templateVarTags:      tags,
		templateVarExcerpt:   data.Excerpt,
		templateVarURL:       data.URL,
		templateVarTopicID:   data.TopicID,
		templateVarDate:      date,
		templateVarSignature: data.Signature,
	}
	values := make(map[string]string, len(canonical)*2)
	for name, value := range canonical {
		values[name] = value
	}
	values["标题"] = canonical[templateVarTitle]
	values["作者"] = canonical[templateVarAuthor]
	values["分类"] = canonical[templateVarCategory]
	values["标签"] = canonical[templateVarTags]
	values["摘要"] = canonical[templateVarExcerpt]
	values["链接"] = canonical[templateVarURL]
	values["主题ID"] = canonical[templateVarTopicID]
	values["日期"] = canonical[templateVarDate]
	values["签名"] = canonical[templateVarSignature]
	return values
}

// collapseWhitespace 把任意连续空白压成一个空格并去掉首尾空白。变量值里的换行
// 会被压平，避免一条主题把 QQ 消息撑成几十行。
func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func escapeMarkdown(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) + 8)
	for _, r := range value {
		if strings.ContainsRune(markdownEscapeChars, r) {
			builder.WriteByte('\\')
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// truncateRunes 按 rune 截断并追加省略号。limit<=0 表示不限制。
func truncateRunes(value string, limit int) (string, bool) {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value, false
	}
	return string([]rune(value)[:limit]) + "…", true
}
