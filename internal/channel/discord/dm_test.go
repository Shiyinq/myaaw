package discord

import (
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

type fakeDMAPI struct {
	createErr  error
	channel    *discordgo.Channel
	channelErr error
	created    []string
	lookedUp   []string
}

func (f *fakeDMAPI) UserChannelCreate(recipientID string, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	f.created = append(f.created, recipientID)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &discordgo.Channel{ID: "dm-" + recipientID, Type: discordgo.ChannelTypeDM}, nil
}

func (f *fakeDMAPI) Channel(channelID string, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	f.lookedUp = append(f.lookedUp, channelID)
	if f.channelErr != nil {
		return nil, f.channelErr
	}
	if f.channel == nil {
		return nil, errors.New("Unknown Channel")
	}
	return f.channel, nil
}

func TestDMChannelOpensDMForUserID(t *testing.T) {
	api := &fakeDMAPI{}
	d := &DiscordAdapter{dmResolver: api}

	got, err := d.resolveChannel(DiscordMeta{UserID: "111222333444555666", IsDM: true})
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if got != "dm-111222333444555666" {
		t.Fatalf("channel = %q", got)
	}

	// The resolved channel is cached, so a second scheduled turn is one call.
	if _, err := d.resolveChannel(DiscordMeta{UserID: "111222333444555666", IsDM: true}); err != nil {
		t.Fatalf("resolveChannel (cached): %v", err)
	}
	if len(api.created) != 1 {
		t.Fatalf("UserChannelCreate calls = %d, want 1", len(api.created))
	}
}

// A model that copies the DM channel id instead of the speaker's user id must
// still get the reminder: Discord confirms the target is a DM channel.
func TestDMChannelAcceptsDMChannelID(t *testing.T) {
	api := &fakeDMAPI{
		createErr: errors.New(`HTTP 400 Bad Request, {"message": "Invalid Recipient(s)", "code": 50033}`),
		channel:   &discordgo.Channel{ID: "111222333444555666", Type: discordgo.ChannelTypeDM},
	}
	d := &DiscordAdapter{dmResolver: api}

	got, err := d.resolveChannel(DiscordMeta{UserID: "111222333444555666", IsDM: true})
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if got != "111222333444555666" {
		t.Fatalf("channel = %q, want the DM channel id", got)
	}
	if len(api.lookedUp) != 1 || api.lookedUp[0] != "111222333444555666" {
		t.Fatalf("lookups = %v", api.lookedUp)
	}
}

// A guild channel id with a stale chat_type "dm" must still be delivered there:
// that is the target the job names, and a user id never resolves as a channel.
func TestDMChannelFallsBackToGuildChannel(t *testing.T) {
	api := &fakeDMAPI{
		createErr: errors.New("Invalid Recipient(s)"),
		channel:   &discordgo.Channel{ID: "333444555666777888", Type: discordgo.ChannelTypeGuildText},
	}
	d := &DiscordAdapter{dmResolver: api}

	got, err := d.resolveChannel(DiscordMeta{UserID: "333444555666777888", IsDM: true})
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if got != "333444555666777888" {
		t.Fatalf("channel = %q, want the guild channel", got)
	}
}

// A target that is neither a DM-able user nor a channel keeps the original error
// so the log still says why the reminder could not be delivered.
func TestDMChannelKeepsErrorWhenTargetIsNotAChannel(t *testing.T) {
	openErr := errors.New("Invalid Recipient(s)")
	api := &fakeDMAPI{createErr: openErr}
	d := &DiscordAdapter{dmResolver: api}

	_, err := d.resolveChannel(DiscordMeta{UserID: "111222333444555666", IsDM: true})
	if err == nil || !strings.Contains(err.Error(), "Invalid Recipient(s)") {
		t.Fatalf("err = %v, want the original DM error", err)
	}
}

func TestResolveChannelUsesExplicitChannelID(t *testing.T) {
	api := &fakeDMAPI{}
	d := &DiscordAdapter{dmResolver: api}

	got, err := d.resolveChannel(DiscordMeta{ChannelID: "222333444555666777"})
	if err != nil {
		t.Fatalf("resolveChannel: %v", err)
	}
	if got != "222333444555666777" || len(api.created) != 0 {
		t.Fatalf("channel = %q, created = %v", got, api.created)
	}
}
