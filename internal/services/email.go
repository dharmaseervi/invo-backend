package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type EmailService struct {
	apiKey    string
	fromEmail string
	fromName  string
}

func NewEmailService(apiKey, fromEmail, fromName string) *EmailService {
	return &EmailService{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

type resendAttachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"` // base64
}

type resendRequest struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

func (s *EmailService) send(to, subject, html string, attachments []resendAttachment) error {
	payload := resendRequest{
		From:        fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail),
		To:          []string{to},
		Subject:     subject,
		HTML:        html,
		Attachments: attachments,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("resend error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Email sent to %s, status: %d", to, resp.StatusCode)
	return nil
}

// --- Public methods (same signatures as before) ---

func (s *EmailService) SendInvoiceEmail(
	toEmail, toName, invoiceNumber string,
	invoicePDF []byte,
) error {
	subject := fmt.Sprintf("Invoice %s from %s", invoiceNumber, s.fromName)

	html := fmt.Sprintf(`
		<h2>Invoice %s</h2>
		<p>Dear %s,</p>
		<p>Please find your invoice attached.</p>
		<p>Thank you for your business!</p>
		<br/>
		<p>Regards,<br/>%s</p>
	`, invoiceNumber, toName, s.fromName)

	attachments := []resendAttachment{
		{
			Filename: fmt.Sprintf("invoice-%s.pdf", invoiceNumber),
			Content:  base64.StdEncoding.EncodeToString(invoicePDF),
		},
	}

	return s.send(toEmail, subject, html, attachments)
}

func (s *EmailService) SendOTPEmail(toEmail, code string) error {
	subject := "Your Invo Billing Login Code"

	html := fmt.Sprintf(`
	<div style="font-family:Arial,sans-serif;max-width:420px;margin:0 auto;padding:32px;">
		<h2 style="color:#1A1A1A;margin-bottom:8px;">Login Code</h2>
		<p style="color:#666;margin-bottom:24px;">
			Use this code to login to Invo Billing. It expires in 10 minutes.
		</p>
		<div style="background:#f5f5f5;border-radius:8px;padding:24px;text-align:center;">
			<span style="font-size:40px;font-weight:bold;letter-spacing:12px;color:#1A1A1A;">
				%s
			</span>
		</div>
		<p style="color:#999;font-size:12px;margin-top:24px;">
			If you didn't request this, ignore this email.
			Never share this code with anyone.
		</p>
	</div>
	`, code)

	return s.send(toEmail, subject, html, nil)
}

func (s *EmailService) SendVerificationEmail(toEmail, code string) error {
	subject := "Verify your Invo Billing account"

	html := fmt.Sprintf(`
	<div style="font-family:Arial,sans-serif;max-width:420px;margin:0 auto;padding:32px;">
		<h2 style="color:#1A1A1A;margin-bottom:8px;">Verify Your Email</h2>
		<p style="color:#666;margin-bottom:24px;">
			Welcome to Invo Billing! Enter this code to verify your account:
		</p>
		<div style="background:#f5f5f5;border-radius:8px;padding:24px;text-align:center;">
			<span style="font-size:40px;font-weight:bold;letter-spacing:12px;color:#1A1A1A;">
				%s
			</span>
		</div>
		<p style="color:#999;font-size:12px;margin-top:24px;">
			This code expires in 10 minutes.<br/>
			If you didn't create an account, ignore this email.
		</p>
	</div>
	`, code)

	return s.send(toEmail, subject, html, nil)
}

func (s *EmailService) SendPasswordResetEmail(toEmail, code string) error {
	subject := "Reset your Invo Billing password"

	html := fmt.Sprintf(`
	<div style="font-family:Arial,sans-serif;max-width:420px;margin:0 auto;padding:32px;">
		<h2 style="color:#1A1A1A;margin-bottom:8px;">Reset Your Password</h2>
		<p style="color:#666;margin-bottom:24px;">
			Enter this code to reset your Invo Billing password:
		</p>
		<div style="background:#f5f5f5;border-radius:8px;padding:24px;text-align:center;">
			<span style="font-size:40px;font-weight:bold;letter-spacing:12px;color:#1A1A1A;">
				%s
			</span>
		</div>
		<p style="color:#999;font-size:12px;margin-top:24px;">
			This code expires in <strong>10 minutes</strong>.<br/>
			If you didn't request a password reset, ignore this email.
		</p>
	</div>
	`, code)

	return s.send(toEmail, subject, html, nil)
}
