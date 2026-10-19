package domain

import (
	"strings"
	"testing"
)

const sgmlSample = "OFXHEADER:100\r\n" +
	"DATA:OFXSGML\r\n" +
	"VERSION:102\r\n" +
	"SECURITY:NONE\r\n" +
	"ENCODING:USASCII\r\n" +
	"CHARSET:1252\r\n" +
	"COMPRESSION:NONE\r\n" +
	"OLDFILEUID:NONE\r\n" +
	"NEWFILEUID:NONE\r\n" +
	"\r\n" +
	"<OFX>\r\n" +
	"<BANKMSGSRSV1>\r\n" +
	"<STMTTRNRS>\r\n" +
	"<STMTRS>\r\n" +
	"<BANKTRANLIST>\r\n" +
	"<STMTTRN>\r\n" +
	"<TRNTYPE>DEBIT\r\n" +
	"<DTPOSTED>20261001120000[-3:BRT]\r\n" +
	"<TRNAMT>-123.45\r\n" +
	"<FITID>NUB-001\r\n" +
	"<NAME>MERCADO SILVA\r\n" +
	"<MEMO>Compra no mercado\r\n" +
	"</STMTTRN>\r\n" +
	"<STMTTRN>\r\n" +
	"<TRNTYPE>CREDIT\r\n" +
	"<DTPOSTED>20261002\r\n" +
	"<TRNAMT>50\r\n" +
	"<FITID>NUB-002\r\n" +
	"<NAME>SALARIO\r\n" +
	"<MEMO>\r\n" +
	"</STMTTRN>\r\n" +
	"</BANKTRANLIST>\r\n" +
	"</STMTRS>\r\n" +
	"</STMTTRNRS>\r\n" +
	"</BANKMSGSRSV1>\r\n" +
	"</OFX>\r\n"

const xmlSample = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
	`<?OFX OFXHEADER="200" VERSION="211" SECURITY="NONE" OLDFILEUID="NONE" NEWFILEUID="NONE"?>` + "\n" +
	"<OFX>\n" +
	"<BANKMSGSRSV1>\n" +
	"<STMTTRNRS>\n" +
	"<STMTRS>\n" +
	"<BANKTRANLIST>\n" +
	"<STMTTRN>\n" +
	"<TRNTYPE>DEBIT</TRNTYPE>\n" +
	"<DTPOSTED>20261001120000[-3:BRT]</DTPOSTED>\n" +
	"<TRNAMT>-123.45</TRNAMT>\n" +
	"<FITID>NUB-001</FITID>\n" +
	"<NAME>MERCADO SILVA</NAME>\n" +
	"<MEMO>Compra no mercado</MEMO>\n" +
	"</STMTTRN>\n" +
	"<STMTTRN>\n" +
	"<TRNTYPE>CREDIT</TRNTYPE>\n" +
	"<DTPOSTED>20261002</DTPOSTED>\n" +
	"<TRNAMT>50</TRNAMT>\n" +
	"<FITID>NUB-002</FITID>\n" +
	"<NAME>SALARIO</NAME>\n" +
	"<MEMO></MEMO>\n" +
	"</STMTTRN>\n" +
	"</BANKTRANLIST>\n" +
	"</STMTRS>\n" +
	"</STMTTRNRS>\n" +
	"</BANKMSGSRSV1>\n" +
	"</OFX>\n"

func TestParseOFXSGMLProducesOneRowPerTransaction(t *testing.T) {
	rows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
}

func TestParseOFXSGMLAndXMLAreEquivalent(t *testing.T) {
	sgmlRows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatalf("ParseOFX sgml: %v", err)
	}
	xmlRows, err := ParseOFX([]byte(xmlSample))
	if err != nil {
		t.Fatalf("ParseOFX xml: %v", err)
	}
	if len(sgmlRows) != len(xmlRows) {
		t.Fatalf("sgml has %d rows, xml has %d", len(sgmlRows), len(xmlRows))
	}
	for i := range sgmlRows {
		if sgmlRows[i] != xmlRows[i] {
			t.Fatalf("row %d differs: sgml=%+v xml=%+v", i, sgmlRows[i], xmlRows[i])
		}
	}
}

