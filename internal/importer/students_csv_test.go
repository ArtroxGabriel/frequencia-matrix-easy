package importer_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"frequencia-matrix-integration/internal/importer"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

func rows(pairs ...string) []importer.StudentRow {
	out := make([]importer.StudentRow, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, importer.StudentRow{Name: pairs[i], Registration: pairs[i+1]})
	}
	return out
}

func sameRows(a, b []importer.StudentRow) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func TestParseStudentsCSV(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []importer.StudentRow
	}{
		{"comma with header", "aluno,matricula\nÉrika Souza,5011\nDaniel Lima,5012\n", rows("Érika Souza", "5011", "Daniel Lima", "5012")},
		{"semicolon without header", "João Pedro;5013\nÁgatha Reis;9001\n", rows("João Pedro", "5013", "Ágatha Reis", "9001")},
		{"tab separated", "Nome\tMatrícula\nAna\t1\n", rows("Ana", "1")},
		{"utf8 bom", "\xEF\xBB\xBFaluno;matricula\nAna;1\n", rows("Ana", "1")},
		{"crlf line endings", "Ana;1\r\nBia;2\r\n", rows("Ana", "1", "Bia", "2")},
		{"quoted name with comma", "\"Silva, João\",5011\n", rows("Silva, João", "5011")},
		{"quoted name with comma and semicolon", "\"Silva, João\";5011\n", rows("Silva, João", "5011")},
		{"missing registration", "Ana\nBia,\n", rows("Ana", "", "Bia", "")},
		{"extra spaces and blank lines", "\n  Ana   Maria  ,  77 \n\n\n", rows("Ana Maria", "77")},
		{"header with accent and case", "Nome Completo;Matrícula\nAna;1\n", rows("Ana", "1")},
		{"extra columns are ignored", "Ana;1;curso;outro\n", rows("Ana", "1")},
		{"first line is a student when it looks like one", "Maria Aluno;1\n", rows("Maria Aluno", "1")},
		{"empty file", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := importer.ParseStudentsCSV(strings.NewReader(c.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !sameRows(got.Rows, c.want) {
				t.Errorf("got %+v, want %+v", got.Rows, c.want)
			}
		})
	}
}

func TestParseStudentsCSVDecodesWindows1252(t *testing.T) {
	encoded, err := charmap.Windows1252.NewEncoder().String("João;5013\nÁgatha;9001\n")
	if err != nil {
		t.Fatal(err)
	}

	got, err := importer.ParseStudentsCSV(strings.NewReader(encoded))

	if err != nil || !reflect.DeepEqual(got.Rows, rows("João", "5013", "Ágatha", "9001")) {
		t.Errorf("got %+v (err %v)", got.Rows, err)
	}
}

func TestParseStudentsCSVDecodesUTF16(t *testing.T) {
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder()
	encoded, err := encoder.String("Nome\tMatrícula\nÉrika\t5011\n")
	if err != nil {
		t.Fatal(err)
	}

	got, err := importer.ParseStudentsCSV(strings.NewReader(encoded))

	if err != nil || !reflect.DeepEqual(got.Rows, rows("Érika", "5011")) {
		t.Errorf("got %+v (err %v)", got.Rows, err)
	}
}

func TestParseStudentsCSVReportsSkippedLines(t *testing.T) {
	input := "aluno,matricula\nAna,1\n,2\nBia,1\nCaio,3\nCaio,4\n"

	got, err := importer.ParseStudentsCSV(strings.NewReader(input))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got.Rows, rows("Ana", "1", "Caio", "3", "Caio", "4")) {
		t.Errorf("unexpected rows: %+v", got.Rows)
	}
	want := []importer.SkippedRow{{Line: 3, Reason: "nome vazio"}, {Line: 4, Reason: "duplicado no arquivo"}}
	if !reflect.DeepEqual(got.Skipped, want) {
		t.Errorf("got skipped %+v, want %+v", got.Skipped, want)
	}
}

func TestParseStudentsCSVDeduplicatesByNameWhenNoRegistration(t *testing.T) {
	got, _ := importer.ParseStudentsCSV(strings.NewReader("Érika\nErika\nBia\n"))

	if len(got.Rows) != 2 || len(got.Skipped) != 1 {
		t.Errorf("expected 2 rows and 1 skipped, got %+v", got)
	}
}

func TestParseStudentsCSVLimits(t *testing.T) {
	big := strings.Repeat("a", importer.MaxBytes+1)
	if _, err := importer.ParseStudentsCSV(strings.NewReader(big)); err != importer.ErrTooLarge {
		t.Errorf("expected ErrTooLarge, got %v", err)
	}

	var many strings.Builder
	for i := 0; i <= importer.MaxRows; i++ {
		fmt.Fprintf(&many, "Aluno %d,%d\n", i, i)
	}
	if _, err := importer.ParseStudentsCSV(strings.NewReader(many.String())); err != importer.ErrTooManyRow {
		t.Errorf("expected ErrTooManyRow, got %v", err)
	}
}

func TestParseStudentsCSVSkipsMalformedLine(t *testing.T) {
	got, err := importer.ParseStudentsCSV(strings.NewReader("Ana,1\n\"Bia,2\nCaio,3\n"))

	if err != nil {
		t.Fatalf("a malformed line must not fail the whole import: %v", err)
	}
	if len(got.Rows) == 0 || got.Rows[0].Name != "Ana" {
		t.Errorf("valid lines before the problem must be kept, got %+v", got.Rows)
	}
}
