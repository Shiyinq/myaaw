package discord

import "testing"

func TestDiscordScope(t *testing.T) {
	cases := []struct {
		name      string
		chatType  string
		channelID string
		want      string
	}{
		{"dm", "dm", "999", ""},
		{"guild channel", "guild", "123456", "dc:123456"},
		{"thread is a channel", "guild", "777888", "dc:777888"},
		{"old payload without type", "", "123456", ""},
		{"missing channel", "guild", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := discordScope(c.chatType, c.channelID); got != c.want {
				t.Fatalf("discordScope = %q, want %q", got, c.want)
			}
		})
	}
}
