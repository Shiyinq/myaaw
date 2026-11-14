package utils

import "testing"

func TestParseCommandPrefixes(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		want   bool
		cmd    string
		args   string
		prefix byte
	}{
		{name: "slash", text: "/stop", want: true, cmd: "stop", prefix: '/'},
		{name: "bang", text: "!stop", want: true, cmd: "stop", prefix: '!'},
		{name: "uppercase", text: "/STOP", want: true, cmd: "stop", prefix: '/'},
		{name: "bang uppercase", text: "!Stop", want: true, cmd: "stop", prefix: '!'},
		{name: "slash with args", text: "/new topic", want: true, cmd: "new", args: "topic", prefix: '/'},
		{name: "bang with args", text: "!stop now", want: true, cmd: "stop", args: "now", prefix: '!'},
		{name: "mention", text: "/stop@YourBot", want: true, cmd: "stop", prefix: '/'},
		{name: "bang mention", text: "!stop@SomeBot", want: true, cmd: "stop", prefix: '!'},
		{name: "plain text", text: "stop", want: false, prefix: 0},
		{name: "bang inside text", text: "do it now! stop", want: false, prefix: 0},
		{name: "empty", text: "", want: false, prefix: 0},
		{name: "leading space", text: " /stop", want: false, prefix: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, command, args := ParseCommand(tc.text)
			if got != tc.want {
				t.Fatalf("ParseCommand(%q) command = %v, want %v", tc.text, got, tc.want)
			}
			if command != tc.cmd {
				t.Fatalf("ParseCommand(%q) name = %q, want %q", tc.text, command, tc.cmd)
			}
			if args != tc.args {
				t.Fatalf("ParseCommand(%q) args = %q, want %q", tc.text, args, tc.args)
			}
			if prefix := CommandPrefix(tc.text); prefix != tc.prefix {
				t.Fatalf("CommandPrefix(%q) = %q, want %q", tc.text, prefix, tc.prefix)
			}
		})
	}
}
