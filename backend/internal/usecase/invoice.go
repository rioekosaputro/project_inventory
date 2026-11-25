package usecase

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/clock"
	"rental-inventory-api/pkg/money"
	"rental-inventory-api/pkg/pdf"
	"rental-inventory-api/templates"
)

// InvoiceUsecase mengelola daftar/detail invoice, terbit (issue), void, pembayaran + PPh 23,
// cetak PDF A4, dan export Excel daftar tagihan + rekap PPN (T2.9).
type InvoiceUsecase struct {
	tx          domain.TxManager
	invoices    domain.InvoiceRepository
	audit       domain.AuditRepository
	clk         clock.Clock
	pdfRenderer pdf.Renderer
}

func NewInvoiceUsecase(
	tx domain.TxManager,
	invoices domain.InvoiceRepository,
	audit domain.AuditRepository,
	clk clock.Clock,
) *InvoiceUsecase {
	return &InvoiceUsecase{tx: tx, invoices: invoices, audit: audit, clk: clk}
}

func (u *InvoiceUsecase) WithPDF(r pdf.Renderer) *InvoiceUsecase {
	u.pdfRenderer = r
	return u
}

type IssueInvoiceInput struct {
	FakturPajakNumber *string
}

type VoidInvoiceInput struct {
	VoidReason string
}

type RecordPaymentInput struct {
	PaymentDate     *string // YYYY-MM-DD, default hari ini WIB
	AmountReceived  string
	PPh23Amount     *string
	PPh23SlipNumber *string
	Method          string
	ReferenceNumber *string
	ProofPath       *string
	Notes           *string
}

func (u *InvoiceUsecase) List(ctx context.Context, f domain.InvoiceFilter, p domain.PageParams) ([]domain.Invoice, int64, error) {
	f.Today = todayDateWIB(u.clk.Now())
	return u.invoices.List(ctx, f, p)
}

func (u *InvoiceUsecase) Get(ctx context.Context, id int64) (*domain.InvoiceDetail, error) {
	inv, err := u.invoices.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := u.invoices.ListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	payments, err := u.invoices.ListPayments(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.InvoiceDetail{Invoice: inv, Items: items, Payments: payments}, nil
}

func (u *InvoiceUsecase) Issue(ctx context.Context, actor domain.Principal, id int64, in IssueInvoiceInput) (*domain.InvoiceDetail, error) {
	faktur := cleanOpt(in.FakturPajakNumber)
	if errs := optionalText("faktur_pajak_number", faktur, 40); len(errs) > 0 {
		return nil, masterInvalid("Penerbitan invoice", errs)
	}
	now := u.clk.Now()
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.invoices.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.InvoiceDraft {
			return fmt.Errorf("%w: hanya invoice berstatus DRAFT yang dapat diterbitkan", domain.ErrInvalidStateTransition)
		}
		if err := u.invoices.MarkIssued(ctx, id, actor.UserID, now, faktur); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "INVOICE_ISSUE",
			EntityType: domain.EntityInvoice,
			EntityID:   &id,
			Before:     map[string]any{"status": string(cur.Status)},
			After:      map[string]any{"status": string(domain.InvoiceIssued), "faktur_pajak_number": ptrVal(faktur)},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, id)
}

func (u *InvoiceUsecase) Void(ctx context.Context, actor domain.Principal, id int64, in VoidInvoiceInput) (*domain.InvoiceDetail, error) {
	reason := trimmed(in.VoidReason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 500 {
		return nil, masterInvalid("Pembatalan invoice", []domain.FieldError{
			{Field: "void_reason", Message: "alasan pembatalan wajib diisi (5–500 karakter)"},
		})
	}
	now := u.clk.Now()
	err := u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.invoices.FindByIDForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status != domain.InvoiceDraft && cur.Status != domain.InvoiceIssued {
			return fmt.Errorf("%w: hanya invoice DRAFT atau ISSUED tanpa pembayaran yang dapat di-void", domain.ErrInvalidStateTransition)
		}
		if cur.PaidAmount.GreaterThan(decimal.Zero) {
			return fmt.Errorf("%w: invoice yang sudah memiliki pembayaran tidak dapat di-void", domain.ErrInvalidStateTransition)
		}
		if err := u.invoices.MarkVoided(ctx, id, actor.UserID, now, reason); err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "INVOICE_VOID",
			EntityType: domain.EntityInvoice,
			EntityID:   &id,
			Before:     map[string]any{"status": string(cur.Status)},
			After:      map[string]any{"status": string(domain.InvoiceVoid), "void_reason": reason},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, id)
}