func TestParseOFXDTPostedWithAndWithoutTime(t *testing.T) {
	rows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Date != "2026-10-01" {
		t.Fatalf("date with time = %q", rows[0].Date)
	}
	if rows[1].Date != "2026-10-02" {
		t.Fatalf("date without time = %q", rows[1].Date)
	}
}

func TestParseOFXNegativeAmountIsExpenseWithPositiveCents(t *testing.T) {
	rows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Type != Expense || rows[0].AmountCents != 12345 {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1].Type != Income || rows[1].AmountCents != 5000 {
		t.Fatalf("row1 = %+v", rows[1])
	}
}

func TestParseOFXMemoEmptyFallsBackToName(t *testing.T) {
	rows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Description != "Compra no mercado" {
		t.Fatalf("description with memo = %q", rows[0].Description)
	}
	if rows[1].Description != "SALARIO" {
		t.Fatalf("description falling back to name = %q", rows[1].Description)
	}
}

func TestParseOFXFITIDIsCaptured(t *testing.T) {
	rows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].FITID != "NUB-001" || rows[1].FITID != "NUB-002" {
		t.Fatalf("fitids = %q, %q", rows[0].FITID, rows[1].FITID)
	}
}

func TestParseOFXAcceptsLFOnly(t *testing.T) {
	lf := strings.ReplaceAll(sgmlSample, "\r\n", "\n")
	rows, err := ParseOFX([]byte(lf))
	if err != nil {
		t.Fatalf("ParseOFX LF: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
}

func TestParseOFXEmptyStatementIsValidWithZeroRows(t *testing.T) {
	empty := "<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST></BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>"
	rows, err := ParseOFX([]byte(empty))
	if err != nil {
		t.Fatalf("ParseOFX empty: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows, want 0", len(rows))
	}
}

func TestParseOFXRejectsRandomText(t *testing.T) {
	if _, err := ParseOFX([]byte("this is not an ofx file at all")); err == nil {
		t.Fatal("expected error for random text")
	}
}

func TestParseOFXRejectsUnclosedAggregate(t *testing.T) {
	malformed := "<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><BANKTRANLIST><STMTTRN><FITID>1<TRNAMT>-1<DTPOSTED>20260101"
	if _, err := ParseOFX([]byte(malformed)); err == nil {
		t.Fatal("expected error for unclosed STMTTRN")
	}
}

func TestParseOFXRejectsMissingFITID(t *testing.T) {
	missing := "<OFX><BANKTRANLIST><STMTTRN><TRNAMT>-10<DTPOSTED>20260101<NAME>X</STMTTRN></BANKTRANLIST></OFX>"
	if _, err := ParseOFX([]byte(missing)); err == nil {
		t.Fatal("expected error for missing FITID")
	}
}

func TestNormalizeDescriptionUppercasesAndCollapsesSpaces(t *testing.T) {
	got := NormalizeDescription("  compra   no   Mercado  ")
	if got != "COMPRA NO MERCADO" {
		t.Fatalf("got %q", got)
	}
}

func TestParseOFXDecodesCP1252DeclaredInSGMLHeader(t *testing.T) {
	sgml := "OFXHEADER:100\r\n" +
		"DATA:OFXSGML\r\n" +
		"VERSION:102\r\n" +
		"CHARSET:1252\r\n" +
		"\r\n" +
		"<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>1<MEMO>Farm" + string([]byte{0xE1}) + "cia S" + string([]byte{0xE3}) + "o Jo" + string([]byte{0xE3}) + "o</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if want := "Farmácia São João"; rows[0].Description != want {
		t.Fatalf("description = %q, want %q", rows[0].Description, want)
	}
}

func TestParseOFXDecodesCP1252DeclaredInXMLProcessingInstruction(t *testing.T) {
	xml := `<?xml version="1.0" encoding="ISO-8859-1"?>` +
		"<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001</DTPOSTED><TRNAMT>-10</TRNAMT><FITID>1</FITID>" +
		"<MEMO>Farm" + string([]byte{0xE1}) + "cia</MEMO></STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(xml))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if want := "Farmácia"; rows[0].Description != want {
		t.Fatalf("description = %q, want %q", rows[0].Description, want)
	}
}

