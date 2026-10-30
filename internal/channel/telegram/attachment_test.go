package telegram

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"myaaw/internal/channel"
)

func TestSendTelegramPhotoMultipart(t *testing.T) {
	oldBase := telegramAPIBaseURL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	telegramAPIBaseURL = server.URL
	t.Cleanup(func() { telegramAPIBaseURL = oldBase })

	err := sendTelegramPhoto(42, 7, channel.Attachment{
		FileName: "shot.png",
		MimeType: "image/png",
		Data:     []byte("PNGDATA"),
	})
	if err != nil {
		t.Fatalf("sendTelegramPhoto: %v", err)
	}
}
