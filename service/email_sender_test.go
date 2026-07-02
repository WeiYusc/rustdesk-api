package service

import (
	"context"
	"strings"
	"testing"
)

func TestSettingsServiceGetSMTPForSendPreservesPasswordWhileGetSMTPMasks(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}
	settings := SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: SMTPSecurityStartTLS, Username: "user", Password: "send-secret", FromEmail: "noreply@example.test"}
	if err := svc.SaveSMTP(settings, 1); err != nil {
		t.Fatalf("SaveSMTP error: %v", err)
	}

	masked, err := svc.GetSMTP()
	if err != nil {
		t.Fatalf("GetSMTP error: %v", err)
	}
	if masked.Password != "" || !masked.HasPassword {
		t.Fatalf("GetSMTP password/has_password = %q/%v, want masked/true", masked.Password, masked.HasPassword)
	}

	forSend, err := svc.GetSMTPForSend()
	if err != nil {
		t.Fatalf("GetSMTPForSend error: %v", err)
	}
	if forSend.Password != "send-secret" || !forSend.HasPassword {
		t.Fatalf("GetSMTPForSend password/has_password = %q/%v, want preserved/true", forSend.Password, forSend.HasPassword)
	}
}

type recordingEmailTransport struct {
	settings SMTPSettings
	message  SMTPMessage
}

func (r *recordingEmailTransport) Send(ctx context.Context, settings SMTPSettings, message SMTPMessage) error {
	r.settings = settings
	r.message = message
	return nil
}

func TestSMTPSenderValidation(t *testing.T) {
	sender := NewSMTPSender(nil)
	if err := sender.Send(context.Background(), SMTPSettings{}, SMTPMessage{To: "user@example.com", Subject: "Subject", TextBody: "body"}); err == nil {
		t.Fatalf("Send with disabled config succeeded")
	}
	if err := sender.Send(context.Background(), SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: "bogus", FromEmail: "noreply@example.test"}, SMTPMessage{To: "user@example.com", Subject: "Subject", TextBody: "body"}); err == nil {
		t.Fatalf("Send with invalid security succeeded")
	}
	if err := sender.Send(context.Background(), SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: SMTPSecurityStartTLS, FromEmail: "not-an-email"}, SMTPMessage{To: "user@example.com", Subject: "Subject", TextBody: "body"}); err == nil {
		t.Fatalf("Send with invalid from email succeeded")
	}
	if err := sender.Send(context.Background(), SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: SMTPSecurityStartTLS, FromEmail: "noreply@example.test"}, SMTPMessage{To: "not-an-email", Subject: "Subject", TextBody: "body"}); err == nil {
		t.Fatalf("Send with invalid recipient succeeded")
	}
}

func TestSMTPSenderComposesAndUsesInjectedTransport(t *testing.T) {
	transport := &recordingEmailTransport{}
	sender := NewSMTPSender(transport)
	settings := SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: SMTPSecurityStartTLS, Username: "user", Password: "smtp-secret", FromEmail: "noreply@example.test", FromName: "RustDesk API", ReplyTo: "support@example.test"}
	message := SMTPMessage{To: "user@example.test", Subject: "Verify your email", TextBody: "Your code is 123456"}
	if err := sender.Send(context.Background(), settings, message); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if transport.settings.Password != "smtp-secret" {
		t.Fatalf("transport received password %q, want unmasked", transport.settings.Password)
	}
	body := transport.message.Raw
	for _, want := range []string{"From: \"RustDesk API\" <noreply@example.test>", "To: user@example.test", "Reply-To: support@example.test", "Subject: Verify your email", "Content-Type: text/plain; charset=UTF-8", "Your code is 123456"} {
		if !strings.Contains(body, want) {
			t.Fatalf("composed message missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "smtp-secret") {
		t.Fatalf("composed message exposed SMTP password")
	}
}
