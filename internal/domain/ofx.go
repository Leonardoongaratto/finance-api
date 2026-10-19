package domain

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrInvalidOFX = errors.New("file is not a valid OFX statement")

// OFXTransaction is one STMTTRN parsed from an OFX 1.x (SGML) or OFX 2.x
// (XML) bank statement, normalized to this app's money and date rules.
type OFXTransaction struct {
	FITID       string
	Date        Date
	AmountCents int64
	Type        TransactionType
	Description string
}

type ofxToken struct {
	name    string
	closing bool
	value   string
}

// ParseOFX reads either OFX grammar without any third-party dependency: OFX
// 1.x SGML, whose leaf tags are never closed, and OFX 2.x XML, whose tags
// always are. Both dialects close every aggregate tag (e.g. STMTTRN), which
// is the only closing behavior this parser relies on.
func ParseOFX(data []byte) ([]OFXTransaction, error) {
	body := skipHeader(decodeToUTF8(data))
	if !strings.Contains(strings.ToUpper(body), "<OFX") {
		return nil, ErrInvalidOFX
	}

	tokens, err := tokenize(body)
	if err != nil {
		return nil, err
	}

	rows, err := extractTransactions(tokens)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// decodeToUTF8 decides the encoding from the bytes themselves, never from
// the header's own claim. A declared CHARSET/ENCODING cannot be trusted:
// exporters routinely copy a CP1252 header template while writing UTF-8, and
// others omit it while writing CP1252/ISO-8859-1 anyway (common in Brazilian
// bank exports). Genuine CP1252 text with accents almost never forms a valid
// UTF-8 sequence, so deciding by validity is both simpler and more reliable
// than trusting the declaration.
func decodeToUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	return cp1252ToUTF8(data)
}

// cp1252HighRange maps the CP1252 code points in 0x80-0x9F that differ from
// ISO-8859-1 (which maps that range to the matching C1 control codes). Bytes
// 0xA0-0xFF match their Unicode code point in both encodings.
var cp1252HighRange = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…',
	0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š',
	0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž', 0x91: '‘', 0x92: '’',
	0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—',
	0x98: '˜', 0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ',
	0x9E: 'ž', 0x9F: 'Ÿ',
}

func cp1252ToUTF8(data []byte) string {
	var b strings.Builder
	b.Grow(len(data))
	for _, c := range data {
		switch {
		case c < 0x80:
			b.WriteByte(c)
		case c >= 0x80 && c <= 0x9F:
			if r, ok := cp1252HighRange[c]; ok {
				b.WriteRune(r)
			} else {
				b.WriteRune(rune(c))
			}
		default:
			b.WriteRune(rune(c))
		}
	}
	return b.String()
}

var entityReplacer = strings.NewReplacer(
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", "\"",
	"&apos;", "'",
	"&amp;", "&",
)

func skipHeader(raw string) string {
	i := strings.IndexByte(raw, '<')
	if i < 0 {
		return raw
	}
	return raw[i:]
}

func tokenize(body string) ([]ofxToken, error) {
	var tokens []ofxToken
	i := 0
	for i < len(body) {
		if body[i] != '<' {
			i++
			continue
		}
		end := strings.IndexByte(body[i:], '>')
		if end < 0 {
			return nil, ErrInvalidOFX
		}
		end += i
		content := strings.TrimSpace(body[i+1 : end])
		i = end + 1

		if content == "" || content[0] == '?' || content[0] == '!' {
			continue
		}

		closing := strings.HasPrefix(content, "/")
		name := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(content, "/")))

		next := strings.IndexByte(body[i:], '<')
		var text string
		if next < 0 {
			text = strings.TrimSpace(body[i:])
			i = len(body)
		} else {
			text = strings.TrimSpace(body[i : i+next])
			i += next
		}
		tokens = append(tokens, ofxToken{name: name, closing: closing, value: entityReplacer.Replace(text)})
	}
	return tokens, nil
}

