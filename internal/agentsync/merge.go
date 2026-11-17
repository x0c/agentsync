package agentsync

import (
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
	data, err := os.ReadFile(draftPath)
	if err != nil {
		return nil, err
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
	perm := os.FileMode(0o644)
	if info, err := os.Stat(source); err == nil {
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := writeFileAtomic(source, data, perm); err != nil {
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

	base, err := importablePayload(files[0])
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

// appendImportedContent preserves unique user text under a minimal generated
// delimiter. Paths and hashes add no Agent behavior, so deduplication compares
// the complete imported body instead of writing provenance into fixed context.
func appendImportedContent(source, origin string) (bool, error) {
	data, err := importablePayload(origin)
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

	var b strings.Builder
	if len(sourceData) > 0 && sourceData[len(sourceData)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("<!-- agentsync:begin import -->\n")
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
