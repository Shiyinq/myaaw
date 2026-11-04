package handler

import "testing"

func TestHeartbeatLockKey(t *testing.T) {
	cases := []struct {
		name string
		req  HeartbeatRequest
		want string
	}{
		{
			name: "pinned session keys by session",
			req: HeartbeatRequest{
				SessionID: " 0f4e8b3a-1111-4222-8333-944455556666 ",
				Channel:   "telegram",
				To:        "-1001234567890",
				ThreadID:  "77",
			},
			want: "session:0f4e8b3a-1111-4222-8333-944455556666",
		},
		{
			name: "unpinned target keys by room",
			req:  HeartbeatRequest{Channel: "telegram", To: "-1001234567890", ThreadID: "77"},
			want: "target:telegram:-1001234567890:77",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heartbeatLockKey(tc.req); got != tc.want {
				t.Fatalf("heartbeatLockKey(%+v) = %q, want %q", tc.req, got, tc.want)
			}
		})
	}

	// Two topics in one group, and the group itself, must not share a lock.
	group := heartbeatLockKey(HeartbeatRequest{Channel: "telegram", To: "-1001234567890"})
	topic := heartbeatLockKey(HeartbeatRequest{Channel: "telegram", To: "-1001234567890", ThreadID: "77"})
	other := heartbeatLockKey(HeartbeatRequest{Channel: "telegram", To: "-1001234567890", ThreadID: "78"})
	if group == topic || topic == other {
		t.Fatalf("room locks collide: %q %q %q", group, topic, other)
	}
}
