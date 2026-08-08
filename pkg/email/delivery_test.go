package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// smtpScript drives a fake SMTP server. Each entry replaces the default reply
// for a command prefix, which is how a test makes one specific step fail.
type smtpScript map[string]string

// startFakeSMTP runs a minimal SMTP server that speaks just enough of the
// protocol for net/smtp, replying from the script where one is given.
//
// A real conversation is the only way to test the classification that matters
// here: the whole point is telling a 5xx on RCPT TO apart from a 5xx on AUTH,
// and a stubbed error value cannot express that difference.
func startFakeSMTP(t *testing.T, script smtpScript) (host, port string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveFakeSMTP(conn, script)
		}
	}()

	// Close and wait in one cleanup: the accept loop only returns once the
	// listener is closed, so waiting before closing would deadlock.
	t.Cleanup(func() {
		_ = listener.Close()
		wg.Wait()
	})

	host, port, err = net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	return host, port
}

func serveFakeSMTP(conn net.Conn, script smtpScript) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }

	reply := func(command, fallback string) string {
		for prefix, scripted := range script {
			if strings.HasPrefix(strings.ToUpper(command), prefix) {
				return scripted
			}
		}
		return fallback
	}

	write("220 fake.test ESMTP")
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				write(reply("ENDDATA", "250 2.0.0 Ok: queued"))
			}
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			// No STARTTLS and no AUTH advertised by default, so the client
			// talks plaintext to this loopback server.
			write("250-fake.test")
			write(reply("EHLO", "250 SIZE 35882577"))
		case strings.HasPrefix(upper, "AUTH"):
			write(reply("AUTH", "235 2.7.0 Accepted"))
		case strings.HasPrefix(upper, "MAIL FROM"):
			write(reply("MAIL FROM", "250 2.1.0 Ok"))
		case strings.HasPrefix(upper, "RCPT TO"):
			write(reply("RCPT TO", "250 2.1.5 Ok"))
		case strings.HasPrefix(upper, "DATA"):
			if scripted := reply("DATA", ""); scripted != "" && !strings.HasPrefix(scripted, "354") {
				write(scripted)
				continue
			}
			inData = true
			write("354 End data with <CR><LF>.<CR><LF>")
		case strings.HasPrefix(upper, "QUIT"):
			write("221 2.0.0 Bye")
			return
		default:
			write("250 2.0.0 Ok")
		}
	}
}

// recordingRecorder captures what the sender reports, and can pretend an
// address is suppressed.
type recordingRecorder struct {
	attempts   []Attempt
	suppressed map[string]bool
}

func (r *recordingRecorder) RecordAttempt(_ context.Context, a Attempt) {
	r.attempts = append(r.attempts, a)
}

func (r *recordingRecorder) IsSuppressed(_ context.Context, recipient string) bool {
	return r.suppressed[recipient]
}

func (r *recordingRecorder) last() Attempt {
	if len(r.attempts) == 0 {
		return Attempt{}
	}
	return r.attempts[len(r.attempts)-1]
}

func senderFor(t *testing.T, script smtpScript, recorder DeliveryRecorder) *Sender {
	t.Helper()
	host, port := startFakeSMTP(t, script)
	s := NewSender(&SMTPConfig{Host: host, Port: port, From: "noreply@ninerlog.test"})
	s.SetDeliveryRecorder(recorder)
	return s
}

func send(t *testing.T, s *Sender, to string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.SendMessage(ctx, Message{
		To: to, Subject: "Subject", HTMLBody: "<p>Body</p>", Type: TypeVerifyEmail,
	})
}

