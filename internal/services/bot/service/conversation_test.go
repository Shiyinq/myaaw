package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/channel"
	"myaaw/internal/provider"
	"myaaw/internal/services/bot/model"
)

type fakeConversationRepo struct {
	conversations map[string]*model.Conversation
	updates       []conversationUpdate
	scopeNames    map[string][2]string
}

type conversationUpdate struct {
	id       string
	messages []provider.Message
}

func newFakeConversationRepo() *fakeConversationRepo {
	return &fakeConversationRepo{
		conversations: make(map[string]*model.Conversation),
		scopeNames:    make(map[string][2]string),
	}
}

func (f *fakeConversationRepo) GetConversationByUserId(int) ([]*model.Conversation, error) {
	return nil, nil
}

func (f *fakeConversationRepo) GetConversationById(id string) (*model.Conversation, error) {
	return f.conversations[id], nil
}

func (f *fakeConversationRepo) CreateConversation(userId int, channel, title string) (*model.Conversation, error) {
	conversation := &model.Conversation{
		Id:       "generated-conversation",
		UserId:   userId,
		Channel:  channel,
		Title:    title,
		Messages: []provider.Message{},
	}
	f.conversations[conversation.Id] = conversation
	return conversation, nil
}

func (f *fakeConversationRepo) CreateConversationWithId(userId int, id, channel, title string) (*model.Conversation, error) {
	conversation := &model.Conversation{
		Id:       id,
		UserId:   userId,
		Channel:  channel,
		Title:    title,
		Messages: []provider.Message{},
	}
	f.conversations[id] = conversation
	return conversation, nil
}

func (f *fakeConversationRepo) CreateConversationInScope(userId int, scope, channel, title string) (*model.Conversation, error) {
	if scope == "" {
		return f.CreateConversation(userId, channel, title)
	}
	// Rotate the scope: only one conversation per scope stays active.
	for _, conv := range f.conversations {
		if conv.Scope == scope {
			conv.Active = false
		}
	}
	conversation := &model.Conversation{
		Id:       fmt.Sprintf("generated-%d", len(f.conversations)+1),
		UserId:   userId,
		Scope:    scope,
		Channel:  channel,
		Title:    title,
		Messages: []provider.Message{},
		Active:   true,
	}
	f.conversations[conversation.Id] = conversation
	return conversation, nil
}

func (f *fakeConversationRepo) UpdateConversationById(id string, messages []provider.Message, _ string) error {
	f.updates = append(f.updates, conversationUpdate{id: id, messages: messages})
	return nil
}

func (f *fakeConversationRepo) UpdateConversationTitle(string, string) error { return nil }

func (f *fakeConversationRepo) DeleteConversationById(string) error { return nil }

func (f *fakeConversationRepo) GetActiveConversationByUserId(int) (*model.Conversation, error) {
	return nil, nil
}

func (f *fakeConversationRepo) GetActiveConversationByScope(userId int, scope string) (*model.Conversation, error) {
	if scope == "" {
		return f.GetActiveConversationByUserId(userId)
	}
	for _, conv := range f.conversations {
		if conv.Scope == scope && conv.Active {
			return conv, nil
		}
	}
	return nil, nil
}

func (f *fakeConversationRepo) UpsertScopeNames(string, string, string, bool) error { return nil }

func (f *fakeConversationRepo) GetScopeNames(scope string) (string, string, bool, error) {
	names, ok := f.scopeNames[scope]
	if !ok {
		return "", "", false, nil
	}
	return names[0], names[1], false, nil
}

func TestBuildConversationMessagesCreatesA2AContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	service := &BotServiceImpl{conversationRepo: repo}
	user := &model.User{UserId: -1}
	msg := &channel.IncomingMessage{
		UserID:         -1,
		Text:           "hello",
		Channel:        "a2a",
		ConversationID: "ctx-new",
	}

	messages, err := service.buildConversationMessages(user, msg)
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}
	if repo.conversations["ctx-new"] == nil {
		t.Fatal("A2A conversation was not created")
	}
	if repo.conversations["ctx-new"].UserId != -1 {
		t.Fatalf("conversation user = %d", repo.conversations["ctx-new"].UserId)
	}
	if msg.ConversationID != "ctx-new" {
		t.Fatalf("conversation id = %q", msg.ConversationID)
	}
	if len(messages) < 2 {
		t.Fatalf("messages = %d", len(messages))
	}
}