func (u *InvoiceUsecase) RecordPayment(ctx context.Context, actor domain.Principal, invoiceID int64, in RecordPaymentInput) (*domain.InvoiceDetail, error) {
	var errs []domain.FieldError

	amtRecv, err := money.ParseNonNegative(in.AmountReceived)
	if err != nil {
		errs = append(errs, domain.FieldError{Field: "amount_received", Message: "nominal diterima harus angka tidak negatif"})
	}
	pph23 := money.Zero
	if in.PPh23Amount != nil && strings.TrimSpace(*in.PPh23Amount) != "" {
		d, err := money.ParseNonNegative(*in.PPh23Amount)
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "pph23_amount", Message: "PPh 23 harus angka tidak negatif"})
		} else {
			pph23 = d
		}
	}
	totalApplied := money.Round2(amtRecv.Add(pph23))
	if totalApplied.LessThanOrEqual(money.Zero) {
		errs = append(errs, domain.FieldError{Field: "amount_received", Message: "jumlah diterima + PPh 23 harus lebih dari 0"})
	}

	method := domain.PaymentMethod(strings.ToUpper(strings.TrimSpace(in.Method)))
	if !method.Valid() {
		errs = append(errs, domain.FieldError{Field: "method", Message: "metode pembayaran harus TRANSFER, CASH, GIRO, atau OTHER"})
	}

	payDate := todayDateWIB(u.clk.Now())
	if in.PaymentDate != nil && strings.TrimSpace(*in.PaymentDate) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(*in.PaymentDate))
		if err != nil {
			errs = append(errs, domain.FieldError{Field: "payment_date", Message: "format tanggal harus YYYY-MM-DD"})
		} else {
			payDate = parsed
		}
	}

	slip := cleanOpt(in.PPh23SlipNumber)
	ref := cleanOpt(in.ReferenceNumber)
	notes := cleanOpt(in.Notes)
	errs = append(errs, optionalText("pph23_slip_number", slip, 60)...)
	errs = append(errs, optionalText("reference_number", ref, 80)...)
	errs = append(errs, optionalText("notes", notes, 500)...)
	if len(errs) > 0 {
		return nil, masterInvalid("Data pembayaran", errs)
	}

	err = u.tx.WithinTransaction(ctx, func(ctx context.Context) error {
		cur, err := u.invoices.FindByIDForUpdate(ctx, invoiceID)
		if err != nil {
			return err
		}
		if cur.Status != domain.InvoiceIssued && cur.Status != domain.InvoicePartiallyPaid {
			return fmt.Errorf("%w: pembayaran hanya dapat dicatat untuk invoice ISSUED atau PARTIALLY_PAID", domain.ErrInvalidStateTransition)
		}

		newPaid := money.Round2(cur.PaidAmount.Add(totalApplied))
		if newPaid.GreaterThan(cur.GrandTotal) {
			remaining := cur.GrandTotal.Sub(cur.PaidAmount)
			return masterInvalid("Data pembayaran", []domain.FieldError{
				{Field: "amount_received", Message: fmt.Sprintf("total pembayaran melebihi sisa tagihan (%s)", money.FormatRupiah(remaining))},
			})
		}

		newStatus := domain.InvoicePartiallyPaid
		if newPaid.Equal(cur.GrandTotal) {
			newStatus = domain.InvoicePaid
		}

		pm := &domain.Payment{
			InvoiceID:       invoiceID,
			PaymentDate:     payDate,
			AmountReceived:  money.Round2(amtRecv),
			PPh23Amount:     money.Round2(pph23),
			PPh23SlipNumber: slip,
			Method:          method,
			ReferenceNumber: ref,
			ProofPath:       cleanOpt(in.ProofPath),
			Notes:           notes,
			RecordedBy:      actor.UserID,
		}
		saved, err := u.invoices.InsertPaymentAndUpdateInvoice(ctx, pm, newPaid, newStatus)
		if err != nil {
			return err
		}
		return u.audit.Insert(ctx, domain.AuditEntry{
			Action:     "INVOICE_PAYMENT",
			EntityType: domain.EntityInvoice,
			EntityID:   &invoiceID,
			Before:     map[string]any{"status": string(cur.Status), "paid_amount": cur.PaidAmount.StringFixed(2)},
			After: map[string]any{
				"payment_id":      saved.ID,
				"amount_received": saved.AmountReceived.StringFixed(2),
				"pph23_amount":    saved.PPh23Amount.StringFixed(2),
				"paid_amount":     newPaid.StringFixed(2),
				"status":          string(newStatus),
			},
		})
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, invoiceID)
}

