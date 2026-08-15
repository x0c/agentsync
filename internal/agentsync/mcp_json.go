package agentsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func stripJSONC(data []byte) []byte {
	var out bytes.Buffer
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inLineComment {
			if c == '\n' {
				inLineComment = false
				out.WriteByte(c)
			}
			continue
		}
		if inBlockComment {
			if c == '*' && i+1 < len(data) && data[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			out.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/':
				inLineComment = true
				i++
				continue
			case '*':
				inBlockComment = true
				i++
				continue
			}
		}
		out.WriteByte(c)
	}
	return out.Bytes()
}

func setTopLevelJSONKey(doc []byte, key string, rawValue []byte) ([]byte, error) {
	doc = bytes.TrimSpace(doc)
	if len(doc) == 0 {
		doc = []byte("{}")
	}
	if doc[0] != '{' {
		return nil, fmt.Errorf("json document is not an object")
	}
	span, err := findTopLevelJSONKey(doc, key)
	if err != nil {
		return nil, err
	}
	if span.start >= 0 {
		var b bytes.Buffer
		b.Write(doc[:span.start])
		b.Write(compactJSONValue(rawValue))
		b.Write(doc[span.end:])
		return b.Bytes(), nil
	}
	return insertTopLevelJSONKey(doc, key, rawValue)
}

type jsonSpan struct {
	start int
	end   int
}

func findTopLevelJSONKey(doc []byte, key string) (jsonSpan, error) {
	i := 0
	if skipJSONSpace(doc, &i); i >= len(doc) || doc[i] != '{' {
		return jsonSpan{start: -1}, fmt.Errorf("json document is not an object")
	}
	i++
	for {
		skipJSONSpace(doc, &i)
		if i >= len(doc) {
			return jsonSpan{start: -1}, fmt.Errorf("json object is truncated")
		}
		if doc[i] == '}' {
			return jsonSpan{start: -1}, nil
		}
		k, next, err := parseJSONString(doc, i)
		if err != nil {
			return jsonSpan{start: -1}, err
		}
		i = next
		skipJSONSpace(doc, &i)
		if i >= len(doc) || doc[i] != ':' {
			return jsonSpan{start: -1}, fmt.Errorf("json object missing colon")
		}
		i++
		skipJSONSpace(doc, &i)
		valueStart := i
		valueEnd, err := skipJSONValue(doc, i)
		if err != nil {
			return jsonSpan{start: -1}, err
		}
		if k == key {
			return jsonSpan{start: valueStart, end: valueEnd}, nil
		}
		i = valueEnd
		skipJSONSpace(doc, &i)
		if i < len(doc) && doc[i] == ',' {
			i++
		}
	}
}

func insertTopLevelJSONKey(doc []byte, key string, rawValue []byte) ([]byte, error) {
	end := lastNonSpace(doc)
	if end < 0 || doc[end] != '}' {
		return nil, fmt.Errorf("json object is truncated")
	}
	inner := bytes.TrimSpace(doc[1:end])
	var b bytes.Buffer
	b.WriteByte('{')
	if len(inner) > 0 {
		b.WriteByte('\n')
		b.Write(indentJSONBlock(inner, 1))
		if inner[len(inner)-1] != ',' {
			b.WriteByte(',')
		}
	}
	b.WriteString("\n  ")
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	b.Write(keyJSON)
	b.WriteString(": ")
	b.Write(indentJSONValue(rawValue, 1))
	b.WriteString("\n}")
	return b.Bytes(), nil
}

func compactJSONValue(raw []byte) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return bytes.TrimSpace(raw)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return bytes.TrimSpace(raw)
	}
	return out
}

func indentJSONValue(raw []byte, level int) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return bytes.TrimSpace(raw)
	}
	out, err := json.MarshalIndent(v, strings.Repeat("  ", level), "  ")
	if err != nil {
		return bytes.TrimSpace(raw)
	}
	return out
}

func indentJSONBlock(inner []byte, level int) []byte {
	pad := strings.Repeat("  ", level)
	lines := bytes.Split(inner, []byte("\n"))
	var b bytes.Buffer
	for i, line := range lines {
		trim := bytes.TrimRightFunc(line, unicode.IsSpace)
		if len(trim) == 0 {
			continue
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		if !bytes.HasPrefix(trim, []byte(" ")) && !bytes.HasPrefix(trim, []byte("\t")) {
			b.WriteString(pad)
		}
		b.Write(trim)
	}
	return b.Bytes()
}

func lastNonSpace(doc []byte) int {
	for i := len(doc) - 1; i >= 0; i-- {
		if !unicode.IsSpace(rune(doc[i])) {
			return i
		}
	}
	return -1
}

func skipJSONSpace(doc []byte, i *int) {
	for *i < len(doc) {
		r, size := utf8.DecodeRune(doc[*i:])
		if !unicode.IsSpace(r) {
			return
		}
		*i += size
	}
}

func parseJSONString(doc []byte, i int) (string, int, error) {
	if i >= len(doc) || doc[i] != '"' {
		return "", i, fmt.Errorf("json string expected")
	}
	end, err := skipJSONValue(doc, i)
	if err != nil {
		return "", i, err
	}
	var s string
	if err := json.Unmarshal(doc[i:end], &s); err != nil {
		return "", i, err
	}
	return s, end, nil
}

func skipJSONValue(doc []byte, i int) (int, error) {
	skipJSONSpace(doc, &i)
	if i >= len(doc) {
		return i, fmt.Errorf("json value is truncated")
	}
	switch doc[i] {
	case '"':
		i++
		escaped := false
		for i < len(doc) {
			c := doc[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if c == '\\' {
				escaped = true
				i++
				continue
			}
			if c == '"' {
				return i + 1, nil
			}
			i++
		}
		return i, fmt.Errorf("json string is truncated")
	case '{', '[':
		stack := []byte{doc[i]}
		i++
		inString := false
		escaped := false
		for i < len(doc) && len(stack) > 0 {
			c := doc[i]
			if inString {
				if escaped {
					escaped = false
					i++
					continue
				}
				if c == '\\' {
					escaped = true
					i++
					continue
				}
				if c == '"' {
					inString = false
				}
				i++
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{', '[':
				stack = append(stack, c)
			case '}':
				if stack[len(stack)-1] != '{' {
					return i, fmt.Errorf("json object is malformed")
				}
				stack = stack[:len(stack)-1]
			case ']':
				if stack[len(stack)-1] != '[' {
					return i, fmt.Errorf("json array is malformed")
				}
				stack = stack[:len(stack)-1]
			}
			i++
		}
		if len(stack) != 0 {
			return i, fmt.Errorf("json value is truncated")
		}
		return i, nil
	default:
		for i < len(doc) {
			c := doc[i]
			if c == ',' || c == '}' || c == ']' || unicode.IsSpace(rune(c)) {
				break
			}
			i++
		}
		return i, nil
	}
}

func unmarshalJSONObject(data []byte) (map[string]any, error) {
	data = bytes.TrimSpace(stripJSONC(data))
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}