func TestBuildConversationMessagesRejectsUnknownNonA2AContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	service := &BotServiceImpl{conversationRepo: repo}
	msg := &channel.IncomingMessage{
		UserID:         0,
		Text:           "hello",
		Channel:        "api",
		ConversationID: "missing",
	}

	if _, err := service.buildConversationMessages(&model.User{UserId: 0}, msg); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("err = %v, want ErrConversationNotFound", err)
	}
}

func TestBuildConversationMessagesReusesA2AContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	repo.conversations["ctx-old"] = &model.Conversation{
		Id:     "ctx-old",
		UserId: -1,
		Messages: []provider.Message{
			{Role: "user", Content: "previous question"},
			{Role: "assistant", Content: "previous answer"},
		},
	}
	service := &BotServiceImpl{conversationRepo: repo}
	msg := &channel.IncomingMessage{
		UserID:         -1,
		Text:           "follow up",
		Channel:        "a2a",
		ConversationID: "ctx-old",
	}

	messages, err := service.buildConversationMessages(&model.User{UserId: -1}, msg)
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want 4 (system + 2 history + new)", len(messages))
	}
}

func TestBuildUserMessageTagsGroupSenders(t *testing.T) {
	svc := &BotServiceImpl{llmProvider: provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model")}

	group := svc.buildUserMessage(&channel.IncomingMessage{
		UserID:         123456789,
		Scope:          "tg:-1001111:77",
		SenderID:       "123456789",
		SenderName:     "Alice",
		SenderUsername: "alice",
		Text:           "hello",
	}, nil)
	if group.SenderID != "123456789" || group.SenderName != "Alice" || group.SenderUsername != "alice" {
		t.Fatalf("group sender = %+v", group)
	}

	// Direct chats keep the transcript clean: no sender metadata.
	dm := svc.buildUserMessage(&channel.IncomingMessage{UserID: 123456789, Text: "hello"}, nil)
	if dm.SenderID != "" || dm.SenderName != "" || dm.SenderUsername != "" {
		t.Fatalf("dm sender = %+v", dm)
	}
}

func TestBuildUserMessageFallsBackToUserID(t *testing.T) {
	svc := &BotServiceImpl{llmProvider: provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model")}

	msg := svc.buildUserMessage(&channel.IncomingMessage{
		UserID: 42,
		Scope:  "tg:-1001111",
		Text:   "hi",
	}, nil)
	if msg.SenderID != "42" {
		t.Fatalf("sender id = %q, want the user id fallback", msg.SenderID)
	}
}

// A trigger turn stores the whole prompt inside a marker block, so the model
// knows why it woke up while people see only a badge and the prompt.
func TestBuildUserMessageTagsTriggerTurns(t *testing.T) {
	svc := &BotServiceImpl{llmProvider: provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model")}

	cron := svc.buildUserMessage(&channel.IncomingMessage{
		UserID:      42,
		Channel:     "telegram",
		Scope:       "tg:-1001111:77",
		Text:        "Time to drink water",
		TriggerType: "cron",
	}, nil)
	content, _ := cron.Content.(string)
	// The block sits after <userinfo> and wraps the prompt.
	if !strings.Contains(content, `<trigger kind="cron">`) {
		t.Fatalf("cron turn has no trigger block: %q", content)
	}
	if strings.Index(content, `<trigger kind="cron">`) > strings.Index(content, "Time to drink water") {
		t.Fatalf("trigger block does not start before the prompt: %q", content)
	}
	if !strings.Contains(content, "\nTime to drink water\n</trigger>") {
		t.Fatalf("prompt is not inside the block: %q", content)
	}
	if strings.Contains(content, "SYSTEM TRIGGER") {
		t.Fatalf("cron turn kept the old header: %q", content)
	}
	if got := provider.StripContextBlocks(content); got != "Time to drink water" {
		t.Fatalf("stripped = %q, want the prompt only", got)
	}

	// A heartbeat's prompt is all rules, so it folds into the rules and no part
	// of the body is a message.
	heartbeat := svc.buildUserMessage(&channel.IncomingMessage{
		UserID:      42,
		Channel:     "api",
		Text:        "check the inbox",
		TriggerType: "heartbeat",
	}, nil)
	heartbeatContent, _ := heartbeat.Content.(string)
	if !strings.Contains(heartbeatContent, `<trigger kind="heartbeat">`) {
		t.Fatalf("heartbeat block = %q", heartbeatContent)
	}
	if !strings.Contains(heartbeatContent, `<trigger kind="heartbeat">`+"\ncheck the inbox") {
		t.Fatalf("tick prompt is not at the top of the block: %q", heartbeatContent)
	}
	if got := provider.StripContextBlocks(heartbeatContent); got != "" {
		t.Fatalf("heartbeat stripped = %q, want empty", got)
	}
	if strings.Contains(heartbeatContent, "SYSTEM TRIGGER") {
		t.Fatalf("heartbeat kept the old header: %q", heartbeatContent)
	}
}

func TestBuildUserMessageWrapsUploadPathsInAttachedBlock(t *testing.T) {
	svc := &BotServiceImpl{llmProvider: provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model")}

	uploads := []provider.Upload{
		{Name: "a.txt", Path: "/agent/user/uploads/conv/msg/a.txt"},
		{Name: "b.csv", Path: "/agent/user/uploads/conv/msg/b.csv"},
	}
	msg := svc.buildUserMessage(&channel.IncomingMessage{Text: "summarize these"}, uploads)

	want := "<attached>\n[read them with the filesystem or bash tools]\n" +
		"/agent/user/uploads/conv/msg/a.txt\n/agent/user/uploads/conv/msg/b.csv\n</attached>\n\nsummarize these"
	if msg.Content != want {
		t.Fatalf("content = %q, want %q", msg.Content, want)
	}
}

func TestBuildConversationMessagesStoresUploads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	previous := agentdir.Active()
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(previous) })

	repo := newFakeConversationRepo()
	repo.conversations["conv-1"] = &model.Conversation{Id: "conv-1", UserId: -1}
	service := &BotServiceImpl{
		conversationRepo: repo,
		llmProvider:      provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model"),
	}
	msg := &channel.IncomingMessage{
		UserID:         -1,
		Text:           "summarize this",
		Channel:        "api",
		ConversationID: "conv-1",
		Images:         []string{base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n-fake-png"))},
		Files: []channel.FileUpload{{
			Name:       "notes.txt",
			MimeType:   "text/plain",
			DataBase64: base64.StdEncoding.EncodeToString([]byte("hello")),
		}},
	}

	messages, err := service.buildConversationMessages(&model.User{UserId: -1}, msg)
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}
	last := messages[len(messages)-1]
	if len(last.Uploads) != 1 || last.ID == "" {
		t.Fatalf("uploads = %+v, id = %q", last.Uploads, last.ID)
	}

	upload := last.Uploads[0]
	wantDir := filepath.Join(home, ".myaaw", "agents", "myfriend", "user", "uploads", "conv-1", last.ID)
	if filepath.Dir(upload.Path) != wantDir {
		t.Fatalf("upload dir = %q, want %q", filepath.Dir(upload.Path), wantDir)
	}
	if _, err := os.Stat(upload.Path); err != nil {
		t.Fatalf("stored upload missing: %v", err)
	}

	// The image is sent inline to the model and also stored next to the document,
	// so the person finds it in the Files page.
	imagePath := filepath.Join(wantDir, "image-1.png")
	if _, err := os.Stat(imagePath); err != nil {
		t.Fatalf("stored image missing: %v", err)
	}

	text := provider.MessageText(last)
	if !strings.Contains(text, "<attached>\n") || !strings.Contains(text, upload.Path) || !strings.HasSuffix(text, "\n\nsummarize this") {
		t.Fatalf("content = %q", text)
	}
	items := provider.MessageContentItems(last)
	if len(items) != 2 || items[1].Type != "image_url" {
		t.Fatalf("content items = %+v, want the text and the image", items)
	}
}

func TestBuildUserMessageImagesWithoutText(t *testing.T) {
	svc := &BotServiceImpl{llmProvider: provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model")}

	msg := svc.buildUserMessage(&channel.IncomingMessage{Images: []string{"AAAA"}}, nil)
	items := provider.MessageContentItems(msg)
	if len(items) != 1 {
		t.Fatalf("content items = %+v, want only the image", items)
	}
	if items[0].Type != "image_url" || items[0].ImageURL == nil || items[0].ImageURL.URL != "data:image/jpeg;base64,AAAA" {
		t.Fatalf("image item = %+v", items[0])
	}
}

func TestRewindToUserMessageDropsTrailingTurns(t *testing.T) {
	messages := []provider.Message{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "answer one"},
		{Role: "user", Content: "two"},
		{Role: "assistant", Content: "answer two"},
		{Role: "user", Content: "three"},
		{Role: "assistant", Content: "answer three"},
	}

	// Regenerate the last answer: keep up to "three".
	kept, err := rewindToUserMessage(messages, 0, "")
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if len(kept) != 5 || provider.MessageText(kept[4]) != "three" {
		t.Fatalf("kept = %+v", kept)
	}

	// Edit the first question: everything after it is dropped.
	edited, err := rewindToUserMessage(messages, 2, "one, edited")
	if err != nil {
		t.Fatalf("rewind edit: %v", err)
	}
	if len(edited) != 1 || provider.MessageText(edited[0]) != "one, edited" {
		t.Fatalf("edited = %+v", edited)
	}
	if edited[0].CreatedAt == nil {
		t.Fatal("an edited message should be stamped")
	}
}

func TestRewindToUserMessageRejectsMissingTurn(t *testing.T) {
	_, err := rewindToUserMessage([]provider.Message{{Role: "assistant", Content: "hi"}}, 0, "")
	if err == nil {
		t.Fatal("rewind without a user message should fail")
	}
}

func TestBuildConversationMessagesRewindsBeforeRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	repo.conversations["conv-1"] = &model.Conversation{
		Id:     "conv-1",
		UserId: 1,
		Messages: []provider.Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "one"},
			{Role: "user", Content: "second"},
			{Role: "assistant", Content: "two"},
		},
	}
	service := &BotServiceImpl{conversationRepo: repo}

	// Regenerate: keep the transcript up to the last user message and append
	// nothing, so the model answers that same question again.
	zero := 0
	regenerate := &channel.IncomingMessage{
		UserID:           1,
		Channel:          "api",
		ConversationID:   "conv-1",
		RewindUserOffset: &zero,
	}
	messages, err := service.buildConversationMessages(&model.User{UserId: 1}, regenerate)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if len(messages) != 4 || provider.MessageText(messages[3]) != "second" {
		t.Fatalf("messages = %+v", messages)
	}
	// The run carries no text of its own, but the trace should still show the
	// question that was replayed.
	if regenerate.Text != "second" {
		t.Fatalf("trace text = %q, want the replayed question", regenerate.Text)
	}

	// Edit an earlier turn: the replacement text replaces it and everything
	// after it is dropped.
	one := 1
	messages, err = service.buildConversationMessages(&model.User{UserId: 1}, &channel.IncomingMessage{
		UserID:           1,
		Text:             "first, edited",
		Channel:          "api",
		ConversationID:   "conv-1",
		RewindUserOffset: &one,
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if len(messages) != 2 || !strings.HasSuffix(provider.MessageText(messages[1]), "first, edited") {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[1].CreatedAt == nil {
		t.Fatal("an edited user message should be stamped")
	}

	// A normal send still appends the user message and stamps it in UTC.
	messages, err = service.buildConversationMessages(&model.User{UserId: 1}, &channel.IncomingMessage{
		UserID:         1,
		Text:           "third",
		Channel:        "api",
		ConversationID: "conv-1",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	last := messages[len(messages)-1]
	if last.Role != "user" || !strings.HasSuffix(provider.MessageText(last), "third") || last.CreatedAt == nil {
		t.Fatalf("last = %+v", last)
	}
}

func TestRewindToUserMessagePreservesContextBlocks(t *testing.T) {
	block := "<userinfo>\nPlatform: API · 2026-11-04 14:03\n</userinfo>"
	attached := "<attached>\n[read them with the filesystem or bash tools]\n/a.txt\n</attached>"
	original := block + "\n\n" + attached + "\n\nfirst question"

	messages := []provider.Message{
		{Role: "user", Content: original},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "answer two"},
	}

	// Edit the first turn: only the text changes, the context and the uploaded
	// file list stay exactly as they were.
	kept, err := rewindToUserMessage(messages, 1, "first, edited")
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept = %d messages, want 1", len(kept))
	}
	text := provider.MessageText(kept[0])
	if !strings.HasPrefix(text, block) || !strings.Contains(text, attached) || !strings.HasSuffix(text, "first, edited") {
		t.Fatalf("edited content = %q", text)
	}
}

func TestRewindToUserMessagePreservesImageParts(t *testing.T) {
	image := provider.ContentItem{Type: "image_url", ImageURL: &provider.ImageInfo{URL: "data:image/jpeg;base64,AAAA"}}
	messages := []provider.Message{{
		Role: "user",
		Content: []provider.ContentItem{
			{Type: "text", Text: "<userinfo>\nPlatform: API\n</userinfo>\n\nlook at this"},
			image,
		},
	}}

	kept, err := rewindToUserMessage(messages, 0, "look at this again")
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	items := provider.MessageContentItems(kept[0])
	if len(items) != 2 || items[0].Type != "text" || !strings.HasSuffix(items[0].Text, "look at this again") {
		t.Fatalf("content items = %+v", items)
	}
	if !strings.HasPrefix(items[0].Text, "<userinfo>") {
		t.Fatalf("context block lost: %q", items[0].Text)
	}
	if items[1].Type != "image_url" || items[1].ImageURL == nil {
		t.Fatalf("image item lost: %+v", items[1])
	}
}

func TestBuildConversationMessagesFillsScopeNamesFromStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	// A normal forum message carries the topic id but not its name; the name was
	// learned from a previous update and lives in chat_scopes.
	repo.scopeNames["tg:-100:77"] = [2]string{"Tim Support", "Laporan Harian"}
	service := &BotServiceImpl{conversationRepo: repo}

	messages, err := service.buildConversationMessages(&model.User{UserId: 111}, &channel.IncomingMessage{
		UserID:         111,
		Text:           "hi",
		Channel:        "telegram",
		ChatType:       "supergroup",
		ChatID:         "-100",
		TopicID:        "77",
		IsForum:        true,
		IsThread:       true,
		ChatName:       "Tim Support",
		Scope:          "tg:-100:77",
		SenderID:       "222",
		SenderName:     "Andi",
		SenderUsername: "andi",
	})
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}

	content := provider.MessageText(messages[len(messages)-1])
	if !strings.Contains(content, "Chat: Tim Support › Laporan Harian (forum topic, shared)") {
		t.Fatalf("content = %q", content)
	}
	if !strings.Contains(content, "TopicID: 77") {
		t.Fatalf("content = %q", content)
	}
	// A brand-new session gets its id before the user message is built, so even
	// the first turn can name the session it belongs to.
	if !strings.Contains(content, "Session: generated-1") {
		t.Fatalf("new chat should carry its session id: %q", content)
	}
}

