package admin

import "strings"

func classifySMTPDeliveryError(err error) string {
	if err == nil {
		return "SMTPTestFailed"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "auth") || strings.Contains(message, "535") || strings.Contains(message, "username") || strings.Contains(message, "password"):
		return "SMTPTestAuthFailed"
	case strings.Contains(message, "tls") || strings.Contains(message, "certificate") || strings.Contains(message, "starttls") || strings.Contains(message, "x509") || strings.Contains(message, "unknown authority") || strings.Contains(message, "handshake"):
		return "SMTPTestTLSFailed"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline"):
		return "SMTPTestTimeout"
	case strings.Contains(message, "connection refused") || strings.Contains(message, "no such host") || strings.Contains(message, "network") || strings.Contains(message, "dial") || strings.Contains(message, "eof") || strings.Contains(message, "reset by peer"):
		return "SMTPTestConnectionFailed"
	case strings.Contains(message, "recipient") || strings.Contains(message, "rcpt") || strings.Contains(message, "sender") || strings.Contains(message, "mail from"):
		return "SMTPTestRejected"
	default:
		return "SMTPTestFailed"
	}
}

func classifySMTPTestError(err error) string {
	return classifySMTPDeliveryError(err)
}
