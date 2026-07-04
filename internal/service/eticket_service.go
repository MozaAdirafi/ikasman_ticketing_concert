package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	fpdf "github.com/go-pdf/fpdf"
	"github.com/google/uuid"
	resend "github.com/resend/resend-go/v2"
	qrcode "github.com/skip2/go-qrcode"

	db "github.com/MozaAdirafi/ikasman_ticketing_concert/internal/db/sqlc"
)

type EticketService struct {
	q *db.Queries
}

func NewEticketService(q *db.Queries) *EticketService {
	return &EticketService{q: q}
}

type CheckinResult struct {
	Valid        bool   `json:"valid"`
	Message      string `json:"message,omitempty"`
	TicketHolder string `json:"ticket_holder,omitempty"`
	TicketType   string `json:"ticket_type,omitempty"`
}

type ticketGenData struct {
	index      int
	total      int
	ticketName string
	qrPNG      []byte
	pdfData    []byte
}

func (s *EticketService) GenerateAndSend(ctx context.Context, orderID string) error {
	log.Printf("[INFO] E-ticket generation start for order: %s", orderID)

	order, err := s.q.GetOrderWithDetails(ctx, orderID)
	if err != nil {
		return fmt.Errorf("order not found: %w", err)
	}

	fullOrder, err := s.q.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("failed to get order: %w", err)
	}

	// Fetch all item types with their ticket names for this order.
	// When a user buys multiple ticket types (e.g. Gold + Silver), each type
	// is stored as a separate order_item row. We generate one QR per unit.
	orderItems, err := s.q.GetOrderItemsWithTicketNames(ctx, order.ID)
	if err != nil {
		log.Printf("[WARN] Could not fetch order items, falling back to single-ticket mode: %v", err)
	}

	// Build a flat list of (ticketID, ticketName) – one entry per unit to scan.
	type ticketUnit struct {
		ticketID   uuid.UUID
		ticketName string
	}
	var units []ticketUnit

	if len(orderItems) > 0 {
		for _, item := range orderItems {
			for j := int32(0); j < item.Quantity; j++ {
				units = append(units, ticketUnit{
					ticketID:   item.TicketID,
					ticketName: item.TicketName,
				})
			}
		}
		log.Printf("[INFO] Order %s: %d item type(s), %d total units", orderID, len(orderItems), len(units))
	} else {
		// Fallback: use the denormalized quantity on the order row.
		qty := int(fullOrder.Quantity)
		if qty < 1 {
			qty = 1
		}
		for j := 0; j < qty; j++ {
			units = append(units, ticketUnit{
				ticketID:   order.TicketID,
				ticketName: order.TicketName,
			})
		}
		log.Printf("[INFO] Order %s: fallback mode, %d unit(s) from order.quantity", orderID, len(units))
	}

	orderIDShort := strings.ToUpper(orderID[:8])
	totalTickets := len(units)
	var tickets []ticketGenData

	for i, unit := range units {
		ticketNumber := i + 1
		qrValue := uuid.New().String()

		if _, err := s.q.CreateEticket(ctx, db.CreateEticketParams{
			OrderID:  order.ID,
			UserID:   order.UserID,
			TicketID: unit.ticketID,
			QrCode:   qrValue,
		}); err != nil {
			return fmt.Errorf("failed to create eticket %d: %w", ticketNumber, err)
		}

		qrPNG, err := qrcode.Encode(qrValue, qrcode.Medium, 256)
		if err != nil {
			return fmt.Errorf("failed to generate QR code %d: %w", ticketNumber, err)
		}

		pdfData, err := generateTicketPDF(ticketNumber, totalTickets, qrPNG, order.UserName, unit.ticketName, orderIDShort)
		if err != nil {
			log.Printf("[WARN] Failed to generate PDF for ticket %d: %v", ticketNumber, err)
			pdfData = nil
		}

		tickets = append(tickets, ticketGenData{
			index:      ticketNumber,
			total:      totalTickets,
			ticketName: unit.ticketName,
			qrPNG:      qrPNG,
			pdfData:    pdfData,
		})
	}

	if err := sendEticketEmail(
		order.UserEmail, order.UserName,
		orderID, fullOrder.CreatedAt, fullOrder.TotalAmount, tickets,
	); err != nil {
		return fmt.Errorf("failed to send eticket email: %w", err)
	}

	log.Printf("[INFO] E-ticket generation complete for order: %s (%d tickets)", orderID, totalTickets)
	return nil
}

