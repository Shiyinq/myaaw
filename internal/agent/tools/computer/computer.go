// Package computer exposes one desktop-automation tool with typed actions.
// It is registered only when computer use is enabled (see
// config.ComputerUseEnabled); every action maps to an approval key
// "computer:<action>" through the generic approval gate.
package computer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
)

const captureCacheLimit = 20

const toolSchema = `{
  "type": "function",
  "function": {
    "name": "computer",
    "description": "Control the local desktop. Use one typed action per call. Screenshots are returned as images; set deliver=true to also send the PNG to the user.",
    "parameters": {
      "type": "object",
      "properties": {
        "action": {
          "type": "string",
          "enum": ["screenshot", "move", "click", "double_click", "right_click", "drag", "scroll", "type", "key", "wait", "list_windows", "focus_app"],
          "description": "Action to perform."
        },
        "x": {"type": "integer", "description": "X coordinate (move/click/drag/scroll)."},
        "y": {"type": "integer", "description": "Y coordinate (move/click/drag/scroll)."},
        "x2": {"type": "integer", "description": "End X for drag."},
        "y2": {"type": "integer", "description": "End Y for drag."},
        "text": {"type": "string", "description": "Text to type (action type)."},
        "keys": {"type": "string", "description": "Key combination, e.g. cmd+shift+t (action key)."},
        "duration": {"type": "integer", "description": "Wait duration in milliseconds (action wait), default 500."},
        "window": {"type": "string", "description": "Application name (action focus_app)."},
        "button": {"type": "string", "enum": ["left", "right", "middle"], "description": "Mouse button for click."},
        "amount": {"type": "integer", "description": "Scroll amount (positive integer), default 3."},
        "direction": {"type": "string", "enum": ["up", "down"], "description": "Scroll direction."},
        "deliver": {"type": "boolean", "description": "Attach the screenshot to the user's chat."}
      },
      "required": ["action"]
    }
  }
}`

