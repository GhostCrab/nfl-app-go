package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/smtp"
)

// EmailConfig holds SMTP configuration
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
}

// EmailService handles sending emails
type EmailService struct {
	config EmailConfig
}

// NewEmailService creates a new email service
func NewEmailService(config EmailConfig) *EmailService {
	return &EmailService{
		config: config,
	}
}

// SendPasswordResetEmail sends a password reset email
func (e *EmailService) SendPasswordResetEmail(toEmail, toName, resetToken, baseURL string) error {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, resetToken)
	
	// Email subject and body
	subject := "NFL Games - Password Reset"
	
	// HTML email template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Password Reset</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; margin: 0; padding: 20px; background-color: #f4f4f4; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 30px; }
        .header h1 { color: #2c3e50; margin: 0; }
        .content { margin-bottom: 30px; }
        .button { display: inline-block; padding: 12px 24px; background-color: #3b82f6; color: white; text-decoration: none; border-radius: 4px; font-weight: bold; }
        .button:hover { background-color: #2563eb; }
        .footer { text-align: center; font-size: 0.9em; color: #666; margin-top: 30px; padding-top: 20px; border-top: 1px solid #eee; }
        .warning { background-color: #fff3cd; border: 1px solid #ffeaa7; padding: 15px; border-radius: 4px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🏈 NFL Games</h1>
            <h2>Password Reset Request</h2>
        </div>
        
        <div class="content">
            <p>Hello {{.Name}},</p>
            
            <p>We received a request to reset your password for your NFL Games account. If you made this request, click the button below to reset your password:</p>
            
            <p style="text-align: center; margin: 30px 0;">
                <a href="{{.ResetURL}}" class="button">Reset Your Password</a>
            </p>
            
            <div class="warning">
                <strong>Important:</strong>
                <ul>
                    <li>This link will expire in 24 hours</li>
                    <li>If you didn't request a password reset, you can safely ignore this email</li>
                    <li>For security, don't share this link with anyone</li>
                </ul>
            </div>
            
            <p>If the button doesn't work, you can copy and paste this link into your browser:</p>
            <p style="word-break: break-all; background-color: #f8f9fa; padding: 10px; border-radius: 4px; font-family: monospace;">
                {{.ResetURL}}
            </p>
        </div>
        
        <div class="footer">
            <p>This email was sent to {{.Email}} because a password reset was requested for your NFL Games account.</p>
            <p>If you have any questions, please contact your league administrator.</p>
        </div>
    </div>
</body>
</html>`

	// Plain text version
	textTemplate := `
NFL Games - Password Reset

Hello {{.Name}},

We received a request to reset your password for your NFL Games account.

Reset your password by visiting this link:
{{.ResetURL}}

Important:
- This link will expire in 24 hours
- If you didn't request a password reset, you can safely ignore this email
- For security, don't share this link with anyone

This email was sent to {{.Email}} because a password reset was requested for your NFL Games account.
`

	// Template data
	data := struct {
		Name     string
		Email    string
		ResetURL string
	}{
		Name:     toName,
		Email:    toEmail,
		ResetURL: resetURL,
	}

	// Parse templates
	htmlTmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %v", err)
	}

	textTmpl, err := template.New("text").Parse(textTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse text template: %v", err)
	}

	// Execute templates
	var htmlBody bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBody, data); err != nil {
		return fmt.Errorf("failed to execute HTML template: %v", err)
	}

	var textBody bytes.Buffer
	if err := textTmpl.Execute(&textBody, data); err != nil {
		return fmt.Errorf("failed to execute text template: %v", err)
	}

	// Send email
	return e.sendEmail(toEmail, subject, textBody.String(), htmlBody.String())
}

// sendEmail sends an email using SMTP with TLS support
func (e *EmailService) sendEmail(to, subject, textBody, htmlBody string) error {
	// SMTP server configuration
	smtpAddr := fmt.Sprintf("%s:%s", e.config.SMTPHost, e.config.SMTPPort)
	
	// Connect to the SMTP server
	conn, err := net.Dial("tcp", smtpAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %v", err)
	}
	defer client.Close()

	// Start TLS if supported
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName:         e.config.SMTPHost,
			InsecureSkipVerify: false,
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	// Authentication
	auth := smtp.PlainAuth("", e.config.SMTPUsername, e.config.SMTPPassword, e.config.SMTPHost)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %v", err)
	}

	// Set sender
	if err = client.Mail(e.config.FromEmail); err != nil {
		return fmt.Errorf("failed to set sender: %v", err)
	}

	// Set recipient
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %v", err)
	}

	// Send email body
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %v", err)
	}
	defer writer.Close()

	// Email headers and body
	from := fmt.Sprintf("%s <%s>", e.config.FromName, e.config.FromEmail)
	
	// Create multipart message
	boundary := "boundary123456789"
	
	msg := fmt.Sprintf(`From: %s
To: %s
Subject: %s
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="%s"

--%s
Content-Type: text/plain; charset=UTF-8

%s

--%s
Content-Type: text/html; charset=UTF-8

%s

--%s--
`, from, to, subject, boundary, boundary, textBody, boundary, htmlBody, boundary)

	_, err = writer.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("failed to write email body: %v", err)
	}

	log.Printf("Password reset email sent successfully to %s", to)
	return nil
}

// IsConfigured checks if the email service is properly configured
func (e *EmailService) IsConfigured() bool {
	return e.config.SMTPHost != "" && 
		   e.config.SMTPPort != "" && 
		   e.config.SMTPUsername != "" && 
		   e.config.SMTPPassword != "" && 
		   e.config.FromEmail != ""
}

// TestConnection tests the SMTP connection
func (e *EmailService) TestConnection() error {
	if !e.IsConfigured() {
		return fmt.Errorf("email service not configured")
	}

	smtpAddr := fmt.Sprintf("%s:%s", e.config.SMTPHost, e.config.SMTPPort)
	
	// Connect to the SMTP server
	conn, err := net.Dial("tcp", smtpAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %v", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %v", err)
	}
	defer client.Close()

	// Start TLS if supported
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName:         e.config.SMTPHost,
			InsecureSkipVerify: false,
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	// Test authentication
	auth := smtp.PlainAuth("", e.config.SMTPUsername, e.config.SMTPPassword, e.config.SMTPHost)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %v", err)
	}

	return nil
}

// GetSupportedProviders returns commonly used email providers and their settings
func GetSupportedProviders() map[string]EmailConfig {
	return map[string]EmailConfig{
		"gmail": {
			SMTPHost: "smtp.gmail.com",
			SMTPPort: "587",
		},
		"outlook": {
			SMTPHost: "smtp-mail.outlook.com",
			SMTPPort: "587",
		},
		"yahoo": {
			SMTPHost: "smtp.mail.yahoo.com",
			SMTPPort: "587",
		},
		"smtp2go": {
			SMTPHost: "mail.smtp2go.com",
			SMTPPort: "587",
		},
	}
}