func (u *InvoiceUsecase) RenderPDF(ctx context.Context, id int64) ([]byte, string, error) {
	if u.pdfRenderer == nil {
		return nil, "", domain.ErrPDFServiceUnavailable
	}
	det, err := u.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	inv := det.Invoice
	periodLabel := ""
	if inv.BillingPeriodStart != nil && inv.BillingPeriodEnd != nil {
		periodLabel = fmt.Sprintf("%s s/d %s", inv.BillingPeriodStart.Format("2006-01-02"), inv.BillingPeriodEnd.Format("2006-01-02"))
	}
	items := make([]templates.InvoicePDFItem, 0, len(det.Items))
	for i, it := range det.Items {
		items = append(items, templates.InvoicePDFItem{
			No:          i + 1,
			Description: it.Description,
			Qty:         it.Qty.String(),
			UnitPrice:   money.FormatRupiah(it.UnitPrice),
			LineTotal:   money.FormatRupiah(it.LineTotal),
		})
	}
	remaining := inv.GrandTotal.Sub(inv.PaidAmount)
	if remaining.IsNegative() {
		remaining = decimal.Zero
	}
	html, err := templates.RenderInvoicePDF(templates.InvoicePDFData{
		InvoiceNumber:   inv.InvoiceNumber,
		InvoiceType:     string(inv.InvoiceType),
		Status:          string(inv.Status),
		CustomerCode:    inv.CustomerCode,
		CustomerName:    inv.CustomerName,
		CustomerNPWP:    derefStr(inv.CustomerNPWP),
		BillingAddress:  inv.CustomerBillingAddress,
		ContractNumber:  derefStr(inv.ContractNumber),
		PeriodLabel:     periodLabel,
		InvoiceDate:     inv.InvoiceDate.Format("2006-01-02"),
		DueDate:         inv.DueDate.Format("2006-01-02"),
		FakturPajak:     derefStr(inv.FakturPajakNumber),
		IsPPN:           inv.IsPPN,
		Subtotal:        money.FormatRupiah(inv.Subtotal),
		Discount:        money.FormatRupiah(inv.DiscountAmount),
		DPP:             money.FormatRupiah(inv.DPP),
		DPPOtherValue:   money.FormatRupiah(inv.DPPOtherValue),
		TaxRate:         inv.TaxRate.StringFixed(2),
		TaxAmount:       money.FormatRupiah(inv.TaxAmount),
		GrandTotal:      money.FormatRupiah(inv.GrandTotal),
		PaidAmount:      money.FormatRupiah(inv.PaidAmount),
		RemainingAmount: money.FormatRupiah(remaining),
		Notes:           derefStr(inv.Notes),
		PrintedAt:       u.clk.Now().In(clock.WIB).Format("02 Jan 2006 15:04 WIB"),
		Items:           items,
	})
	if err != nil {
		return nil, "", err
	}
	pdfBytes, err := u.pdfRenderer.RenderHTML(ctx, html, pdf.A4Options())
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", domain.ErrPDFServiceUnavailable, err)
	}
	filename := strings.ReplaceAll(inv.InvoiceNumber, "/", "-") + ".pdf"
	return pdfBytes, filename, nil
}