func TestSendMessage_ClassifiesOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		script     smtpScript
		wantStatus DeliveryStatus
		wantCode   int
		// permanent is what the reaper keys on to decide whether to start the
		// deletion clock, so it is asserted separately from the status.
		permanent bool
	}{
		{
			name:       "accepted message is delivered",
			wantStatus: StatusDelivered,
			wantCode:   250,
		},
		{
			name:       "5xx on RCPT TO is a hard bounce",
			script:     smtpScript{"RCPT TO": "550 5.1.1 No such user here"},
			wantStatus: StatusHardBounce,
			wantCode:   550,
			permanent:  true,
		},
		{
			name:       "4xx on RCPT TO is a soft bounce",
			script:     smtpScript{"RCPT TO": "452 4.2.2 Mailbox full"},
			wantStatus: StatusSoftBounce,
			wantCode:   452,
		},
		{
			// The regression this whole design exists to prevent: a permanent
			// failure that is about our credentials, not the recipient. If this
			// were classified as a bounce, one stale SMTP password would
			// suppress every address the system mails.
			name:       "5xx on AUTH is a server error, never a bounce",
			script:     smtpScript{"EHLO": "250 AUTH PLAIN LOGIN", "AUTH": "535 5.7.8 Authentication credentials invalid"},
			wantStatus: StatusServerError,
			wantCode:   535,
		},
		{
			name:       "5xx on MAIL FROM is a server error",
			script:     smtpScript{"MAIL FROM": "553 5.7.1 Sender address rejected"},
			wantStatus: StatusServerError,
			wantCode:   553,
		},
		{
			// The recipient was accepted; only the message was refused. The
			// mailbox is real, so the address must survive.
			name:       "5xx after the message body is a rejection, not a bounce",
			script:     smtpScript{"ENDDATA": "552 5.3.4 Message too big"},
			wantStatus: StatusRejected,
			wantCode:   552,
		},
		{
			name:       "4xx after the message body is a soft bounce",
			script:     smtpScript{"ENDDATA": "451 4.3.0 Try again later"},
			wantStatus: StatusSoftBounce,
			wantCode:   451,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &recordingRecorder{}
			sender := senderFor(t, tc.script, recorder)
			// A password makes the client attempt AUTH when the server offers it.
			sender.config.Password = "hunter2hunter2"
			sender.config.Username = "user"

			err := send(t, sender, "pilot@example.test")

			if tc.wantStatus == StatusDelivered {
				if err != nil {
					t.Fatalf("expected delivery, got %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected %s, got success", tc.wantStatus)
				}
				var sendErr *SendError
				if !errors.As(err, &sendErr) {
					t.Fatalf("expected *SendError, got %T: %v", err, err)
				}
				if sendErr.Status != tc.wantStatus {
					t.Errorf("status = %q, want %q (%v)", sendErr.Status, tc.wantStatus, sendErr.Err)
				}
				if sendErr.Code != tc.wantCode {
					t.Errorf("smtp code = %d, want %d", sendErr.Code, tc.wantCode)
				}
				if sendErr.Permanent() != tc.permanent {
					t.Errorf("Permanent() = %v, want %v", sendErr.Permanent(), tc.permanent)
				}
			}

			if got := recorder.last(); got.Status != tc.wantStatus {
				t.Errorf("recorded status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got := recorder.last(); got.Recipient != "pilot@example.test" {
				t.Errorf("recorded recipient = %q", got.Recipient)
			}
			if got := recorder.last(); got.Type != TypeVerifyEmail {
				t.Errorf("recorded type = %q, want %q", got.Type, TypeVerifyEmail)
			}
		})
	}
}

func TestSendMessage_SuppressedAddressIsNotDialled(t *testing.T) {
	recorder := &recordingRecorder{suppressed: map[string]bool{"dead@example.test": true}}
	// Point at an address nothing is listening on: if the sender dialled, the
	// failure would be a server error rather than a suppression.
	sender := NewSender(&SMTPConfig{Host: "127.0.0.1", Port: "1", From: "noreply@ninerlog.test"})
	sender.SetDeliveryRecorder(recorder)

	err := send(t, sender, "dead@example.test")

	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("expected *SendError, got %v", err)
	}
	if sendErr.Status != StatusSuppressed {
		t.Fatalf("status = %q, want %q", sendErr.Status, StatusSuppressed)
	}
	if !sendErr.Permanent() {
		t.Error("a suppressed address must be permanent, so the reaper stops retrying it")
	}
	if got := recorder.last().Status; got != StatusSuppressed {
		t.Errorf("recorded status = %q, want %q", got, StatusSuppressed)
	}
}

func TestSendMessage_InvalidAddressIsPermanentAndRecorded(t *testing.T) {
	recorder := &recordingRecorder{}
	sender := NewSender(&SMTPConfig{Host: "127.0.0.1", Port: "1", From: "noreply@ninerlog.test"})
	sender.SetDeliveryRecorder(recorder)

	err := send(t, sender, "not-an-address")

	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("expected *SendError, got %v", err)
	}
	if sendErr.Status != StatusInvalidAddress || !sendErr.Permanent() {
		t.Fatalf("status = %q permanent = %v", sendErr.Status, sendErr.Permanent())
	}
	if got := recorder.last().Status; got != StatusInvalidAddress {
		t.Errorf("recorded status = %q", got)
	}
}

