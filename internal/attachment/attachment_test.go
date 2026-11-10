package attachment

import (
	"os"
	"path/filepath"
	"testing"
)

func testRoots(t *testing.T) Roots {
	t.Helper()
	// The install root is a hidden directory (for example ~/.myaaw), so every
	// test also guards against rejecting paths because of a dot in the root.
	root := filepath.Join(t.TempDir(), ".myaaw")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return Roots{
		Home:   home,
		User:   filepath.Join(root, "user"),
		Skills: filepath.Join(root, "skills"),
		Jobs:   filepath.Join(root, "jobs"),
	}
}

func writeFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCollectResolvesFile(t *testing.T) {
	roots := testRoots(t)
	image := writeFile(t, filepath.Join(roots.Home, "workspace", "chart.png"))

	items := Collect("Here is the chart.\n@attach("+image+")\nDone.", roots)

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if item.Virtual != "/home/workspace/chart.png" {
		t.Errorf("virtual = %q", item.Virtual)
	}
	if item.Kind != KindImage {
		t.Errorf("kind = %q, want image", item.Kind)
	}
	if item.Name != "chart.png" {
		t.Errorf("name = %q", item.Name)
	}
	if item.Size != 4 {
		t.Errorf("size = %d, want 4", item.Size)
	}
}

func TestCollectMultipleKinds(t *testing.T) {
	roots := testRoots(t)
	video := writeFile(t, filepath.Join(roots.Home, "out", "clip.mp4"))
	audio := writeFile(t, filepath.Join(roots.Home, "out", "voice.mp3"))
	doc := writeFile(t, filepath.Join(roots.Home, "out", "report.pdf"))
	zip := writeFile(t, filepath.Join(roots.Home, "out", "bundle.zip"))

	items := Collect(
		"Files: @attach("+video+") @attach("+audio+") @attach("+doc+") @attach("+zip+")",
		roots,
	)

	kinds := map[string]Kind{}
	for _, item := range items {
		kinds[item.Name] = item.Kind
	}
	if kinds["clip.mp4"] != KindVideo {
		t.Errorf("clip.mp4 = %q", kinds["clip.mp4"])
	}
	if kinds["voice.mp3"] != KindAudio {
		t.Errorf("voice.mp3 = %q", kinds["voice.mp3"])
	}
	if kinds["report.pdf"] != KindPDF {
		t.Errorf("report.pdf = %q", kinds["report.pdf"])
	}
	if kinds["bundle.zip"] != KindFile {
		t.Errorf("bundle.zip = %q", kinds["bundle.zip"])
	}
}

func TestCollectQuotedPathWithSpaces(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "my chart.png"))

	items := Collect(`@attach("`+file+`")`, roots)

	if len(items) != 1 || items[0].Name != "my chart.png" {
		t.Fatalf("items = %+v", items)
	}
}

func TestCollectDeduplicates(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "a.png"))

	items := Collect("@attach("+file+")\n@attach("+file+")", roots)

	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}

func TestCollectKeepsUnresolved(t *testing.T) {
	roots := testRoots(t)
	dir := filepath.Join(roots.Home, "workspace", "site")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if items := Collect("@attach("+dir+")", roots); len(items) != 0 {
		t.Fatalf("items = %+v, want none for a directory", items)
	}
}

func TestCollectOutsideRoots(t *testing.T) {
	roots := testRoots(t)
	outside := writeFile(t, filepath.Join(t.TempDir(), "secret.txt"))

	if items := Collect("@attach("+outside+")", roots); len(items) != 0 {
		t.Fatalf("items = %+v, want none", items)
	}
}

func TestCollectBareAtPath(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "poem.md"))

	items := Collect("Ini filenya\n@"+file, roots)

	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Kind != KindFile {
		t.Errorf("kind = %q", items[0].Kind)
	}
}

func TestCollectBareAtPathWithPunctuation(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "poem.md"))

	items := Collect("Lihat @"+file+".", roots)

	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
}

func TestSubstituteBareAtPath(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "poem.md"))

	got := Substitute("Ini filenya\n@"+file, roots)

	if got != "Ini filenya\n"+file {
		t.Fatalf("Substitute = %q", got)
	}
}

func TestCollectListItem(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "a.md"))

	items := Collect("1. @attach("+file+")", roots)

	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
}

func TestCollectIgnoresFencedDirective(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "a.png"))

	text := "Real:\n@attach(" + file + ")\n\n```\n@attach(" + file + ")\n```\n"
	items := Collect(text, roots)

	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
}

func TestSubstituteListItem(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "a.md"))

	got := Substitute("1. @attach("+file+")", roots)

	if got != "1. "+file {
		t.Fatalf("Substitute = %q", got)
	}
}

func TestPromoteIgnoresFencedPath(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "clip.mp4"))

	items := Promote("```\n"+file+"\n```", roots)

	if len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestPromote(t *testing.T) {
	roots := testRoots(t)
	video := writeFile(t, filepath.Join(roots.Home, "workspace", "clip.mp4"))
	txt := writeFile(t, filepath.Join(roots.Home, "workspace", "notes.txt"))

	items := Promote("Video di "+video+" dan "+txt, roots)

	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Kind != KindVideo {
		t.Errorf("kind = %q", items[0].Kind)
	}
}

func TestPromoteTrailingPunctuation(t *testing.T) {
	roots := testRoots(t)
	video := writeFile(t, filepath.Join(roots.Home, "workspace", "clip.mp4"))

	items := Promote("Lihat "+video+".", roots)

	if len(items) != 1 {
		t.Fatalf("items = %+v", items)
	}
}

func TestSubstitute(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "workspace", "photo.png"))

	got := Substitute("See @attach("+file+") for details.", roots)

	if got != "See "+file+" for details." {
		t.Fatalf("Substitute = %q", got)
	}
}

func TestSubstituteLeavesUnresolved(t *testing.T) {
	roots := testRoots(t)
	outside := writeFile(t, filepath.Join(t.TempDir(), "secret.txt"))

	got := Substitute("@attach("+outside+")", roots)

	if got != "@attach("+outside+")" {
		t.Fatalf("Substitute = %q", got)
	}
}

func TestResolveRejectsHidden(t *testing.T) {
	roots := testRoots(t)
	hidden := writeFile(t, filepath.Join(roots.Home, ".env"))

	if _, ok := Resolve(hidden, roots); ok {
		t.Fatal("hidden file should be rejected")
	}
}

func TestResolveRejectsCredentialName(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.Home, "auth.json"))

	if _, ok := Resolve(file, roots); ok {
		t.Fatal("auth.json should be rejected")
	}
}

func TestResolveRejectsRelative(t *testing.T) {
	roots := testRoots(t)
	if _, ok := Resolve("workspace/a.png", roots); ok {
		t.Fatal("relative path should be rejected")
	}
}

func TestResolveUserRoot(t *testing.T) {
	roots := testRoots(t)
	file := writeFile(t, filepath.Join(roots.User, "upload.txt"))

	item, ok := Resolve(file, roots)
	if !ok {
		t.Fatal("expected user-root file to resolve")
	}
	if item.Virtual != "/user/upload.txt" {
		t.Errorf("virtual = %q", item.Virtual)
	}
}
