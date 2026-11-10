package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/onboard"
)

func setupSkillAgent(t *testing.T) string {
	t.Helper()
	agentRoot := setupConfigAgent(t, nil)
	skillsDir := filepath.Join(agentRoot, "skills")
	for _, dir := range []string{
		filepath.Join(skillsDir, "weather"),
		filepath.Join(skillsDir, "calendar"),
		filepath.Join(skillsDir, "notes"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	write := func(dir, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(skillsDir, dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s/SKILL.md: %v", dir, err)
		}
	}
	write("weather", "---\nname: Weather\ndescription: Fetch weather\n---\nbody")
	write("calendar", "---\nname: Calendar\ndescription: Manage events\nenabled: false\n---\nbody")
	return agentRoot
}

func TestSkillServiceList(t *testing.T) {
	setupSkillAgent(t)
	svc := NewSkillService()

	list, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("skills = %+v", list)
	}
	byName := map[string]struct{}{}
	for _, skill := range list {
		byName[skill.Name] = struct{}{}
		switch skill.Name {
		case "weather":
			if !skill.Enabled || skill.Source != "default" || skill.DisplayName != "Weather" || !skill.ParseOK {
				t.Fatalf("weather = %+v", skill)
			}
		case "calendar":
			if skill.Enabled || skill.Source != "frontmatter" {
				t.Fatalf("calendar = %+v", skill)
			}
		case "notes":
			if !skill.Enabled || skill.Source != "default" || skill.ParseOK || skill.DisplayName != "notes" {
				t.Fatalf("notes = %+v", skill)
			}
		}
	}
	if len(byName) != 3 {
		t.Fatalf("skills = %+v", list)
	}
}

func TestSkillServiceListEmptyOrMissing(t *testing.T) {
	agentRoot := setupConfigAgent(t, nil)
	svc := NewSkillService()

	list, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List without skills dir: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list = %+v", list)
	}

	skillsDir := filepath.Join(agentRoot, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "skills.json"), []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("write skills.json: %v", err)
	}
	list, err = svc.List("myfriend")
	if err != nil {
		t.Fatalf("List with invalid skills.json: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list = %+v", list)
	}
}

func TestSkillServiceToggle(t *testing.T) {
	agentRoot := setupSkillAgent(t)
	svc := NewSkillService()
	skillsDir := filepath.Join(agentRoot, "skills")

	if err := os.WriteFile(filepath.Join(skillsDir, "skills.json"), []byte(`{"calendar":false}`), 0o644); err != nil {
		t.Fatalf("write skills.json: %v", err)
	}

	if err := svc.SetEnabled("myfriend", "weather", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(skillsDir, "skills.json"))
	if err != nil {
		t.Fatalf("read skills.json: %v", err)
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("unmarshal skills.json: %v", err)
	}
	if filter["weather"] != false || filter["calendar"] != false {
		t.Fatalf("filter = %+v", filter)
	}

	list, err := svc.List("myfriend")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, skill := range list {
		if skill.Name == "weather" && (skill.Enabled || skill.Source != "config") {
			t.Fatalf("weather = %+v", skill)
		}
		if skill.Name == "calendar" && (skill.Enabled || skill.Source != "config") {
			t.Fatalf("calendar = %+v", skill)
		}
	}

	if err := svc.SetEnabled("myfriend", "missing", true); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("err = %v, want ErrSkillNotFound", err)
	}
	if err := svc.SetEnabled("myfriend", "../evil", true); !errors.Is(err, ErrInvalidSkillName) {
		t.Fatalf("err = %v, want ErrInvalidSkillName", err)
	}
}