func TestSendMessage_UnreachableServerIsNotPermanent(t *testing.T) {
	recorder := &recordingRecorder{}
	sender := NewSender(&SMTPConfig{Host: "127.0.0.1", Port: "1", From: "noreply@ninerlog.test"})
	sender.SetDeliveryRecorder(recorder)

	err := send(t, sender, "pilot@example.test")

	var sendErr *SendError
	if !errors.As(err, &sendErr) {
		t.Fatalf("expected *SendError, got %v", err)
	}
	if sendErr.Status != StatusServerError {
		t.Fatalf("status = %q, want %q", sendErr.Status, StatusServerError)
	}
	if sendErr.Permanent() {
		t.Error("an unreachable server must stay retryable, or one outage reaps every unverified account")
	}
}

func TestSendMessage_DryRunWhenSMTPUnconfigured(t *testing.T) {
	recorder := &recordingRecorder{}
	sender := NewSender(&SMTPConfig{From: "noreply@ninerlog.test"})
	sender.SetDeliveryRecorder(recorder)

	if err := send(t, sender, "pilot@example.test"); err != nil {
		t.Fatalf("dry run should not fail: %v", err)
	}
	if got := recorder.last().Status; got != StatusDryRun {
		t.Errorf("recorded status = %q, want %q", got, StatusDryRun)
	}
}

func TestSendMessage_RecipientNeverReachesMessageHeaders(t *testing.T) {
	// The recipient travels in the envelope only (CWE-640). Capture the DATA
	// payload the fake server receives and assert the address is absent.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		reader := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake.test ESMTP")

		var body strings.Builder
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					write("250 2.0.0 Ok")
					received <- body.String()
					inData = false
					continue
				}
				body.WriteString(line + "\n")
				continue
			}
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				write("250-fake.test")
				write("250 SIZE 35882577")
			case strings.HasPrefix(upper, "DATA"):
				inData = true
				write("354 Go ahead")
			case strings.HasPrefix(upper, "QUIT"):
				write("221 Bye")
				return
			default:
				write("250 2.0.0 Ok")
			}
		}
	}()

	host, port, _ := net.SplitHostPort(listener.Addr().String())
	sender := NewSender(&SMTPConfig{Host: host, Port: port, From: "noreply@ninerlog.test"})

	if err := send(t, sender, "pilot@example.test"); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case body := <-received:
		if strings.Contains(body, "pilot@example.test") {
			t.Errorf("recipient leaked into message headers:\n%s", body)
		}
		if !strings.Contains(body, "Subject:") {
			t.Errorf("expected a Subject header, got:\n%s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message data")
	}
}

func TestDeliveryStatus_OnlyRecipientFailuresCondemnAnAddress(t *testing.T) {
	// Guards the rule the suppression list depends on. A status added later
	// that blames the server or the message must not silently start
	// suppressing real addresses.
	recipientFailures := []DeliveryStatus{StatusHardBounce, StatusInvalidAddress, StatusSuppressed}
	others := []DeliveryStatus{StatusDelivered, StatusSoftBounce, StatusRejected, StatusServerError, StatusDryRun}

	for _, s := range recipientFailures {
		if !s.IsRecipientFailure() {
			t.Errorf("%s should be a recipient failure", s)
		}
	}
	for _, s := range others {
		if s.IsRecipientFailure() {
			t.Errorf("%s must NOT be a recipient failure — it would suppress a working address", s)
		}
	}
}

func TestSendError_MessageIncludesCode(t *testing.T) {
	err := &SendError{Status: StatusHardBounce, Code: 550, Err: errors.New("no such user")}
	if got := err.Error(); !strings.Contains(got, "550") || !strings.Contains(got, "hard_bounce") {
		t.Errorf("unhelpful error message: %s", got)
	}
	if !errors.Is(err, err.Err) {
		t.Error("SendError should unwrap to the underlying error")
	}
	_ = fmt.Sprint(err)
}
