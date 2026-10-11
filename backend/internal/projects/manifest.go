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