func TestBuildConversationMessagesSharesScopeAcrossUsers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	repo.conversations["topic-session"] = &model.Conversation{
		Id:     "topic-session",
		UserId: 111,
		Scope:  "tg:-100:77",
		Active: true,
		Messages: []provider.Message{
			{Role: "user", Content: "hello from 111"},
			{Role: "assistant", Content: "hi"},
		},
	}
	service := &BotServiceImpl{conversationRepo: repo}
	msg := &channel.IncomingMessage{
		UserID:  222,
		Text:    "hello from 222",
		Channel: "telegram",
		Scope:   "tg:-100:77",
	}

	messages, err := service.buildConversationMessages(&model.User{UserId: 222}, msg)
	if err != nil {
		t.Fatalf("buildConversationMessages: %v", err)
	}
	if msg.ConversationID != "topic-session" {
		t.Fatalf("conversation id = %q, want the shared topic session", msg.ConversationID)
	}
	if len(messages) != 4 {
		t.Fatalf("messages = %d, want 4 (system + shared history + new)", len(messages))
	}
}

func TestBuildConversationMessagesRejectsOtherScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	repo := newFakeConversationRepo()
	repo.conversations["topic-session"] = &model.Conversation{
		Id:     "topic-session",
		UserId: 111,
		Scope:  "tg:-100:77",
		Active: true,
	}
	service := &BotServiceImpl{conversationRepo: repo}
	msg := &channel.IncomingMessage{
		UserID:         222,
		Text:           "sneaky",
		Channel:        "telegram",
		Scope:          "tg:-100:78",
		ConversationID: "topic-session",
	}

	if _, err := service.buildConversationMessages(&model.User{UserId: 222}, msg); !errors.Is(err, ErrConversationForbidden) {
		t.Fatalf("err = %v, want ErrConversationForbidden", err)
	}
}

