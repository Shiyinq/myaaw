// Package attachment turns an explicit delivery directive in a model reply into
// file references that a chat surface can render.
//
// The model writes `@attach(/absolute/path)` in its final answer. The directive
// is removed from the visible text and the referenced file is resolved to a
// virtual path (such as /home/workspace/projects/site/index.html) that the
// control plane can serve. Directories, unreadable files, and paths outside the
// allowed roots are ignored.
package attachment

import (
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Kind classifies how a file is rendered.
type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
	KindAudio Kind = "audio"
	KindPDF   Kind = "pdf"
	KindFile  Kind = "file"
)

// Roots are the absolute directories an attachment may come from, paired with
// the virtual path each one is addressed by.
type Roots struct {
	Home   string
	User   string
	Skills string
	Jobs   string
}

// Item is one resolved attachment candidate.
type Item struct {
	AbsPath string
	Virtual string
	Name    string
	Kind    Kind
	Mime    string
	Size    int64
}

// directiveRE matches @attach(path), @attach("path with spaces"), and
// @attach('path'). A bare path ends at whitespace or a closing parenthesis.
var directiveRE = regexp.MustCompile(`(?i)@attach\(\s*(?:"([^"]+)"|'([^']+)'|([^)\s]+))\s*\)`)

// bareAtRE matches the short form @/absolute/path (and @~/home/path) that the
// model sometimes writes instead of the full directive. It deliberately requires
// a slash after @ so mentions such as @username are untouched.
var bareAtRE = regexp.MustCompile(`@((?:~/|/|~)[^\s"'<>()]+)`)

// resolveLoose resolves a path, retrying without sentence punctuation so a path
// followed by a period or comma still resolves.
func resolveLoose(raw string, roots Roots) (Item, bool) {
	if item, ok := Resolve(raw, roots); ok {
		return item, true
	}
	trimmed := strings.TrimRight(raw, ".,;:!?")
	if trimmed != raw {
		return Resolve(trimmed, roots)
	}
	return Item{}, false
}

var imageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".svg": true, ".avif": true, ".apng": true, ".ico": true,
}

var videoExt = map[string]bool{
	".mp4": true, ".m4v": true, ".webm": true, ".mov": true, ".mkv": true,
	".avi": true, ".ogv": true,
}

var audioExt = map[string]bool{
	".mp3": true, ".wav": true, ".m4a": true, ".aac": true, ".flac": true,
	".ogg": true, ".opus": true, ".oga": true,
}

var deniedNames = map[string]bool{
	"auth.json":        true,
	"credentials.json": true,
	"id_rsa":           true,
	"id_ed25519":       true,
	"id_ecdsa":         true,
}

// Collect finds attachment references in text without changing it: resolved
// @attach(...) directives plus bare previewable media paths. Directives and
// paths inside a fenced code block are documentation, not delivery. The caller
// decides how the text is presented, so a surface can keep the directive and
// render it its own way.
func Collect(text string, roots Roots) []Item {
	if !directiveRE.MatchString(text) && !bareAtRE.MatchString(text) && !mediaPathRE.MatchString(text) {
		return nil
	}

	items := make([]Item, 0, 2)
	seen := make(map[string]bool)
	add := func(raw string) {
		if raw == "" {
			return
		}
		item, ok := resolveLoose(raw, roots)
		if !ok || seen[item.Virtual] {
			return
		}
		seen[item.Virtual] = true
		items = append(items, item)
	}

	fence := ""
	for _, line := range strings.Split(text, "\n") {
		if isFence, next := fenceState(line, fence); isFence {
			fence = next
			continue
		}
		if fence != "" {
			continue
		}
		for _, match := range directiveRE.FindAllStringSubmatch(line, -1) {
			add(firstNonEmpty(match[1], match[2], match[3]))
		}
		for _, match := range bareAtRE.FindAllStringSubmatch(line, -1) {
			add(match[1])
		}
	}

	for _, item := range Promote(text, roots) {
		if seen[item.Virtual] {
			continue
		}
		seen[item.Virtual] = true
		items = append(items, item)
	}
	return items
}

