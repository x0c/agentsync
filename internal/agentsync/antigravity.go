package agentsync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// antigravityMaxChunkBytes stays cleanly below Antigravity's 24,000-byte
// per-file rule truncation threshold so headers and UTF-8 text fit safely.
const antigravityMaxChunkBytes = 22000

// geminiCLIDetectPath resolves Gemini CLI's gating path without triggering on an
// Antigravity-only ~/.gemini directory that has config/ or antigravity-cli/.
func geminiCLIDetectPath(geminiHome, source string) string {
	hasAntigravity := pathExists(filepath.Join(geminiHome, "config")) ||
		pathExists(filepath.Join(geminiHome, "antigravity-cli"))
	if !hasAntigravity {
		return geminiHome
	}
	if hasGeminiCLIMarker(geminiHome, source) {
		return geminiHome
	}
	// Point Detect at an absent Gemini-CLI-only marker so defaultGlobalConfig
	// still sets a non-empty Detect path while skipping ~/.gemini/GEMINI.md and
	// ~/.gemini/settings.json on Antigravity-only hosts.
	return filepath.Join(geminiHome, "oauth_creds.json")
}

func hasGeminiCLIMarker(geminiHome, source string) bool {
	for _, marker := range []string{
		"oauth_creds.json",
		"google_accounts.json",
		"trustedFolders.json",
		"state.json",
		"projects.json",
		"installation_id",
		"tmp",
		"extensions",
		"commands",
		"policies",
	} {
		if pathExists(filepath.Join(geminiHome, marker)) {
			return true
		}
	}
	geminiMD := filepath.Join(geminiHome, "GEMINI.md")
	if info, err := os.Lstat(geminiMD); err == nil {
		switch {
		case isManagedLink(info) && linkPointsTo(geminiMD, source):
		case info.Mode().IsRegular() && isManagedAgentsyncFile(geminiMD):
		default:
			return true
		}
	}
	settingsPath := filepath.Join(geminiHome, "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		if len(bytes.TrimSpace(data)) == 0 {
			return false
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			return true
		}
		for k := range doc {
			if k != "mcpServers" {
				return true
			}
		}
	}
	return false
}

func antigravityRulesDir(targetPath string) string {
	return filepath.Join(filepath.Dir(targetPath), "config", "rules")
}

func antigravityPartPath(rulesDir string, part int) string {
	return filepath.Join(rulesDir, fmt.Sprintf("agentsync-part-%02d.md", part))
}

