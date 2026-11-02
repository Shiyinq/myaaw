package service

import (
	"encoding/base64"
	"errors"
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
}

type conversationUpdate struct {
	id       string
	messages []provider.Message
}

func newFakeConversationRepo() *fakeConversationRepo {
	return &fakeConversationRepo{conversations: make(map[string]*model.Conversation)}
}

func (f *fakeConversationRepo) GetConversationByUserId(int) ([]*model.Conversation, error) {
	return nil, nil
}

func (f *fakeConversationRepo) GetConversationById(id string) (*model.Conversation, error) {
	return f.conversations[id], nil
}

func (f *fakeConversationRepo) CreateConversation(userId int, title string) (*model.Conversation, error) {
	conversation := &model.Conversation{
		Id:       "generated-conversation",
		UserId:   userId,
		Title:    title,
		Messages: []provider.Message{},
	}
	f.conversations[conversation.Id] = conversation
	return conversation, nil
}

func (f *fakeConversationRepo) CreateConversationWithId(userId int, id, title string) (*model.Conversation, error) {
	conversation := &model.Conversation{
		Id:       id,
		UserId:   userId,
		Title:    title,
		Messages: []provider.Message{},
	}
	f.conversations[id] = conversation
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
	wantDir := filepath.Join(home, ".myaaw", "myfriend", "user", "uploads", "conv-1", last.ID)
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
	if len(messages) != 2 || provider.MessageText(messages[1]) != "first, edited" {
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
	if last.Role != "user" || provider.MessageText(last) != "third" || last.CreatedAt == nil {
		t.Fatalf("last = %+v", last)
	}
}