func generateTicketPDF(index, total int, qrPNG []byte, buyerName, ticketType, orderIDShort string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()

	// Header bar
	pdf.SetFillColor(15, 22, 48)
	pdf.Rect(0, 0, pageW, 38, "F")

	pdf.SetTextColor(212, 175, 55)
	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetXY(0, 8)
	pdf.CellFormat(pageW, 12, "KIRRIBILLY", "", 1, "C", false, 0, "")

	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetX(0)
	pdf.CellFormat(pageW, 8, "Road to Liverpool", "", 1, "C", false, 0, "")

	// Ticket index label
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetXY(20, 50)
	pdf.CellFormat(pageW-40, 10, fmt.Sprintf("Tiket %d dari %d", index, total), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// QR code centered
	imgReader := bytes.NewReader(qrPNG)
	pdf.RegisterImageOptionsReader("qr", fpdf.ImageOptions{ImageType: "PNG"}, imgReader)
	qrSize := 90.0
	qrX := (pageW - qrSize) / 2
	curY := pdf.GetY()
	pdf.ImageOptions("qr", qrX, curY, qrSize, qrSize, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	pdf.SetY(curY + qrSize + 8)

	// Details table
	details := [][]string{
		{"Nama", buyerName},
		{"Jenis Tiket", strings.ToUpper(ticketType)},
		{"No. Pesanan", orderIDShort},
		{"Acara", "Kamis, 30 Juli 2026 | 19:30 WIB"},
		{"Tempat", "Deheng House, Jakarta Selatan"},
		{"Validitas", "Valid untuk 1 orang"},
	}
	for _, row := range details {
		pdf.SetX(20)
		pdf.SetFont("Helvetica", "B", 11)
		pdf.CellFormat(45, 8, row[0]+":", "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 11)
		pdf.CellFormat(pageW-65, 8, row[1], "", 1, "L", false, 0, "")
	}

	// Footer
	pdf.SetY(-18)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(pageW, 8, "Tunjukkan QR code ini di pintu masuk venue.", "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sendEticketEmail(toEmail, buyerName, orderID string, createdAt time.Time, totalAmount int64, tickets []ticketGenData) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		return errors.New("RESEND_API_KEY not set")
	}

	fromEmail := os.Getenv("FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "ikasman37@gmail.com"
	}

	orderIDShort := strings.ToUpper(orderID[:8])

	// Build ticket cards HTML using CID inline images (works in Gmail and all clients).
	var ticketCards strings.Builder
	for _, t := range tickets {
		cidID := fmt.Sprintf("qr-%d", t.index)
		ticketCards.WriteString(fmt.Sprintf(`
<div style="background:#fff;border:2px solid #e8e8e8;border-radius:12px;padding:24px;margin-bottom:20px;box-shadow:0 2px 8px rgba(0,0,0,0.07);">
  <div style="text-align:center;margin-bottom:14px;">
    <span style="background:#0f1630;color:#d4af37;font-weight:bold;font-size:13px;padding:5px 18px;border-radius:20px;letter-spacing:1px;">
      Tiket %d dari %d
    </span>
  </div>
  <table style="width:100%%;font-size:14px;margin-bottom:16px;">
    <tr><td style="color:#777;padding:4px 0;width:130px;">Jenis Tiket</td><td style="font-weight:bold;color:#0f1630;">%s</td></tr>
    <tr><td style="color:#777;padding:4px 0;">Nama</td><td style="font-weight:bold;">%s</td></tr>
  </table>
  <div style="text-align:center;padding:16px 0;">
		<img src="cid:%s" width="200" height="200" alt="QR Code"
      style="border:4px solid #0f1630;border-radius:8px;display:block;margin:0 auto;" />
    <p style="font-size:12px;color:#999;margin:10px 0 0;">Scan QR code ini di pintu masuk</p>
  </div>
</div>`,
			t.index, t.total,
			strings.ToUpper(t.ticketName),
			buyerName,
			cidID,
		))
	}

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f0f0f0;font-family:Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f0f0f0;padding:30px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.12);">

<!-- HEADER -->
<tr><td style="background:#0f1630;padding:40px;text-align:center;">
  <div style="color:#d4af37;font-size:40px;font-weight:900;letter-spacing:8px;line-height:1;">KIRRIBILLY</div>
  <div style="color:#ffffff;font-size:15px;margin-top:8px;letter-spacing:3px;opacity:0.9;">Road to Liverpool</div>
</td></tr>

<!-- GREETING -->
<tr><td style="padding:32px 40px 0;">
  <p style="font-size:18px;color:#222;margin:0 0 8px;">Hi <strong>%s</strong>,</p>
  <p style="font-size:15px;color:#666;margin:0;line-height:1.6;">Terima kasih telah membeli tiket! Berikut adalah e-ticket Anda.</p>
</td></tr>

<!-- ORDER DETAILS -->
<tr><td style="padding:24px 40px 0;">
  <div style="background:#f8f9fa;border-radius:10px;padding:20px 24px;">
    <h3 style="margin:0 0 14px;font-size:12px;text-transform:uppercase;letter-spacing:2px;color:#0f1630;border-bottom:1px solid #e0e0e0;padding-bottom:10px;">Order Details</h3>
    <table style="width:100%%;font-size:14px;border-collapse:collapse;">
      <tr><td style="color:#888;padding:5px 0;width:175px;">No. Pesanan</td><td style="font-weight:bold;font-family:monospace;">%s</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Waktu Pemesanan</td><td>%s</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Total</td><td style="font-weight:bold;color:#0f1630;font-size:15px;">Rp %s</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Metode Pembayaran</td><td>Midtrans</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Pembeli</td><td>%s &mdash; %s</td></tr>
    </table>
  </div>
</td></tr>

<!-- EVENT DETAILS -->
<tr><td style="padding:16px 40px 0;">
  <div style="background:#f8f9fa;border-radius:10px;padding:20px 24px;">
    <h3 style="margin:0 0 14px;font-size:12px;text-transform:uppercase;letter-spacing:2px;color:#0f1630;border-bottom:1px solid #e0e0e0;padding-bottom:10px;">Event Details</h3>
    <table style="width:100%%;font-size:14px;border-collapse:collapse;">
      <tr><td style="color:#888;padding:5px 0;width:175px;">Acara</td><td style="font-weight:bold;">Kirribilly &mdash; Road to Liverpool</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Featuring</td><td>Cakra Khan &amp; Astrid</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Tanggal</td><td style="font-weight:bold;">Kamis, 30 Juli 2026</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Waktu</td><td>19:30 &ndash; 22:00 WIB</td></tr>
      <tr><td style="color:#888;padding:5px 0;">Tempat</td><td>Deheng House, Jl. Taman Kemang No.32, Jakarta Selatan</td></tr>
    </table>
  </div>
</td></tr>

<!-- TICKETS -->
<tr><td style="padding:24px 40px 0;">
  <h3 style="margin:0 0 16px;font-size:12px;text-transform:uppercase;letter-spacing:2px;color:#0f1630;">Tiket Anda</h3>
  %s
</td></tr>

<!-- FOOTER -->
<tr><td style="padding:24px 40px 36px;">
  <div style="background:#0f1630;border-radius:10px;padding:18px 24px;text-align:center;">
    <p style="color:#d4af37;font-size:14px;margin:0 0 6px;font-weight:bold;">Tunjukkan QR code ini di pintu masuk venue.</p>
    <p style="color:#aaaaaa;font-size:12px;margin:0;">Tiket tidak perlu dicetak.</p>
  </div>
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`,
		buyerName,
		orderIDShort,
		formatIndonesianDate(createdAt),
		formatRupiah(totalAmount),
		buyerName, toEmail,
		ticketCards.String(),
	)

	var attachments []*resend.Attachment
	for _, t := range tickets {
		// Attach QR PNG as inline CID image — works in Gmail and all major email clients.
		cidID := fmt.Sprintf("qr-%d", t.index)
		attachments = append(attachments, &resend.Attachment{
			Filename:        fmt.Sprintf("qr-%d.png", t.index),
			Content:         t.qrPNG,
			ContentType:     "image/png",
			ContentId:       cidID,
			InlineContentId: cidID,
		})
		if t.pdfData != nil {
			attachments = append(attachments, &resend.Attachment{
				Filename: fmt.Sprintf("tiket-%d-%s.pdf", t.index, strings.ToLower(orderID[:8])),
				Content:  t.pdfData,
			})
		}
	}

	client := resend.NewClient(apiKey)
	params := &resend.SendEmailRequest{
		From:        fromEmail,
		To:          []string{toEmail},
		Subject:     fmt.Sprintf("E-Ticket Kirribilly Road to Liverpool - %s", buyerName),
		Html:        htmlBody,
		Attachments: attachments,
	}

	_, err := client.Emails.Send(params)
	return err
}

func formatRupiah(amount int64) string {
	s := strconv.FormatInt(amount, 10)
	var result strings.Builder
	n := len(s)
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteByte('.')
		}
		result.WriteRune(c)
	}
	return result.String()
}

func formatIndonesianDate(t time.Time) string {
	days := []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	months := []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("%s, %d %s %d %02d:%02d WIB",
		days[t.Weekday()], t.Day(), months[t.Month()], t.Year(), t.Hour(), t.Minute())
}

func isValidUUID(u string) bool {
	_, err := uuid.Parse(u)
	return err == nil
}

func (s *EticketService) Checkin(ctx context.Context, qrCode string) (*CheckinResult, error) {
	eticket, err := s.q.GetEticketByQRCode(ctx, qrCode)
	if err != nil {
		return &CheckinResult{Valid: false, Message: "Ticket not found"}, nil
	}

	if eticket.IsUsed {
		return &CheckinResult{Valid: false, Message: "Ticket already used"}, nil
	}

	if err := s.q.MarkEticketUsed(ctx, db.MarkEticketUsedParams{
		QrCode: qrCode,
		UsedAt: time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("failed to mark ticket used: %w", err)
	}

	details, err := s.q.GetEticketDetails(ctx, qrCode)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticket details: %w", err)
	}

	return &CheckinResult{
		Valid:        true,
		TicketHolder: details.UserName,
		TicketType:   details.TicketName,
	}, nil
}
