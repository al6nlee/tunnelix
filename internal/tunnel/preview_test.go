package tunnel

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
	"tunnelix/internal/config"
)

func TestCommandPreviewEscaping(t *testing.T) {
	args := []string{"-i", "/tmp/Alan's keys/$(echo unsafe)`id`", "", "-L", "[::1]:5432:[::1]:5432", "host"}
	if runtime.GOOS != "windows" {
		command := CommandPreview("ssh", args, "linux")
		out, e := exec.Command("sh", "-c", "set -- "+command+`; printf '%s\000' "$@"`).Output()
		if e != nil {
			t.Fatal(e)
		}
		want := strings.Join(append([]string{"ssh"}, args...), "\x00") + "\x00"
		if string(out) != want {
			t.Fatalf("shell escaping changed arguments: %q", out)
		}
	}
	command := CommandPreview(`C:\Program Files\OpenSSH\ssh.exe`, []string{"-i", `C:\Alan's keys\id`}, "windows")
	if command != `& 'C:\Program Files\OpenSSH\ssh.exe' '-i' 'C:\Alan''s keys\id'` {
		t.Fatal(command)
	}
}

func TestRespondRetiresPromptImmediately(t *testing.T) {
	m := New("")
	m.pending["challenge"] = pending{Challenge: Challenge{ID: "challenge"}, response: make(chan answer, 1)}
	if e := m.Respond("challenge", "secret", true); e != nil {
		t.Fatal(e)
	}
	_, prompts := m.Snapshot()
	if len(prompts) != 0 {
		t.Fatal("submitted prompt remains visible")
	}
	if e := m.Respond("challenge", "secret", true); e == nil {
		t.Fatal("duplicate response accepted")
	}
}

func TestSavedPasswordDoesNotAnswerUnrelatedPrompts(t *testing.T) {
	for _, prompt := range []string{"jump@bastion's password: ", "Enter passphrase for key '/tmp/key': ", "Are you sure you want to continue connecting (yes/no)?"} {
		t.Run(prompt, func(t *testing.T) {
			m := New("")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session := &session{initialSecret: "target-secret", expectedPrompt: "user@target's password:", secrets: map[string]string{}}
			result := make(chan string, 1)
			go func() { value, _ := m.prompt(ctx, config.Tunnel{ID: "test"}, session, prompt, ""); result <- value }()
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				_, pending := m.Snapshot()
				if len(pending) > 0 {
					if e := m.Respond(pending[0].ID, "", false); e != nil {
						t.Fatal(e)
					}
					if <-result != "" {
						t.Fatal("saved secret sent to unrelated prompt")
					}
					return
				}
				select {
				case <-result:
					t.Fatal("prompt was automatically answered")
				default:
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatal("expected manual prompt")
		})
	}
}