// fenceState reports whether the line is a code-fence marker and what the fence
// marker is afterwards. An opening fence can carry an info string; a closing
// fence must reuse the same marker. A fence that opens and closes on one line
// is inline code, not a block.
func fenceState(line, open string) (bool, string) {
	trimmed := strings.TrimSpace(line)
	if open != "" {
		if strings.HasPrefix(trimmed, open) {
			return true, ""
		}
		return false, open
	}
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) && strings.Count(trimmed, marker)%2 == 1 {
			return true, marker
		}
	}
	return false, ""
}

// Substitute replaces each resolved @attach(...) directive with the absolute
// path it references, for surfaces that cannot hide the directive themselves.
// Unresolved directives and directives inside a fenced code block are left
// untouched.
func Substitute(text string, roots Roots) string {
	if !directiveRE.MatchString(text) && !bareAtRE.MatchString(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	fence := ""
	for i, line := range lines {
		if isFence, next := fenceState(line, fence); isFence {
			fence = next
			continue
		}
		if fence != "" || (!directiveRE.MatchString(line) && !bareAtRE.MatchString(line)) {
			continue
		}
		lines[i] = directiveRE.ReplaceAllStringFunc(line, func(match string) string {
			sub := directiveRE.FindStringSubmatch(match)
			raw := firstNonEmpty(sub[1], sub[2], sub[3])
			if item, ok := resolveLoose(raw, roots); ok {
				return item.AbsPath
			}
			return match
		})
		lines[i] = bareAtRE.ReplaceAllStringFunc(lines[i], func(match string) string {
			sub := bareAtRE.FindStringSubmatch(match)
			if item, ok := resolveLoose(sub[1], roots); ok {
				return item.AbsPath
			}
			return match
		})
	}
	return strings.Join(lines, "\n")
}

// mediaPathRE matches an absolute or ~ path ending in a previewable media
// extension. The leading group requires a word boundary so a URL path or a path
// glued to a word is not promoted by accident.
var mediaPathRE = regexp.MustCompile(`(?:^|[\s("'` + "`" + `])((?:~/?|/)[^\s"'<>()]+\.(?:png|jpe?g|gif|webp|bmp|svg|avif|apng|ico|mp4|m4v|webm|mov|mkv|avi|ogv|mp3|wav|m4a|aac|flac|ogg|opus|oga|pdf)\b)`)

// standalonePathRE matches a line that holds nothing but one absolute or ~
// path, optionally wrapped in inline-code backticks or prefixed by a list
// bullet. A model that lists a deliverable this way means "here is the file";
// media is promoted anywhere in prose, but archives and documents need this
// shape because promoting paths inside sentences would attach incidental files.
var standalonePathRE = regexp.MustCompile("^\\s*(?:[-*]\\s+)?`?((?:~/|/|~)[^\\s\"'<>()`]+)`?\\s*$")

// unpromotableExt keeps scratch output from becoming an attachment when the
// model prints its path on a line of its own.
var unpromotableExt = map[string]bool{
	".log": true, ".tmp": true, ".temp": true, ".cache": true,
	".bak": true, ".swp": true, ".swo": true,
}

// Promote finds bare file paths in the answer and turns them into attachments
// without requiring the model to remember the directive. Previewable media is
// promoted wherever it appears in prose; any other file is promoted only when a
// line contains nothing but the path (or a bullet and the path), which is how a
// model lists a packaged deliverable. Scratch extensions are ignored. Fenced
// code blocks are never scanned.
func Promote(text string, roots Roots) []Item {
	var items []Item
	seen := make(map[string]bool)
	body := stripFencedBlocks(text)

	for _, match := range mediaPathRE.FindAllStringSubmatch(body, -1) {
		item, ok := resolveLoose(match[1], roots)
		if !ok || item.Kind == KindFile {
			continue
		}
		if seen[item.Virtual] {
			continue
		}
		seen[item.Virtual] = true
		items = append(items, item)
	}

	for _, line := range strings.Split(body, "\n") {
		match := standalonePathRE.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		item, ok := resolveLoose(match[1], roots)
		if !ok || unpromotableExt[strings.ToLower(filepath.Ext(item.Name))] {
			continue
		}
		if seen[item.Virtual] {
			continue
		}
		seen[item.Virtual] = true
		items = append(items, item)
	}
	return items
}

// stripFencedBlocks blanks every fenced code block, keeping the line structure so
// offsets outside the fence stay readable by callers that only scan for matches.
func stripFencedBlocks(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	fence := ""
	for _, line := range lines {
		isFence, next := fenceState(line, fence)
		if isFence {
			fence = next
			out = append(out, "")
			continue
		}
		if fence != "" {
			out = append(out, "")
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// Resolve expands and validates one direct path. It reports false for
// directories, missing files, denied names, and paths outside the roots.
func Resolve(raw string, roots Roots) (Item, bool) {
	path, ok := expandPath(raw)
	if !ok {
		return Item{}, false
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Item{}, false
	}

	virtual, rel, ok := virtualPath(path, roots)
	if !ok {
		return Item{}, false
	}
	// Only the part inside the root is checked: the install root itself may be a
	// hidden directory such as ~/.myaaw.
	if denied(rel) || deniedNames[strings.ToLower(filepath.Base(path))] {
		return Item{}, false
	}

	// A symlink may point outside the roots even when its own path looks fine.
	// Validate the resolved target as well.
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Item{}, false
	}
	_, realRel, ok := virtualPath(real, roots)
	if !ok || denied(realRel) {
		return Item{}, false
	}

	ext := strings.ToLower(filepath.Ext(path))
	return Item{
		AbsPath: path,
		Virtual: virtual,
		Name:    filepath.Base(path),
		Kind:    kindOf(ext),
		Mime:    mimeFor(ext),
		Size:    info.Size(),
	}, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// expandPath turns ~ into the user's home directory and requires an absolute
// path. Relative paths are rejected: the model must name the file explicitly.
func expandPath(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", false
	}
	if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		value = filepath.Join(home, strings.TrimLeft(value[1:], `/\`))
	}
	if !filepath.IsAbs(value) {
		return "", false
	}
	return filepath.Clean(value), true
}

// denied rejects hidden segments and known credential names inside a root.
func denied(rel string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(segment, ".") && segment != "." && segment != ".." {
			return true
		}
	}
	return false
}

// virtualPath maps an absolute path under one of the roots to its virtual form
// and the relative path used for denial checks. The root is also compared in its
// symlink-resolved form because macOS temp directories (and other mount points)
// are reached through a symlink.
func virtualPath(path string, roots Roots) (string, string, bool) {
	candidates := []struct {
		root    string
		virtual string
	}{
		{roots.Home, "/home"},
		{roots.User, "/user"},
		{roots.Skills, "/skills"},
		{roots.Jobs, "/jobs"},
	}
	for _, candidate := range candidates {
		if candidate.root == "" {
			continue
		}
		for _, root := range rootVariants(candidate.root) {
			rel, err := filepath.Rel(root, path)
			if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			return candidate.virtual + "/" + filepath.ToSlash(rel), filepath.ToSlash(rel), true
		}
	}
	return "", "", false
}

func rootVariants(root string) []string {
	cleaned := filepath.Clean(root)
	variants := []string{cleaned}
	if real, err := filepath.EvalSymlinks(cleaned); err == nil && real != cleaned {
		variants = append(variants, real)
	}
	return variants
}

func kindOf(ext string) Kind {
	switch {
	case imageExt[ext]:
		return KindImage
	case videoExt[ext]:
		return KindVideo
	case audioExt[ext]:
		return KindAudio
	case ext == ".pdf":
		return KindPDF
	default:
		return KindFile
	}
}

func mimeFor(ext string) string {
	if value := mime.TypeByExtension(ext); value != "" {
		return value
	}
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt", ".log":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
