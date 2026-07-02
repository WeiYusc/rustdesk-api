package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type SMTPMessage struct {
	To       string
	Subject  string
	TextBody string
	Raw      string
}

type EmailTransport interface {
	Send(ctx context.Context, settings SMTPSettings, message SMTPMessage) error
}

type SMTPSender struct {
	transport EmailTransport
}

func NewSMTPSender(transport EmailTransport) *SMTPSender {
	if transport == nil {
		transport = smtpNetworkTransport{}
	}
	return &SMTPSender{transport: transport}
}

func (s *SMTPSender) Send(ctx context.Context, settings SMTPSettings, message SMTPMessage) error {
	settings.applyDefaults()
	if err := validateSMTPForSend(settings); err != nil {
		return err
	}
	if err := validateSMTPMessage(message); err != nil {
		return err
	}
	message.Raw = ComposeSMTPMessage(settings, message)
	return s.transport.Send(ctx, settings, message)
}

func validateSMTPForSend(settings SMTPSettings) error {
	if !settings.Ready() {
		return fmt.Errorf("smtp settings are not ready")
	}
	if err := settings.validate(); err != nil {
		return err
	}
	if _, err := mail.ParseAddress(settings.FromEmail); err != nil {
		return fmt.Errorf("invalid smtp from email: %w", err)
	}
	if strings.TrimSpace(settings.ReplyTo) != "" {
		if _, err := mail.ParseAddress(settings.ReplyTo); err != nil {
			return fmt.Errorf("invalid smtp reply-to email: %w", err)
		}
	}
	return nil
}

func validateSMTPMessage(message SMTPMessage) error {
	if strings.TrimSpace(message.To) == "" {
		return fmt.Errorf("email recipient is required")
	}
	if _, err := mail.ParseAddress(message.To); err != nil {
		return fmt.Errorf("invalid email recipient: %w", err)
	}
	if strings.TrimSpace(message.Subject) == "" {
		return fmt.Errorf("email subject is required")
	}
	if strings.TrimSpace(message.TextBody) == "" {
		return fmt.Errorf("email text body is required")
	}
	return nil
}

func ComposeSMTPMessage(settings SMTPSettings, message SMTPMessage) string {
	from := mail.Address{Name: settings.FromName, Address: settings.FromEmail}
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from.String())
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(message.To)
	b.WriteString("\r\n")
	if strings.TrimSpace(settings.ReplyTo) != "" {
		b.WriteString("Reply-To: ")
		b.WriteString(settings.ReplyTo)
		b.WriteString("\r\n")
	}
	b.WriteString("Subject: ")
	b.WriteString(sanitizeHeader(message.Subject))
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(message.TextBody)
	return b.String()
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}

type smtpNetworkTransport struct{}

func (smtpNetworkTransport) Send(ctx context.Context, settings SMTPSettings, message SMTPMessage) error {
	address := net.JoinHostPort(settings.Host, fmt.Sprintf("%d", settings.Port))
	timeout := time.Duration(settings.TimeoutSeconds) * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	dialer := &net.Dialer{Timeout: timeout}
	var conn net.Conn
	var err error
	if settings.Security == SMTPSecurityTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: settings.Host, InsecureSkipVerify: settings.InsecureSkipVerify})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	client, err := smtp.NewClient(conn, settings.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if settings.Security == SMTPSecurityStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: settings.Host, InsecureSkipVerify: settings.InsecureSkipVerify}); err != nil {
			return err
		}
	}
	if settings.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", settings.Username, settings.Password, settings.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(settings.FromEmail); err != nil {
		return err
	}
	recipient, err := mail.ParseAddress(message.To)
	if err != nil {
		return err
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message.Raw)); err != nil {
		writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