func TestSkillServiceDelete(t *testing.T) {
	agentRoot := setupSkillAgent(t)
	svc := NewSkillService()
	skillsDir := filepath.Join(agentRoot, "skills")

	if err := os.WriteFile(filepath.Join(skillsDir, "skills.json"), []byte(`{"weather":true,"calendar":false}`), 0o644); err != nil {
		t.Fatalf("write skills.json: %v", err)
	}

	if err := svc.Delete("myfriend", "weather"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "weather")); !os.IsNotExist(err) {
		t.Fatalf("weather dir should be removed, err = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(skillsDir, "skills.json"))
	if err != nil {
		t.Fatalf("read skills.json: %v", err)
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("unmarshal skills.json: %v", err)
	}
	if _, exists := filter["weather"]; exists {
		t.Fatalf("weather key should be removed: %+v", filter)
	}
	if filter["calendar"] != false {
		t.Fatalf("calendar key should be preserved: %+v", filter)
	}

	if err := svc.Delete("myfriend", "missing"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("err = %v, want ErrSkillNotFound", err)
	}
	if err := svc.Delete("myfriend", "../evil"); !errors.Is(err, ErrInvalidSkillName) {
		t.Fatalf("err = %v, want ErrInvalidSkillName", err)
	}

	// A folder removed manually leaves a stale skills.json key. DELETE must
	// clean that key even though the folder is already gone.
	if err := os.WriteFile(filepath.Join(skillsDir, "skills.json"), []byte(`{"ghost":true}`), 0o644); err != nil {
		t.Fatalf("write skills.json: %v", err)
	}
	if err := svc.Delete("myfriend", "ghost"); err != nil {
		t.Fatalf("Delete stale key: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(skillsDir, "skills.json"))
	if err != nil {
		t.Fatalf("read skills.json: %v", err)
	}
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("unmarshal skills.json: %v", err)
	}
	if _, exists := filter["ghost"]; exists {
		t.Fatalf("ghost key should be removed: %+v", filter)
	}
	if err := svc.Delete("myfriend", "ghost"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("second delete err = %v, want ErrSkillNotFound", err)
	}
}

func TestSkillServiceGet(t *testing.T) {
	agentRoot := setupSkillAgent(t)
	skillsDir := filepath.Join(agentRoot, "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "weather", "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "weather", "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "weather", "scripts", "weather.py"), []byte("print('hi')"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02}
	if err := os.WriteFile(filepath.Join(skillsDir, "weather", "assets", "icon.png"), pngBytes, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	svc := NewSkillService()
	detail, err := svc.Get("myfriend", "weather")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.DisplayName != "Weather" || !detail.ParseOK || !detail.Enabled || detail.Source != "default" {
		t.Fatalf("detail = %+v", detail)
	}
	if !strings.Contains(detail.Content, "body") {
		t.Fatalf("content = %q", detail.Content)
	}
	paths := []string{}
	for _, file := range detail.Files {
		paths = append(paths, file.Path)
	}
	want := []string{"SKILL.md", "assets/icon.png", "scripts/weather.py"}
	if len(paths) != len(want) {
		t.Fatalf("files = %+v", paths)
	}
	for i, path := range paths {
		if path != want[i] {
			t.Fatalf("files = %+v", paths)
		}
	}

	// File reading: text comes back as-is, binary as base64.
	script, err := svc.ReadFile("myfriend", "weather", "scripts/weather.py")
	if err != nil {
		t.Fatalf("ReadFile script: %v", err)
	}
	if script.Binary || script.Content != "print('hi')" || script.Path != "scripts/weather.py" {
		t.Fatalf("script = %+v", script)
	}

	icon, err := svc.ReadFile("myfriend", "weather", "/assets/icon.png")
	if err != nil {
		t.Fatalf("ReadFile icon: %v", err)
	}
	if !icon.Binary {
		t.Fatalf("icon should be binary: %+v", icon)
	}
	decoded, err := base64.StdEncoding.DecodeString(icon.Content)
	if err != nil {
		t.Fatalf("decode icon: %v", err)
	}
	if string(decoded) != string(pngBytes) {
		t.Fatalf("decoded icon = %q", decoded)
	}

	if _, err := svc.ReadFile("myfriend", "weather", "missing.txt"); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("err = %v, want ErrPathNotFound", err)
	}
	if _, err := svc.ReadFile("myfriend", "weather", "../secret"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath", err)
	}
	if _, err := svc.ReadFile("myfriend", "nope", "SKILL.md"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("err = %v, want ErrSkillNotFound", err)
	}

	// Missing folder with a stale skills.json key: the key is cleaned up and
	// ErrSkillNotFound is returned.
	if err := os.RemoveAll(filepath.Join(skillsDir, "weather")); err != nil {
		t.Fatalf("remove skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "skills.json"), []byte(`{"weather":true}`), 0o644); err != nil {
		t.Fatalf("write skills.json: %v", err)
	}
	if _, err := svc.Get("myfriend", "weather"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("err = %v, want ErrSkillNotFound", err)
	}
	data, err := os.ReadFile(filepath.Join(skillsDir, "skills.json"))
	if err != nil {
		t.Fatalf("read skills.json: %v", err)
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("unmarshal skills.json: %v", err)
	}
	if _, exists := filter["weather"]; exists {
		t.Fatalf("stale key should be removed: %+v", filter)
	}

	if _, err := svc.Get("myfriend", "../evil"); !errors.Is(err, ErrInvalidSkillName) {
		t.Fatalf("err = %v, want ErrInvalidSkillName", err)
	}
}

func TestSkillServiceRemoveConfig(t *testing.T) {
	agentRoot := setupSkillAgent(t)
	skillsDir := filepath.Join(agentRoot, "skills")
	if err := os.WriteFile(filepath.Join(skillsDir, "skills.json"), []byte(`{"weather":false,"calendar":true}`), 0o644); err != nil {
		t.Fatalf("write skills.json: %v", err)
	}

	svc := NewSkillService()
	if err := svc.RemoveConfig("myfriend", "weather"); err != nil {
		t.Fatalf("RemoveConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(skillsDir, "skills.json"))
	if err != nil {
		t.Fatalf("read skills.json: %v", err)
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("unmarshal skills.json: %v", err)
	}
	if _, exists := filter["weather"]; exists {
		t.Fatalf("weather key should be removed: %+v", filter)
	}
	if filter["calendar"] != true {
		t.Fatalf("calendar key should be preserved: %+v", filter)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "weather")); err != nil {
		t.Fatalf("skill folder must remain: %v", err)
	}

	// Idempotent when the key is already gone.
	if err := svc.RemoveConfig("myfriend", "weather"); err != nil {
		t.Fatalf("RemoveConfig again: %v", err)
	}

	if err := svc.RemoveConfig("myfriend", "missing"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("err = %v, want ErrSkillNotFound", err)
	}
}

func TestSkillServiceUnknownAgent(t *testing.T) {
	setupSkillAgent(t)
	svc := NewSkillService()
	if _, err := svc.List("missing"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
}

func TestSkillServiceInstall(t *testing.T) {
	agentRoot := setupConfigAgent(t, nil)
	svc := NewSkillService()
	names := onboard.BuiltinSkillNames()
	if len(names) == 0 {
		t.Skip("no built-in skills embedded")
	}
	name := names[0]

	detail, err := svc.Install("myfriend", name)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if detail.Name != name || !detail.Enabled {
		t.Fatalf("detail = %+v, want %q enabled", detail, name)
	}
	if info, err := os.Stat(filepath.Join(agentRoot, "skills", name)); err != nil || !info.IsDir() {
		t.Fatalf("skill folder missing: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(agentRoot, "skills", "skills.json"))
	if err != nil {
		t.Fatalf("read skills.json: %v", err)
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		t.Fatalf("parse skills.json: %v", err)
	}
	if !filter[name] {
		t.Fatalf("skills.json = %+v, want %q enabled", filter, name)
	}

	// Installing again only ensures the flag.
	if _, err := svc.Install("myfriend", name); err != nil {
		t.Fatalf("Install again: %v", err)
	}

	if _, err := svc.Install("myfriend", "not-a-builtin"); !errors.Is(err, ErrInvalidSkillName) {
		t.Fatalf("err = %v, want ErrInvalidSkillName", err)
	}
}
