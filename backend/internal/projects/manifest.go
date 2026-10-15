package projects

import (
	"strings"
)

// manifestKeyRe 之外的解析全部手写：manifest 是 key=value 文本，
// 但 value 可能是三引号包裹的多行内容（desc 常见），也可能被引号/反引号包裹。
//
// 这里比 fnpackup 的实现多做了两件事：
//  1. 识别 """...""" 多行值（fnpackup 按行切分，多行 desc 会被拆成垃圾键）
//  2. 容忍 " = " 两侧的任意空白（官方 fnpack 生成的模板是对齐过的）
func parseManifest(text string) map[string]string {
	out := make(map[string]string)
	lines := splitLines(text)

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		value := strings.TrimSpace(trimmed[eq+1:])

		if strings.HasPrefix(value, `"""`) {
			rest := value[3:]
			if strings.HasSuffix(rest, `"""`) {
				value = rest[:len(rest)-3]
			} else {
				var b strings.Builder
				b.WriteString(rest)
				for i+1 < len(lines) {
					i++
					next := lines[i]
					if idx := strings.Index(next, `"""`); idx >= 0 {
						b.WriteString("\n")
						b.WriteString(next[:idx])
						break
					}
					b.WriteString("\n")
					b.WriteString(next)
				}
				value = b.String()
			}
		} else {
			value = unquote(value)
		}

		if key != "" {
			out[key] = value
		}
	}
	return out
}

// stripManifestSpaces 去掉 manifest 里「格式用的空格」：`appname   =   demo  ` → `appname=demo`。
//
// **值内部的空格一个都不动**（`display_name = My App` 里的 `My App` 是一个整体，删了
// 就把应用名改了）；多行值（`"""`）的正文整段跳过——那里的缩进和空行都是内容；空行与
// 注释行留着，只去掉它们行尾的空白（注释左侧的缩进是排版，不动）。
//
// 判据和 parseManifest 完全对齐：同样按第一个 `=` 切、同样跳过没闭合的 `"""` 段，
// 所以「解析出来的键值」一个都不会变，变的只有书写格式。没有 `=` 的行、以及 `=` 出现在
// 行首的畸形行都不碰——解析器本来也不认它们。
//
// 唯一在 Pack 里对**暂存副本**调用（见 stripStagedManifestSpaces）：用户项目里的那份
// manifest 一个字节都不动。
func stripManifestSpaces(text string) string {
	lines := splitLines(text)
	inMultiline := false
	for i, line := range lines {
		if inMultiline {
			// 多行值的正文：只看它有没有在这一行闭合，内容原样留着。
			if strings.Contains(line, `"""`) {
				inMultiline = false
			}
			continue
		}
		// 行尾空白先去掉（包括整行只有空白的那些——去掉之后它仍然是一个空行，
		// 「空行」这个结构没被删掉）。
		line = strings.TrimRight(line, " \t")
		lines[i] = line
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		// 值两侧的空白是格式，先 Trim 掉；判多行值和回写用的都是这一份
		// （parseManifest 也是先 TrimSpace 再判 `"""`，两边必须一致）。
		value := strings.TrimSpace(trimmed[eq+1:])
		if strings.HasPrefix(value, `"""`) && !strings.HasSuffix(value[3:], `"""`) {
			inMultiline = true
		}
		lines[i] = strings.TrimSpace(trimmed[:eq]) + "=" + value
	}
	return strings.Join(lines, "\n")
}

// ManifestValue 读取 manifest 中某个键的值。
func ManifestValue(text, key string) string {
	return parseManifest(text)[key]
}

func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if first == last && (first == '"' || first == '\'' || first == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	// 文件末尾的换行会产生一个空元素，保留它才能让「无改动写回」保持字节一致。
	return lines
}