func isAntigravityPartFile(name string) bool {
	if !strings.HasPrefix(name, "agentsync-part-") || !strings.HasSuffix(name, ".md") {
		return false
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(name, "agentsync-part-"), ".md")
	if digits == "" {
		return false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}

func listAntigravityPartFiles(rulesDir string) ([]string, error) {
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if isAntigravityPartFile(entry.Name()) {
			out = append(out, filepath.Join(rulesDir, entry.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func renderAntigravityPrimary(source, sumHex, overlay, body string) []byte {
	var b strings.Builder
	b.WriteString(managedMarkerLine(source, sumHex, overlay, " part=1"))
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func renderAntigravityPart(source, sumHex, overlay string, part, total int, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntrigger: always_on\ndescription: agentsync global instructions (part %d of %d)\n---\n", part, total)
	fmt.Fprintf(&b, "%s", managedMarkerLine(source, sumHex, overlay, fmt.Sprintf(" part=%d total=%d", part, total)))
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func splitAntigravityRules(source, sumHex, overlay string, data []byte, maxFileBytes int) []string {
	headerReserve := len(renderAntigravityPart(source, sumHex, overlay, 999, 999, "")) + 2
	primaryReserve := len(renderAntigravityPrimary(source, sumHex, overlay, "")) + 2
	if primaryReserve > headerReserve {
		headerReserve = primaryReserve
	}
	chunkLimit := maxFileBytes - headerReserve
	if chunkLimit < 1024 {
		chunkLimit = 1024
	}
	chunks := splitMarkdownChunks(string(data), chunkLimit)
	if len(chunks) == 1 && len(chunks[0]) > 1 {
		mid := len(chunks[0]) / 2
		for mid > 1 && !utf8.RuneStart(chunks[0][mid]) {
			mid--
		}
		chunks = []string{chunks[0][:mid], chunks[0][mid:]}
	}
	return chunks
}

func splitMarkdownChunks(text string, limit int) []string {
	var chunks []string
	n := len(text)
	for start := 0; start < n; {
		if len(chunks) > 0 {
			for start < n && text[start] == '\n' {
				start++
			}
			if start >= n {
				break
			}
		}
		if n-start <= limit {
			chunks = append(chunks, text[start:])
			break
		}
		split := findMarkdownSplitPoint(text, start, start+limit)
		if split <= start {
			split = clampUTF8Boundary(text, start, start+limit)
		}
		chunks = append(chunks, text[start:split])
		start = split
	}
	return chunks
}

func findMarkdownSplitPoint(text string, start, limitEnd int) int {
	n := len(text)
	pos := start
	var (
		fenceChar         byte
		fenceLen          int
		majorHeadingSplit int
		subHeadingSplit   int
		blankSplit        int
		lineBreakSplit    int
	)
	for pos < n && pos < limitEnd {
		relNL := strings.IndexByte(text[pos:], '\n')
		lineEnd := n
		nextPos := n
		if relNL >= 0 {
			lineEnd = pos + relNL
			nextPos = lineEnd + 1
		}
		line := text[pos:lineEnd]
		inFence := fenceLen > 0
		if !inFence && pos > start {
			trimmed := strings.TrimLeft(line, " \t")
			switch {
			case strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## "):
				majorHeadingSplit = pos
			case strings.HasPrefix(trimmed, "### "):
				subHeadingSplit = pos
			case len(strings.TrimSpace(line)) == 0 && nextPos <= limitEnd:
				blankSplit = nextPos
			}
		}
		fenceChar, fenceLen = updateMarkdownFence(line, fenceChar, fenceLen)
		if nextPos <= limitEnd {
			lineBreakSplit = nextPos
			pos = nextPos
			continue
		}
		break
	}
	chunkLen := limitEnd - start
	switch {
	case majorHeadingSplit-start >= chunkLen/3:
		return majorHeadingSplit
	case subHeadingSplit-start >= chunkLen/2:
		return subHeadingSplit
	case blankSplit-start >= chunkLen/3:
		return blankSplit
	case lineBreakSplit > start:
		return lineBreakSplit
	default:
		return clampUTF8Boundary(text, start, limitEnd)
	}
}

func updateMarkdownFence(line string, fenceChar byte, fenceLen int) (byte, int) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return fenceChar, fenceLen
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return fenceChar, fenceLen
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	if count < 3 {
		return fenceChar, fenceLen
	}
	if fenceLen == 0 {
		if marker == '`' && strings.IndexByte(trimmed[count:], '`') >= 0 {
			return 0, 0
		}
		return marker, count
	}
	if marker == fenceChar && count >= fenceLen && strings.TrimSpace(trimmed[count:]) == "" {
		return 0, 0
	}
	return fenceChar, fenceLen
}

func clampUTF8Boundary(text string, start, limitEnd int) int {
	if limitEnd >= len(text) {
		return len(text)
	}
	split := limitEnd
	for split > start && !utf8.RuneStart(text[split]) {
		split--
	}
	if split == start {
		_, size := utf8.DecodeRuneInString(text[start:])
		return start + size
	}
	return split
}

type antigravityPartSpec struct {
	path string
	data []byte
}

func syncAntigravityTarget(source string, target Target, render instructionRender, opts Options) (TargetResult, []string, error) {
	result := TargetResult{Path: target.Path}
	if target.Detect != "" && !pathExists(target.Detect) {
		result.Status = "skipped"
		result.Detail = "runtime not installed"
		return result, nil, nil
	}
	if err := checkAntigravityPrimaryTarget(target.Path); err != nil {
		result.Status = "blocked"
		result.Detail = err.Error()
		return result, nil, nil
	}
	rulesDir := antigravityRulesDir(target.Path)
	partFiles, err := listAntigravityPartFiles(rulesDir)
	if err != nil {
		return result, nil, err
	}
	for _, p := range partFiles {
		info, lerr := os.Lstat(p)
		if lerr == nil && !info.Mode().IsRegular() && !isManagedLink(info) {
			result.Status = "blocked"
			result.Detail = "antigravity split rule path is not a regular file"
			return result, nil, nil
		}
	}

	sourceData, err := render.body(source)
	if err != nil {
		if !pathExists(source) {
			// The missing source is reported on its own; a preview must not fail here.
			result.Status = "missing"
			result.Detail = "would create alias"
			return result, nil, nil
		}
		return result, nil, err
	}
	if len(sourceData) <= antigravityMaxChunkBytes {
		if render.active() {
			return syncAntigravityOverlayCopy(source, sourceData, target, partFiles, render, opts)
		}
		return syncAntigravityCompact(source, sourceData, target, partFiles, opts)
	}
	return syncAntigravitySplit(source, sourceData, target, rulesDir, partFiles, render, opts)
}

// syncAntigravityOverlayCopy writes the merged source and runtime fragment as
// one managed file. A standalone ~/.gemini/AGENTS.md must not carry YAML
// frontmatter, and Antigravity does not follow a symlink here.
func syncAntigravityOverlayCopy(source string, body []byte, target Target, partFiles []string, render instructionRender, opts Options) (TargetResult, []string, error) {
	result := TargetResult{Path: target.Path}
	expected := managedCopyBody(source, body, render.Runtime)
	detail := fmt.Sprintf("antigravity managed copy with the %s overlay", render.Runtime)
	info, lstatErr := os.Lstat(target.Path)
	targetExists := lstatErr == nil
	if lstatErr != nil && !os.IsNotExist(lstatErr) {
		return result, nil, lstatErr
	}
	if !opts.Force && len(partFiles) == 0 && targetExists && antigravityFileMatches(target.Path, expected) {
		result.Status = "ok"
		result.Detail = detail
		return result, nil, nil
	}
	if opts.Check {
		if !targetExists {
			result.Status = "missing"
			result.Detail = "would create " + detail
		} else {
			result.Status = "replaceable"
			result.Detail = "would update " + detail
		}
		return result, nil, nil
	}
	var backups []string
	if targetExists {
		backup, err := backupFile(target.Path)
		if err != nil {
			return result, backups, err
		}
		if backup != "" {
			backups = append(backups, backup)
		}
		if isManagedLink(info) {
			if err := removePath(target.Path); err != nil {
				return result, backups, err
			}
		}
	}
	if err := writeFileAtomic(target.Path, expected, 0o644); err != nil {
		return result, backups, err
	}
	for _, part := range partFiles {
		backup, err := backupFile(part)
		if err != nil {
			return result, backups, err
		}
		if backup != "" {
			backups = append(backups, backup)
		}
		if err := removePath(part); err != nil {
			return result, backups, err
		}
	}
	if !targetExists {
		result.Status = "created"
	} else {
		result.Status = "replaced"
	}
	result.Detail = detail
	if len(partFiles) > 0 {
		result.Detail = fmt.Sprintf("%s (pruned %d leftover antigravity split rule(s))", detail, len(partFiles))
	}
	return result, backups, nil
}

func checkAntigravityPrimaryTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() && !isManagedLink(info) {
		return fmt.Errorf("not a regular file")
	}
	return nil
}

func syncAntigravityCompact(source string, sourceData []byte, target Target, partFiles []string, opts Options) (TargetResult, []string, error) {
	result := TargetResult{Path: target.Path}
	linkTarget := Target{Name: target.Name, Path: target.Path, Mode: "link", Detect: target.Detect}
	if len(partFiles) == 0 {
		res, backup, err := syncTarget(source, linkTarget, instructionRender{}, opts)
		var backups []string
		if backup != "" {
			backups = append(backups, backup)
		}
		return res, backups, err
	}

	if opts.Check {
		result.Status = "replaceable"
		result.Detail = fmt.Sprintf("would prune %d leftover antigravity split rule(s) and sync link", len(partFiles))
		return result, nil, nil
	}

	if !opts.Force {
		if changed, err := importInstructionTarget(source, target); err != nil {
			return result, nil, err
		} else if changed {
			updated, err := os.ReadFile(source)
			if err != nil {
				return result, nil, err
			}
			if len(updated) > antigravityMaxChunkBytes {
				return syncAntigravitySplit(source, updated, target, antigravityRulesDir(target.Path), partFiles, instructionRender{}, opts)
			}
			sourceData = updated
		}
	}
	_ = sourceData

	var backups []string
	for _, part := range partFiles {
		backup, err := backupFile(part)
		if err != nil {
			return result, backups, err
		}
		if backup != "" {
			backups = append(backups, backup)
		}
		if err := removePath(part); err != nil {
			return result, backups, err
		}
	}
	res, backup, err := syncTarget(source, linkTarget, instructionRender{}, opts)
	if backup != "" {
		backups = append(backups, backup)
	}
	if err != nil {
		return res, backups, err
	}
	if res.Status == "ok" {
		res.Status = "replaced"
		res.Detail = fmt.Sprintf("pruned %d leftover antigravity split rule(s)", len(partFiles))
	} else {
		res.Detail = fmt.Sprintf("%s (pruned %d leftover antigravity split rule(s))", res.Detail, len(partFiles))
	}
	return res, backups, nil
}

func syncAntigravitySplit(source string, sourceData []byte, target Target, rulesDir string, existingParts []string, render instructionRender, opts Options) (TargetResult, []string, error) {
	result := TargetResult{Path: target.Path}
	info, lstatErr := os.Lstat(target.Path)
	targetExists := lstatErr == nil
	if lstatErr != nil && !os.IsNotExist(lstatErr) {
		return result, nil, lstatErr
	}

	// Preserve any unique unmanaged instruction content before splitting.
	if targetExists && !opts.Force {
		origin := ""
		switch {
		case isManagedLink(info) && !linkPointsTo(target.Path, source):
			origin = readableContentPath(target.Path)
		case info.Mode().IsRegular() && !isManagedAgentsyncFile(target.Path):
			origin = target.Path
		}
		if origin != "" && !sameContent(source, origin) && !isManagedAgentsyncFile(origin) {
			if opts.Check {
				result.Status = "mergeable"
				result.Detail = "would merge unique content and replace with antigravity split rules"
				return result, nil, nil
			}
			merged, err := appendImportedContent(source, origin)
			if err != nil {
				return result, nil, err
			}
			if merged {
				updated, err := os.ReadFile(source)
				if err != nil {
					return result, nil, err
				}
				sourceData = updated
			}
		}
	}

	sum := sha256.Sum256(sourceData)
	sumHex := hex.EncodeToString(sum[:])
	chunks := splitAntigravityRules(source, sumHex, render.Runtime, sourceData, antigravityMaxChunkBytes)
	specs := make([]antigravityPartSpec, 0, len(chunks))
	specs = append(specs, antigravityPartSpec{
		path: target.Path,
		data: renderAntigravityPrimary(source, sumHex, render.Runtime, chunks[0]),
	})
	expectedSet := make(map[string]struct{}, len(chunks))
	for i := 1; i < len(chunks); i++ {
		p := antigravityPartPath(rulesDir, i+1)
		expectedSet[p] = struct{}{}
		specs = append(specs, antigravityPartSpec{
			path: p,
			data: renderAntigravityPart(source, sumHex, render.Runtime, i+1, len(chunks), chunks[i]),
		})
	}
	var extraParts []string
	for _, existing := range existingParts {
		if _, keep := expectedSet[existing]; !keep {
			extraParts = append(extraParts, existing)
		}
	}

	detail := fmt.Sprintf("antigravity split rules (%d parts)", len(chunks))
	if !opts.Force && len(extraParts) == 0 && antigravitySpecsMatch(specs) {
		result.Status = "ok"
		result.Detail = detail
		return result, nil, nil
	}

	if opts.Check {
		if !targetExists {
			result.Status = "missing"
			result.Detail = fmt.Sprintf("would create %s", detail)
		} else {
			result.Status = "replaceable"
			result.Detail = fmt.Sprintf("would update %s", detail)
		}
		return result, nil, nil
	}

	var backups []string
	for _, spec := range specs {
		if !opts.Force && antigravityFileMatches(spec.path, spec.data) {
			continue
		}
		if pathExists(spec.path) {
			backup, err := backupFile(spec.path)
			if err != nil {
				return result, backups, err
			}
			if backup != "" {
				backups = append(backups, backup)
			}
			if st, err := os.Lstat(spec.path); err == nil && isManagedLink(st) {
				if err := removePath(spec.path); err != nil {
					return result, backups, err
				}
			}
		}
		if err := writeFileAtomic(spec.path, spec.data, 0o644); err != nil {
			return result, backups, err
		}
	}
	for _, extra := range extraParts {
		backup, err := backupFile(extra)
		if err != nil {
			return result, backups, err
		}
		if backup != "" {
			backups = append(backups, backup)
		}
		if err := removePath(extra); err != nil {
			return result, backups, err
		}
	}

	if !targetExists {
		result.Status = "created"
	} else {
		result.Status = "replaced"
	}
	result.Detail = detail
	return result, backups, nil
}

func antigravitySpecsMatch(specs []antigravityPartSpec) bool {
	for _, spec := range specs {
		if !antigravityFileMatches(spec.path, spec.data) {
			return false
		}
	}
	return true
}

func antigravityFileMatches(path string, expected []byte) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Equal(actual, expected)
}
