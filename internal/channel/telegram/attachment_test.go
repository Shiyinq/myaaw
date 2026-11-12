package telegram

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"myaaw/internal/channel"
)

func useTelegramTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	oldBase := telegramAPIBaseURL
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	telegramAPIBaseURL = server.URL
	t.Cleanup(func() { telegramAPIBaseURL = oldBase })
}

func TestSendTelegramAttachmentSendsPhotosAsPhotos(t *testing.T) {
	useTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
			return
		}
		if got := r.FormValue("chat_id"); got != "42" {
			t.Errorf("chat_id = %q", got)
		}
		if got := r.FormValue("reply_to_message_id"); got != "7" {
			t.Errorf("reply_to_message_id = %q", got)
		}
		if got := r.FormValue("message_thread_id"); got != "99" {
			t.Errorf("message_thread_id = %q", got)
		}
		file, header, err := r.FormFile("photo")
		if err != nil {
			t.Errorf("photo part: %v", err)
			return
		}
		defer file.Close()
		if header.Filename != "shot.png" {
			t.Errorf("filename = %q", header.Filename)
		}
		data, _ := io.ReadAll(file)
		if string(data) != "PNGDATA" {
			t.Errorf("data = %q", data)
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	err := sendTelegramAttachment(42, 99, 7, channel.Attachment{
		FileName: "shot.png",
		MimeType: "image/png",
		Data:     []byte("PNGDATA"),
	})
	if err != nil {
		t.Fatalf("sendTelegramAttachment: %v", err)
	}
}

func TestSendTelegramAttachmentSendsDocumentsForFiles(t *testing.T) {
	useTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendDocument") {
			t.Errorf("path = %q, want sendDocument", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
			return
		}
		file, header, err := r.FormFile("document")
		if err != nil {
			t.Errorf("document part: %v", err)
			return
		}
		defer file.Close()
		if header.Filename != "photos.zip" {
			t.Errorf("filename = %q", header.Filename)
		}
		data, _ := io.ReadAll(file)
		if string(data) != "ZIPDATA" {
			t.Errorf("data = %q", data)
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	err := sendTelegramAttachment(42, 99, 7, channel.Attachment{
		FileName: "photos.zip",
		MimeType: "application/zip",
		Data:     []byte("ZIPDATA"),
	})
	if err != nil {
		t.Fatalf("sendTelegramAttachment: %v", err)
	}
}

func TestSendTelegramAttachmentRetriesRejectedPhotosAsDocuments(t *testing.T) {
	var photos, documents int32
	useTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			atomic.AddInt32(&photos, 1)
			http.Error(w, `{"ok":false,"description":"Bad Request"}`, http.StatusBadRequest)
		case strings.HasSuffix(r.URL.Path, "/sendDocument"):
			atomic.AddInt32(&documents, 1)
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("multipart: %v", err)
				return
			}
			if _, _, err := r.FormFile("document"); err != nil {
				t.Errorf("document part: %v", err)
			}
			fmt.Fprint(w, `{"ok":true}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	err := sendTelegramAttachment(42, 0, 0, channel.Attachment{
		FileName: "huge.png",
		MimeType: "image/png",
		Data:     []byte("PNGDATA"),
	})
	if err != nil {
		t.Fatalf("sendTelegramAttachment: %v", err)
	}
	if got := atomic.LoadInt32(&photos); got != 1 {
		t.Errorf("photo attempts = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&documents); got != 1 {
		t.Errorf("document attempts = %d, want 1", got)
	}
}

func TestTelegramFileEndpointRoutesMedia(t *testing.T) {
	cases := []struct {
		mime   string
		name   string
		method string
		field  string
	}{
		{"image/jpeg", "a.jpg", "sendPhoto", "photo"},
		{"image/png", "a.png", "sendPhoto", "photo"},
		{"image/gif", "a.gif", "sendAnimation", "animation"},
		{"video/mp4", "a.mp4", "sendVideo", "video"},
		{"audio/mpeg", "a.mp3", "sendAudio", "audio"},
		{"audio/mp4", "a.m4a", "sendAudio", "audio"},
		{"image/webp", "a.webp", "sendDocument", "document"},
		{"application/zip", "a.zip", "sendDocument", "document"},
		{"", "a.gif", "sendAnimation", "animation"},
		{"", "a.mp4", "sendVideo", "video"},
		{"", "a.mp3", "sendAudio", "audio"},
		{"", "noext", "sendDocument", "document"},
	}
	for _, c := range cases {
		method, field := telegramFileEndpoint(c.mime, c.name)
		if method != c.method || field != c.field {
			t.Errorf("(%q, %q) = %s/%s, want %s/%s", c.mime, c.name, method, field, c.method, c.field)
		}
	}
}

func TestSendTelegramAttachmentsGroupsAlbumsOfTen(t *testing.T) {
	var sizes []int
	useTelegramTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMediaGroup") {
			t.Errorf("path = %q, want sendMediaGroup", r.URL.Path)
			return
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Errorf("multipart: %v", err)
			return
		}
		media := r.FormValue("media")
		count := strings.Count(media, "attach://file")
		sizes = append(sizes, count)
		for i := 0; i < count; i++ {
			if _, _, err := r.FormFile(fmt.Sprintf("file%d", i)); err != nil {
				t.Errorf("file%d part: %v", i, err)
			}
		}
		if !strings.Contains(media, `"type":"photo"`) {
			t.Errorf("media types = %q", media)
		}
		fmt.Fprint(w, `{"ok":true}`)
	})

	atts := make([]channel.Attachment, 12)
	for i := range atts {
		atts[i] = channel.Attachment{
			FileName: fmt.Sprintf("p%d.png", i),
			MimeType: "image/png",
			Data:     []byte("PNG"),
		}
	}
	if err := sendTelegramAttachments(42, 0, 0, atts); err != nil {
		t.Fatalf("sendTelegramAttachments: %v", err)
	}
	if len(sizes) != 2 || sizes[0] != 10 || sizes[1] != 2 {
		t.Fatalf("album sizes = %v, want [10 2]", sizes)
	}
}