func TestNewCommandRotatesOnlyItsScope(t *testing.T) {
	repo := newFakeConversationRepo()
	repo.conversations["photo"] = &model.Conversation{
		Id:     "photo",
		UserId: 1,
		Scope:  "tg:-100:77",
		Active: true,
	}
	repo.conversations["notif"] = &model.Conversation{
		Id:     "notif",
		UserId: 1,
		Scope:  "tg:-100:78",
		Active: true,
	}
	repo.conversations["dm"] = &model.Conversation{
		Id:     "dm",
		UserId: 1,
		Active: true,
	}
	service := &BotServiceImpl{conversationRepo: repo}

	isCommand, response, err := service.command(&model.User{UserId: 1}, &channel.IncomingMessage{
		UserID: 1,
		Text:   "/new",
		Scope:  "tg:-100:77",
	})
	if err != nil || !isCommand {
		t.Fatalf("command = %v, %v", isCommand, err)
	}
	if response == "" {
		t.Fatal("expected a confirmation message")
	}
	if repo.conversations["photo"].Active {
		t.Fatal("the topic session should have been rotated")
	}
	if !repo.conversations["notif"].Active || !repo.conversations["dm"].Active {
		t.Fatal("/new in one topic must not touch other scopes")
	}
}

func TestNewCommandWithoutScopeKeepsPerUserBehavior(t *testing.T) {
	repo := newFakeConversationRepo()
	service := &BotServiceImpl{conversationRepo: repo}

	isCommand, response, err := service.command(&model.User{UserId: 5}, &channel.IncomingMessage{
		UserID: 5,
		Text:   "/new",
	})
	if err != nil || !isCommand {
		t.Fatalf("command = %v, %v", isCommand, err)
	}
	if response == "" {
		t.Fatal("expected a confirmation message")
	}
	if len(repo.conversations) != 1 {
		t.Fatalf("conversations = %d, want 1", len(repo.conversations))
	}
	for _, conv := range repo.conversations {
		if conv.Scope != "" {
			t.Fatalf("DM session should be unscoped: %+v", conv)
		}
	}
}
