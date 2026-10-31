package service

import (
	"errors"
	"testing"

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

func TestBuildUserMessageImagesWithoutText(t *testing.T) {
	svc := &BotServiceImpl{llmProvider: provider.NewOpenAIProvider("http://127.0.0.1:1", "sk-test", "test-model")}

	msg := svc.buildUserMessage(&channel.IncomingMessage{Images: []string{"AAAA"}})
	items := provider.MessageContentItems(msg)
	if len(items) != 1 {
		t.Fatalf("content items = %+v, want only the image", items)
	}
	if items[0].Type != "image_url" || items[0].ImageURL == nil || items[0].ImageURL.URL != "data:image/jpeg;base64,AAAA" {
		t.Fatalf("image item = %+v", items[0])
	}
}
