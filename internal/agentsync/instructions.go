package agentsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Per-runtime instruction overlays live in an optional directory beside the
// canonical source: one Markdown fragment per runtime key, for example
// ~/.config/agentsync/instructions/pi.md. agentsync appends that fragment to the
// source for that runtime alone and writes a managed copy, because a symlink
// cannot carry a per-runtime difference.
const (
	overlayFileSuffix    = ".md"
	overlayBeginPrefix   = "<!-- agentsync:begin overlay target=\""
	overlayBeginSuffix   = "\" -->\n"
	overlayEndTag        = "<!-- agentsync:end overlay -->"
	overlayEndMarker     = overlayEndTag + "\n"
	overlayMarkerAttr    = "overlay="
	windsurfRulesLimit   = 6000
	windsurfRulesPathWay = "memories/global_rules.md"
)

// instructionRender is the instruction body a target receives. An inactive
// render means the canonical source applies verbatim and stays a symlink.
type instructionRender struct {
	// Runtime is the target name whose fragment applies; empty means none.
	Runtime string
	// Fragment is the overlay file content appended after the source.
	Fragment string
}

func (r instructionRender) active() bool { return r.Runtime != "" }

// body returns the bytes that represent this target's instructions. It always
// reads the source fresh so imports performed earlier in the run are included.
func (r instructionRender) body(source string) ([]byte, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	if !r.active() {
		return data, nil
	}
	return renderInstructionOverlay(data, r.Runtime, r.Fragment), nil
}

// renderInstructionOverlay appends a runtime fragment under explicit markers so
// readers and diffs can tell which text is runtime specific.
func renderInstructionOverlay(base []byte, runtime, fragment string) []byte {
	var b strings.Builder
	if trimmed := strings.TrimRight(string(base), "\n"); trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString(overlayBeginPrefix)
	b.WriteString(runtime)
	b.WriteString(overlayBeginSuffix)
	b.WriteString(strings.TrimSpace(fragment))
	b.WriteByte('\n')
	b.WriteString(overlayEndMarker)
	return []byte(b.String())
}

// loadInstructionOverlays reads <runtime>.md fragments. Unknown runtime names
// produce warnings, case-insensitive duplicates and unreadable files are errors,
// and a fragment that is blank counts as absent so clearing it restores the
// plain symlink state.
func loadInstructionOverlays(dir string, targets []Target) (map[string]string, []TargetResult, error) {
	if dir == "" {
		return nil, nil, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("instruction overlay path is not a directory: %s", dir)
	}
	known, names := instructionRuntimeKeys(targets)
	files, err := instructionOverlayFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	if first, second, duplicate := duplicateOverlayKey(files); duplicate {
		return nil, nil, fmt.Errorf("instruction overlays %s and %s both define runtime %q; keep one file", first.path, second.path, first.key)
	}
	overlays := make(map[string]string, len(files))
	var warnings []TargetResult
	for _, file := range files {
		if _, ok := known[file.key]; !ok {
			warnings = append(warnings, TargetResult{
				Path:   file.path,
				Status: "warning",
				Detail: fmt.Sprintf("unknown runtime %q; use a runtime key such as %s", file.key, strings.Join(names, ", ")),
			})
			continue
		}
		data, err := os.ReadFile(file.path)
		if err != nil {
			return nil, nil, err
		}
		fragment := strings.TrimSpace(string(data))
		if fragment == "" {
			continue
		}
		overlays[file.key] = fragment
	}
	return overlays, warnings, nil
}

// instructionOverlayFile is one candidate fragment with its runtime key.
type instructionOverlayFile struct {
	key  string
	path string
}

// instructionOverlayFiles lists candidate fragments in name order so duplicate
// and unknown-name reports are stable.
func instructionOverlayFiles(dir string) ([]instructionOverlayFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]instructionOverlayFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		key, ok := overlayRuntimeKey(entry.Name())
		if !ok {
			continue
		}
		files = append(files, instructionOverlayFile{key: key, path: filepath.Join(dir, entry.Name())})
	}
	return files, nil
}

// duplicateOverlayKey reports the first two fragments that name one runtime,
// which a case-insensitive filesystem cannot express but must still be rejected
// where two differently cased files can coexist.
func duplicateOverlayKey(files []instructionOverlayFile) (instructionOverlayFile, instructionOverlayFile, bool) {
	seen := make(map[string]instructionOverlayFile, len(files))
	for _, file := range files {
		if previous, ok := seen[file.key]; ok {
			return previous, file, true
		}
		seen[file.key] = file
	}
	return instructionOverlayFile{}, instructionOverlayFile{}, false
}

// instructionRuntimeKeys maps lowercased runtime keys to their canonical names.
func instructionRuntimeKeys(targets []Target) (map[string]string, []string) {
	known := make(map[string]string, len(targets))
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.Name == "" {
			continue
		}
		key := strings.ToLower(target.Name)
		if _, duplicate := known[key]; duplicate {
			continue
		}
		known[key] = target.Name
		names = append(names, target.Name)
	}
	sort.Strings(names)
	return known, names
}

// overlayRuntimeKey derives the runtime key from an overlay file name and skips
// hidden files, non-Markdown files and synchronization conflict copies.
func overlayRuntimeKey(name string) (string, bool) {
	if strings.HasPrefix(name, ".") || strings.Contains(name, ".sync-conflict") {
		return "", false
	}
	ext := filepath.Ext(name)
	if !strings.EqualFold(ext, overlayFileSuffix) {
		return "", false
	}
	base := strings.TrimSpace(strings.TrimSuffix(name, ext))
	if base == "" {
		return "", false
	}
	return strings.ToLower(base), true
}

// newInstructionRender selects the fragment that applies to a target.
func newInstructionRender(target Target, overlays map[string]string) instructionRender {
	if target.Name == "" || len(overlays) == 0 {
		return instructionRender{}
	}
	key := strings.ToLower(target.Name)
	fragment, ok := overlays[key]
	if !ok {
		return instructionRender{}
	}
	return instructionRender{Runtime: target.Name, Fragment: fragment}
}

// windsurfLimitWarning reports content that Windsurf truncates. Its
// memories/global_rules.md is capped at 6000 characters, so the generated
// marker header counts too whenever the target is a managed copy.
func windsurfLimitWarning(source string, target Target, render instructionRender, status string) *TargetResult {
	if target.Name != "windsurf" || status == "skipped" {
		return nil
	}
	body, err := render.body(source)
	if err != nil {
		return nil
	}
	if render.active() {
		body = managedCopyBody(body, render.Runtime)
	}
	length := utf8.RuneCount(body)
	if length <= windsurfRulesLimit {
		return nil
	}
	detail := fmt.Sprintf("%d characters exceed Windsurf's %d-character limit for %s; Windsurf truncates the excess", length, windsurfRulesLimit, windsurfRulesPathWay)
	return &TargetResult{Path: target.Path, Status: "warning", Detail: detail}
}
