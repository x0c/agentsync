package agentsync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

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
	if _, err := f.WriteString(b.String()); err != nil {
		return false, errors.Join(err, f.Close())
	}
	return true, f.Close()
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