type toolArgs struct {
	Action    string `json:"action"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	X2        int    `json:"x2"`
	Y2        int    `json:"y2"`
	Text      string `json:"text"`
	Keys      string `json:"keys"`
	Duration  int    `json:"duration"`
	Window    string `json:"window"`
	Button    string `json:"button"`
	Amount    int    `json:"amount"`
	Direction string `json:"direction"`
	Deliver   bool   `json:"deliver"`
}

// ComputerTool is the built-in desktop automation tool.
type ComputerTool struct {
	Backend Backend

	mu        sync.Mutex
	lastFrame Frame
}

func NewComputerTool() *ComputerTool {
	return &ComputerTool{Backend: newBackend()}
}

func init() {
	tools.RegisterBuiltin("computer", NewComputerTool())
}

func (t *ComputerTool) ToolDefinition() []byte { return []byte(toolSchema) }

func (t *ComputerTool) ExecutionMode() tools.ExecutionMode {
	return tools.ExecutionSequential
}

func (t *ComputerTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	return t.CallToolStructured(context.Background(), arguments, ctx).Text
}

func (t *ComputerTool) CallToolStructured(ctx context.Context, arguments string, _ *tools.ToolsContext) tools.StructuredResult {
	var args toolArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return errorResult("COMPUTER_BAD_ARGUMENTS", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if t.Backend == nil {
		t.Backend = newBackend()
	}

	action := strings.ToLower(strings.TrimSpace(args.Action))
	switch action {
	case "screenshot":
		return t.screenshot(ctx, args)
	case "move":
		x, y := t.screenPoint(args.X, args.Y)
		if err := t.Backend.MouseMove(ctx, x, y); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Moved the pointer to (%d, %d).", x, y)}
	case "click":
		button := strings.ToLower(strings.TrimSpace(args.Button))
		if button == "" {
			button = "left"
		}
		x, y := t.screenPoint(args.X, args.Y)
		if err := t.Backend.Click(ctx, x, y, button); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Clicked %s at (%d, %d).", button, x, y)}
	case "double_click":
		x, y := t.screenPoint(args.X, args.Y)
		if err := t.Backend.DoubleClick(ctx, x, y); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Double-clicked at (%d, %d).", x, y)}
	case "right_click":
		x, y := t.screenPoint(args.X, args.Y)
		if err := t.Backend.RightClick(ctx, x, y); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Right-clicked at (%d, %d).", x, y)}
	case "drag":
		x1, y1 := t.screenPoint(args.X, args.Y)
		x2, y2 := t.screenPoint(args.X2, args.Y2)
		if err := t.Backend.Drag(ctx, x1, y1, x2, y2); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Dragged from (%d, %d) to (%d, %d).", x1, y1, x2, y2)}
	case "scroll":
		direction := strings.ToLower(strings.TrimSpace(args.Direction))
		if direction == "" {
			direction = "down"
		}
		amount := args.Amount
		if amount <= 0 {
			amount = 3
		}
		if err := t.Backend.Scroll(ctx, args.X, args.Y, amount, direction); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Scrolled %s %d at (%d, %d).", direction, amount, args.X, args.Y)}
	case "type":
		if err := t.Backend.Type(ctx, args.Text); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Typed %d character(s).", len([]rune(args.Text)))}
	case "key":
		if strings.TrimSpace(args.Keys) == "" {
			return errorResult("COMPUTER_BAD_ARGUMENTS", fmt.Errorf("keys is required for action key"))
		}
		if err := t.Backend.Key(ctx, args.Keys); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: "Pressed " + args.Keys + "."}
	case "wait":
		duration := args.Duration
		if duration <= 0 {
			duration = 500
		}
		if duration > 60000 {
			duration = 60000
		}
		if err := t.Backend.Wait(ctx, time.Duration(duration)*time.Millisecond); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: fmt.Sprintf("Waited %d ms.", duration)}
	case "list_windows":
		windows, err := t.Backend.ListWindows(ctx)
		if err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: formatWindows(windows)}
	case "focus_app":
		name := strings.TrimSpace(args.Window)
		if name == "" {
			return errorResult("COMPUTER_BAD_ARGUMENTS", fmt.Errorf("window is required for action focus_app"))
		}
		if err := t.Backend.FocusApp(ctx, name); err != nil {
			return errorResult("COMPUTER_ACTION_FAILED", err)
		}
		return tools.StructuredResult{Text: "Focused " + name + "."}
	default:
		return errorResult("COMPUTER_UNSUPPORTED_ACTION", fmt.Errorf("unknown action %q", args.Action))
	}
}

func (t *ComputerTool) screenshot(ctx context.Context, args toolArgs) tools.StructuredResult {
	frame, err := t.Backend.Screenshot(ctx)
	if err != nil {
		return errorResult("COMPUTER_SCREENSHOT_FAILED", err)
	}
	t.mu.Lock()
	t.lastFrame = frame
	t.mu.Unlock()

	result := tools.StructuredResult{}
	if frame.Unchanged {
		result.Text = fmt.Sprintf("Screenshot unchanged (frame %s, %dx%d).", frame.ID, frame.Width, frame.Height)
	} else {
		result.Text = fmt.Sprintf("Screenshot captured (frame %s, %dx%d).", frame.ID, frame.Width, frame.Height)
		result.Images = []tools.ImageResult{{
			MimeType: "image/png",
			Data:     base64.StdEncoding.EncodeToString(frame.PNG),
		}}
	}
	if frame.DisplayWidth > 0 && frame.DisplayHeight > 0 {
		result.Text += fmt.Sprintf(" Screen is %dx%d points.", frame.DisplayWidth, frame.DisplayHeight)
	}

	if args.Deliver && len(frame.PNG) > 0 {
		path, err := saveCapture(frame)
		if err != nil {
			return errorResult("COMPUTER_DELIVERY_FAILED", err)
		}
		name := filepath.Base(path)
		result.Attachments = []tools.FileAttachment{{
			FileName: name,
			MimeType: "image/png",
			Data:     frame.PNG,
		}}
		result.Text += " Saved as " + name + "."
	}
	return result
}

func formatWindows(windows []Window) string {
	if len(windows) == 0 {
		return "No visible windows."
	}
	var b strings.Builder
	b.WriteString("Visible windows:\n")
	for i, window := range windows {
		if i >= 200 {
			b.WriteString("- …\n")
			break
		}
		title := strings.TrimSpace(window.Title)
		if title == "" {
			b.WriteString("- " + window.App + "\n")
			continue
		}
		b.WriteString("- " + window.App + " — " + title + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// screenPoint maps a coordinate from the last screenshot (pixels, possibly
// downscaled) to screen points. On Retina displays the captured PNG is larger
// than the logical desktop, so unadjusted clicks would land in the wrong place.
func (t *ComputerTool) screenPoint(x, y int) (int, int) {
	t.mu.Lock()
	frame := t.lastFrame
	t.mu.Unlock()
	if frame.Width <= 0 || frame.Height <= 0 || frame.DisplayWidth <= 0 || frame.DisplayHeight <= 0 {
		return x, y
	}
	return int(math.Round(float64(x) * float64(frame.DisplayWidth) / float64(frame.Width))),
		int(math.Round(float64(y) * float64(frame.DisplayHeight) / float64(frame.Height)))
}

func errorResult(code string, err error) tools.StructuredResult {
	return tools.StructuredResult{Text: fmt.Sprintf("Error [%s]: %v", code, err)}
}

// saveCapture writes the frame into the agent's capture cache and prunes it to
// the newest captureCacheLimit files.
func saveCapture(frame Frame) (string, error) {
	jobsDir, err := agentdir.ActiveJobsDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(jobsDir, "captures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := "screenshot-" + frame.ID + ".png"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, frame.PNG, 0o644); err != nil {
		return "", err
	}
	pruneCaptures(dir)
	return path, nil
}

func pruneCaptures(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".png") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path: filepath.Join(dir, entry.Name()), modTime: info.ModTime()})
	}
	if len(files) <= captureCacheLimit {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	for _, file := range files[:len(files)-captureCacheLimit] {
		_ = os.Remove(file.path)
	}
}