func extractTransactions(tokens []ofxToken) ([]OFXTransaction, error) {
	var rows []OFXTransaction
	open := false
	fields := map[string]string{}

	for _, tok := range tokens {
		switch {
		case tok.name == "STMTTRN" && !tok.closing:
			if open {
				return nil, ErrInvalidOFX
			}
			open = true
			fields = map[string]string{}
		case tok.name == "STMTTRN" && tok.closing:
			if !open {
				return nil, ErrInvalidOFX
			}
			row, err := buildTransaction(fields)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
			open = false
		case open && !tok.closing && tok.value != "":
			if _, exists := fields[tok.name]; !exists {
				fields[tok.name] = tok.value
			}
		}
	}
	if open {
		return nil, ErrInvalidOFX
	}
	return rows, nil
}

func buildTransaction(fields map[string]string) (OFXTransaction, error) {
	fitid := strings.TrimSpace(fields["FITID"])
	if fitid == "" {
		return OFXTransaction{}, ErrInvalidOFX
	}

	date, err := parseOFXDate(fields["DTPOSTED"])
	if err != nil {
		return OFXTransaction{}, err
	}

	cents, txType, err := parseOFXAmount(fields["TRNAMT"])
	if err != nil {
		return OFXTransaction{}, err
	}

	description := strings.TrimSpace(fields["MEMO"])
	if description == "" {
		description = strings.TrimSpace(fields["NAME"])
	}

	return OFXTransaction{
		FITID:       fitid,
		Date:        date,
		AmountCents: cents,
		Type:        txType,
		Description: description,
	}, nil
}

func parseOFXDate(raw string) (Date, error) {
	if len(raw) < 8 {
		return "", ErrInvalidOFX
	}
	yyyy, mm, dd := raw[0:4], raw[4:6], raw[6:8]
	if _, err := strconv.Atoi(yyyy + mm + dd); err != nil {
		return "", ErrInvalidOFX
	}
	return ParseDate(yyyy + "-" + mm + "-" + dd)
}

// maxAmountUnits is the largest whole-currency part that still leaves room
// for the cents when multiplied by 100 and added, without overflowing int64.
const maxAmountUnits = (math.MaxInt64 - 99) / 100

func parseOFXAmount(raw string) (int64, TransactionType, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, "", ErrInvalidOFX
	}
	raw = normalizeDecimalSeparator(raw)

	negative := false
	switch {
	case strings.HasPrefix(raw, "-"):
		negative = true
		raw = raw[1:]
	case strings.HasPrefix(raw, "+"):
		raw = raw[1:]
	}
	if raw == "" || strings.ContainsAny(raw, "+-") {
		return 0, "", ErrInvalidOFX
	}

	whole, frac := raw, ""
	if dot := strings.IndexByte(raw, '.'); dot >= 0 {
		whole, frac = raw[:dot], raw[dot+1:]
	}
	if whole == "" {
		whole = "0"
	}
	if !isDigits(whole) || !isDigits(frac) || len(frac) > 2 {
		return 0, "", ErrInvalidOFX
	}
	for len(frac) < 2 {
		frac += "0"
	}

	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || units > maxAmountUnits {
		return 0, "", ErrInvalidOFX
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, "", ErrInvalidOFX
	}
	total := units*100 + cents
	if total == 0 {
		return 0, "", ErrInvalidOFX
	}

	if negative {
		return total, Expense, nil
	}
	return total, Income, nil
}

// normalizeDecimalSeparator accepts the comma as a decimal symbol, which the
// OFX specification allows alongside the dot and some Brazilian exports use.
func normalizeDecimalSeparator(raw string) string {
	if !strings.Contains(raw, ".") && strings.Count(raw, ",") == 1 {
		return strings.Replace(raw, ",", ".", 1)
	}
	return raw
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// NormalizeDescription makes two descriptions of the same merchant compare
// equal regardless of case or incidental whitespace.
func NormalizeDescription(s string) string {
	fields := strings.Fields(strings.ToUpper(s))
	return strings.Join(fields, " ")
}

// SuggestCategory looks up the category used the last time this account had
// a transaction with the same normalized description, falling back to a
// default (e.g. "Outros") when there is no history.
func SuggestCategory(history map[string]int64, normalizedDescription string, fallbackCategoryID int64) int64 {
	if id, ok := history[normalizedDescription]; ok {
		return id
	}
	return fallbackCategoryID
}