func (u *InvoiceUsecase) ExportExcel(ctx context.Context, f domain.InvoiceFilter) ([]byte, string, error) {
	f.Today = todayDateWIB(u.clk.Now())
	list, err := u.invoices.ListAll(ctx, f)
	if err != nil {
		return nil, "", err
	}

	wb := excelize.NewFile()
	defer func() { _ = wb.Close() }()

	sheet := "Daftar Tagihan"
	_ = wb.SetSheetName("Sheet1", sheet)
	headers := []string{
		"No. Invoice", "Jenis", "Kode Pelanggan", "Pelanggan", "NPWP", "No. Kontrak",
		"Tanggal Invoice", "Jatuh Tempo", "Status", "No. Faktur Pajak",
		"DPP", "DPP Nilai Lain", "PPN", "Grand Total", "Terbayar", "Sisa Piutang",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = wb.SetCellValue(sheet, cell, h)
	}

	totDPP, totDPPOther, totPPN, totGrand, totPaid, totRemain := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	for idx, inv := range list {
		row := idx + 2
		rem := inv.GrandTotal.Sub(inv.PaidAmount)
		vals := []any{
			inv.InvoiceNumber, string(inv.InvoiceType), inv.CustomerCode, inv.CustomerName,
			derefStr(inv.CustomerNPWP), derefStr(inv.ContractNumber),
			inv.InvoiceDate.Format("2006-01-02"), inv.DueDate.Format("2006-01-02"),
			string(inv.Status), derefStr(inv.FakturPajakNumber),
			inv.DPP.StringFixed(2), inv.DPPOtherValue.StringFixed(2), inv.TaxAmount.StringFixed(2),
			inv.GrandTotal.StringFixed(2), inv.PaidAmount.StringFixed(2), rem.StringFixed(2),
		}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			_ = wb.SetCellValue(sheet, cell, v)
		}
		if inv.Status != domain.InvoiceVoid {
			totDPP = totDPP.Add(inv.DPP)
			totDPPOther = totDPPOther.Add(inv.DPPOtherValue)
			totPPN = totPPN.Add(inv.TaxAmount)
			totGrand = totGrand.Add(inv.GrandTotal)
			totPaid = totPaid.Add(inv.PaidAmount)
			totRemain = totRemain.Add(rem)
		}
	}

	recap := "Rekap PPN"
	_, _ = wb.NewSheet(recap)
	recapRows := [][]any{
		{"Ringkasan (Non-VOID)", "Nominal (Rp)"},
		{"Total DPP", totDPP.StringFixed(2)},
		{"Total DPP Nilai Lain (11/12)", totDPPOther.StringFixed(2)},
		{"Total PPN", totPPN.StringFixed(2)},
		{"Total Grand Total", totGrand.StringFixed(2)},
		{"Total Terbayar (incl. PPh 23)", totPaid.StringFixed(2)},
		{"Total Sisa Piutang", totRemain.StringFixed(2)},
	}
	for rIdx, r := range recapRows {
		for cIdx, v := range r {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+1)
			_ = wb.SetCellValue(recap, cell, v)
		}
	}

	var buf bytes.Buffer
	if err := wb.Write(&buf); err != nil {
		return nil, "", err
	}
	fname := fmt.Sprintf("tagihan-%s.xlsx", clock.DateWIB(u.clk.Now()))
	return buf.Bytes(), fname, nil
}