func TestParseOFXFallsBackToCP1252WhenNotDeclaredAndNotValidUTF8(t *testing.T) {
	sgml := "<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>1<MEMO>Jo" + string([]byte{0xE3}) + "o</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if want := "João"; rows[0].Description != want {
		t.Fatalf("description = %q, want %q", rows[0].Description, want)
	}
}

func TestParseOFXIgnoresWrongDeclaredEncodingWhenBodyIsValidUTF8(t *testing.T) {
	sgml := "OFXHEADER:100\r\n" +
		"DATA:OFXSGML\r\n" +
		"VERSION:102\r\n" +
		"CHARSET:1252\r\n" +
		"\r\n" +
		"<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>1<MEMO>Farmácia São João</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if want := "Farmácia São João"; rows[0].Description != want {
		t.Fatalf("description = %q, want %q (a CHARSET:1252 header must not force a double-decode of a genuinely UTF-8 body)", rows[0].Description, want)
	}
}

func TestParseOFXDecodesCP1252WhenHeaderWronglyDeclaresUSASCII(t *testing.T) {
	sgml := "OFXHEADER:100\r\n" +
		"DATA:OFXSGML\r\n" +
		"VERSION:102\r\n" +
		"ENCODING:USASCII\r\n" +
		"CHARSET:NONE\r\n" +
		"\r\n" +
		"<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>1<MEMO>Farm" + string([]byte{0xE1}) + "cia</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if want := "Farmácia"; rows[0].Description != want {
		t.Fatalf("description = %q, want %q (real CP1252 bytes must decode even if the header lies and says USASCII)", rows[0].Description, want)
	}
}

func TestParseOFXDoesNotPanicOnInvalidBytesBeforeTheHeaderKeywords(t *testing.T) {
	garbage := strings.Repeat(string([]byte{0xE3}), 10)
	sgml := garbage + "CHARSET:1\r\n\r\n<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>1<NAME>X</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatalf("ParseOFX must not panic nor error on this input: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
}

func TestParseOFXKeepsValidUTF8Untouched(t *testing.T) {
	rows, err := ParseOFX([]byte(sgmlSample))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Description != "Compra no mercado" {
		t.Fatalf("description = %q", rows[0].Description)
	}
}

func TestParseOFXAmountAcceptsCommaDecimalSeparator(t *testing.T) {
	sgml := "<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-89,90<FITID>1<NAME>X</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatalf("ParseOFX: %v", err)
	}
	if rows[0].AmountCents != 8990 || rows[0].Type != Expense {
		t.Fatalf("row = %+v", rows[0])
	}
}

func TestParseOFXAmountRejectsMalformedValues(t *testing.T) {
	for _, amount := range []string{"--5.00", "5.-3", "5.123", "-92233720368547758070.00"} {
		sgml := "<OFX><BANKTRANLIST>" +
			"<STMTTRN><DTPOSTED>20261001<TRNAMT>" + amount + "<FITID>1<NAME>X</STMTTRN>" +
			"</BANKTRANLIST></OFX>"
		if _, err := ParseOFX([]byte(sgml)); err == nil {
			t.Fatalf("amount %q: expected error", amount)
		}
	}
}

func TestParseOFXDecodesHTMLEntitiesInDescription(t *testing.T) {
	sgml := "<OFX><BANKTRANLIST>" +
		"<STMTTRN><DTPOSTED>20261001<TRNAMT>-10<FITID>1<NAME>C&amp;A</STMTTRN>" +
		"</BANKTRANLIST></OFX>"
	rows, err := ParseOFX([]byte(sgml))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Description != "C&A" {
		t.Fatalf("description = %q, want C&A", rows[0].Description)
	}
}

func TestSuggestCategoryFallsBackToDefaultWithoutHistory(t *testing.T) {
	history := map[string]int64{"COMPRA NO MERCADO": 7}
	if got := SuggestCategory(history, "COMPRA NO MERCADO", 9); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
	if got := SuggestCategory(history, "DESCONHECIDO", 9); got != 9 {
		t.Fatalf("got %d, want 9 (fallback)", got)
	}
}
