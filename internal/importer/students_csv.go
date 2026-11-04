// Package importer parses student roster files exported from spreadsheets.
package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"frequencia-matrix-integration/internal/domain"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const (
	// MaxBytes is the largest accepted file.
	MaxBytes = 1 << 20
	// MaxRows is the largest accepted number of students in one file.
	MaxRows = 2000
)

var (
	ErrTooLarge   = fmt.Errorf("csv file is larger than %d bytes", MaxBytes)
	ErrTooManyRow = fmt.Errorf("csv file has more than %d students", MaxRows)
)

// StudentRow is one student read from the file: [name, registration].
type StudentRow struct {
	Name         string
	Registration string
}

// SkippedRow explains why a line was not imported.
type SkippedRow struct {
	Line   int
	Reason string
}

// Result holds the importable rows and the lines that were skipped.
type Result struct {
	Rows    []StudentRow
	Skipped []SkippedRow
}

var headerNames = map[string]bool{"aluno": true, "nome": true, "nome completo": true, "estudante": true}
var headerRegistrations = map[string]bool{"matricula": true, "registro": true}

// ParseStudentsCSV reads a two column [aluno, matricula] file. It accepts
// comma, semicolon or tab separators, an optional header line, a UTF-8 BOM,
// and falls back to Windows-1252 or UTF-16 as exported by spreadsheets.
func ParseStudentsCSV(r io.Reader) (Result, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("failed reading csv: %w", err)
	}
	if len(raw) > MaxBytes {
		return Result{}, ErrTooLarge
	}
	text, err := decodeText(raw)
	if err != nil {
		return Result{}, err
	}
	return readRecords(text)
}

func decodeText(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}), bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return transformBytes(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(), raw)
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		return string(raw[3:]), nil
	case utf8.Valid(raw):
		return string(raw), nil
	default:
		return transformBytes(charmap.Windows1252.NewDecoder(), raw)
	}
}

func transformBytes(t transform.Transformer, raw []byte) (string, error) {
	out, _, err := transform.Bytes(t, raw)
	if err != nil {
		return "", fmt.Errorf("failed decoding csv text: %w", err)
	}
	return string(out), nil
}

func readRecords(text string) (Result, error) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = detectDelimiter(text)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	var result Result
	seen := make(map[string]bool)
	first := true
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedRow{Line: errorLine(err), Reason: "linha malformada"})
			continue
		}
		line, _ := reader.FieldPos(0)
		if first {
			first = false
			if isHeader(record) {
				continue
			}
		}
		if err := collect(&result, seen, record, line); err != nil {
			return Result{}, err
		}
	}
}

func errorLine(err error) int {
	var parseErr *csv.ParseError
	if errors.As(err, &parseErr) {
		return parseErr.StartLine
	}
	return 0
}

func collect(result *Result, seen map[string]bool, record []string, line int) error {
	row, reason := parseRecord(record)
	if reason == "" {
		reason = markDuplicate(seen, row)
	}
	switch reason {
	case "":
		if len(result.Rows) >= MaxRows {
			return ErrTooManyRow
		}
		result.Rows = append(result.Rows, row)
	case "linha vazia":
	default:
		result.Skipped = append(result.Skipped, SkippedRow{Line: line, Reason: reason})
	}
	return nil
}

func parseRecord(record []string) (StudentRow, string) {
	name := strings.Join(strings.Fields(field(record, 0)), " ")
	registration := strings.TrimSpace(field(record, 1))
	if name == "" && registration == "" {
		return StudentRow{}, "linha vazia"
	}
	if name == "" {
		return StudentRow{}, "nome vazio"
	}
	return StudentRow{Name: name, Registration: registration}, ""
}

// markDuplicate flags a repeated registration, or a repeated name when the
// line has no registration to tell students apart.
func markDuplicate(seen map[string]bool, row StudentRow) string {
	key := "n:" + domain.NormalizeForSort(row.Name)
	if row.Registration != "" {
		key = "r:" + row.Registration
	}
	if seen[key] {
		return "duplicado no arquivo"
	}
	seen[key] = true
	return ""
}

func field(record []string, i int) string {
	if i < len(record) {
		return record[i]
	}
	return ""
}

func isHeader(record []string) bool {
	name := domain.NormalizeForSort(field(record, 0))
	registration := domain.NormalizeForSort(field(record, 1))
	return headerNames[name] || headerRegistrations[registration]
}

// detectDelimiter picks the separator that appears most on the first
// non-empty line, ignoring characters inside quotes.
func detectDelimiter(text string) rune {
	line := firstLine(text)
	counts := map[rune]int{}
	inQuotes := false
	for _, c := range line {
		switch {
		case c == '"':
			inQuotes = !inQuotes
		case !inQuotes && (c == ';' || c == ',' || c == '\t'):
			counts[c]++
		}
	}
	best, bestCount := ',', 0
	for _, c := range []rune{';', '\t', ','} {
		if counts[c] > bestCount {
			best, bestCount = c, counts[c]
		}
	}
	return best
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}
