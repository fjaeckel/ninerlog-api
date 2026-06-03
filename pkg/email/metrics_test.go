package email

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSend_DryRun_RecordsDryRunMetric(t *testing.T) {
	before := testutil.ToFloat64(EmailSendTotal.WithLabelValues("dry_run"))

	cfg := &SMTPConfig{} // not configured -> dry-run path
	sender := NewSender(cfg)
	if err := sender.Send("metrics-dryrun@example.com", "Subject", "<p>Hi</p>"); err != nil {
		t.Fatalf("Send() dry-run error = %v, want nil", err)
	}

	after := testutil.ToFloat64(EmailSendTotal.WithLabelValues("dry_run"))
	if after != before+1 {
		t.Errorf("email_send_total{result=dry_run} = %v, want %v", after, before+1)
	}
}

func TestSend_InvalidAddress_RecordsInvalidAddressMetric(t *testing.T) {
	before := testutil.ToFloat64(EmailSendTotal.WithLabelValues("invalid_address"))

	cfg := &SMTPConfig{}
	sender := NewSender(cfg)
	if err := sender.Send("not-an-email", "Subject", "<p>Hi</p>"); err == nil {
		t.Fatal("Send() with malformed recipient should return an error")
	}

	after := testutil.ToFloat64(EmailSendTotal.WithLabelValues("invalid_address"))
	if after != before+1 {
		t.Errorf("email_send_total{result=invalid_address} = %v, want %v", after, before+1)
	}
}
