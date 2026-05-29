package agentsync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func createMergeDraft(source string, conflicts []string) (string, error) {
	dir, err := mergeDraftDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+"-merge.md")

	var b strings.Builder
	b.WriteString("# Unified Agent Instructions\n\n")
	b.WriteString("Review this file, remove duplicates, then run:\n\n")
	b.WriteString("```bash\n")
	b.WriteString("agentsync --adopt ")
	b.WriteString(shellQuote(path))
	b.WriteString("\n```\n\n")

	if pathExists(source) && isRegularFile(source) {
		appendFileSection(&b, "Existing source", source)
	}
	for _, p := range conflicts {
		if pathExists(p) && isRegularFile(p) {
			appendFileSection(&b, "Imported from "+p, p)
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func appendFileSection(b *strings.Builder, title, path string) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")
	data, err := readPayload(path)
	if err != nil {
		b.WriteString("_Could not read file: ")
		b.WriteString(err.Error())
		b.WriteString("_\n\n")
		return
	}
	b.Write(data)
	if len(data) == 0 || data[len(data)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func shellQuote(path string) string {
	if path == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func adoptDraft(source, draft string, check bool) ([]string, error) {
	draftPath, err := expandPath(draft)
	if err != nil {
		return nil, err
	}
	if !pathExists(draftPath) {
		return nil, fmt.Errorf("merge draft does not exist: %s", draftPath)
	}
	if check {
		return nil, nil
	}
	var backups []string
	if pathExists(source) {
		backup, err := backupFile(source)
		if err != nil {
			return nil, err
		}
		if backup != "" {
			backups = append(backups, backup)
		}
	}
	data, err := os.ReadFile(draftPath)
	if err != nil {
		return nil, err
	}
	if err := ensureParent(source); err != nil {
		return nil, err
	}
	if err := os.WriteFile(source, data, 0o644); err != nil {
		return nil, err
	}
	return backups, nil
}

func createSourceFromFiles(source string, files []string) error {
	if err := ensureParent(source); err != nil {
		return err
	}
	if len(files) == 0 {
		return os.WriteFile(source, []byte(defaultSourceContent()), 0o644)
	}

	base, err := readPayload(files[0])
	if err != nil {
		return err
	}
	if len(base) == 0 {
		base = []byte(defaultSourceContent())
	}
	if err := os.WriteFile(source, ensureTrailingNewline(base), 0o644); err != nil {
		return err
	}
	for _, file := range files[1:] {
		if _, err := appendImportedContent(source, file); err != nil {
			return err
		}
	}
	return nil
}

func appendImportedContent(source, origin string) (bool, error) {
	data, err := readPayload(origin)
	if err != nil {
		return false, err
	}
	data = trimOuterWhitespace(data)
	if len(data) == 0 {
		return false, nil
	}
	sourceData, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	if strings.Contains(string(sourceData), string(data)) {
		return false, nil
	}
	hash := sha256.Sum256(data)
	hashText := hex.EncodeToString(hash[:])
	if strings.Contains(string(sourceData), "sha256="+hashText) {
		return false, nil
	}

	var b strings.Builder
	if len(sourceData) > 0 && sourceData[len(sourceData)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("<!-- agentsync:begin import path=\"")
	b.WriteString(escapeMarkerValue(origin))
	b.WriteString("\" sha256=")
	b.WriteString(hashText)
	b.WriteString(" -->\n")
	b.WriteString("## Imported From ")
	b.WriteString(origin)
	b.WriteString("\n\n")
	b.Write(data)
	if data[len(data)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("<!-- agentsync:end import -->\n")

	f, err := os.OpenFile(source, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(b.String()); err != nil {
		return false, err
	}
	return true, nil
}

func trimOuterWhitespace(data []byte) []byte {
	return []byte(strings.TrimSpace(string(data)))
}

func ensureTrailingNewline(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data
	}
	return append(data, '\n')
}

func escapeMarkerValue(value string) string {
	value = strings.ReplaceAll(value, `"`, "&quot;")
	value = strings.ReplaceAll(value, "--", "- -")
	return value
}
