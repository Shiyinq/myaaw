package discord

import (
	"testing"

	"myaaw/internal/agent/approvals"

	"github.com/bwmarrin/discordgo"
)

func TestApprovalActorResolvesGuildMember(t *testing.T) {
	// Guild interaction: User is nil, Member.User carries the clicker.
	guild := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ChannelID: "chan-1",
		Member:    &discordgo.Member{User: &discordgo.User{ID: "42"}},
	}}
	actor := approvalActor(guild)
	if actor.Channel != "discord" || actor.UserID != "42" || actor.ConversationID != "chan-1" {
		t.Fatalf("guild actor = %+v", actor)
	}

	// DM interaction: User is set instead.
	dm := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		ChannelID: "dm-1",
		User:      &discordgo.User{ID: "7"},
	}}
	actor = approvalActor(dm)
	if actor.UserID != "7" || actor.ConversationID != "dm-1" {
		t.Fatalf("dm actor = %+v", actor)
	}

	// No user information: the actor stays empty and cannot match a bound user.
	empty := approvalActor(&discordgo.InteractionCreate{Interaction: &discordgo.Interaction{ChannelID: "chan-2"}})
	if empty.UserID != "" {
		t.Fatalf("empty actor = %+v", empty)
	}
}

func TestParseApprovalCustomID(t *testing.T) {
	cases := map[string]approvals.Decision{
		"apv:abc:y": approvals.AllowOnce,
		"apv:abc:s": approvals.AllowSession,
		"apv:abc:a": approvals.AllowAlways,
		"apv:abc:n": approvals.Deny,
	}
	for data, want := range cases {
		id, got, ok := parseApprovalCustomID(data)
		if !ok || id != "abc" || got != want {
			t.Errorf("%q => id=%q decision=%q ok=%v, want %q", data, id, got, ok, want)
		}
	}

	for _, data := range []string{"", "apv:", "apv:abc", "apv::y", "other:abc:y", "apv:abc:x"} {
		if _, _, ok := parseApprovalCustomID(data); ok {
			t.Errorf("%q should not parse", data)
		}
	}
}
