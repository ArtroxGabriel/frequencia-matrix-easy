package web_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"frequencia-matrix-integration/internal/domain"
	"frequencia-matrix-integration/internal/importer"
	"frequencia-matrix-integration/internal/matrix"
)

func (e *teacherEnv) postMultipart(auth string, fields map[string]string, filename, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if filename != "" {
		part, _ := mw.CreateFormFile("file", filename)
		_, _ = part.Write([]byte(content))
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/widget/students/import"+auth, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *teacherEnv) activeStudents(t *testing.T) []domain.Student {
	t.Helper()
	students, err := e.store.ListStudents(context.Background(), testRoom, true)
	if err != nil {
		t.Fatalf("failed listing students: %v", err)
	}
	return students
}

func TestImportStudentsFromCSV(t *testing.T) {
	env := newTeacherEnv(t)
	csv := "aluno;matricula\nÉrika Souza;5011\nDaniel Lima;5012\n;99\nJoão Pedro;5013\n"

	rec := env.postMultipart(env.auth, map[string]string{"level": "POS_GRADUATION"}, "alunos.csv", csv)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	students := env.activeStudents(t)
	if len(students) != 3 {
		t.Fatalf("expected 3 imported students (the line without a name is skipped), got %d", len(students))
	}
	for _, s := range students {
		if s.Level != domain.LevelPosGraduation {
			t.Errorf("student %s must be imported as Pós-Graduação, got %s", s.FullName, s.Level)
		}
	}
	body := rec.Body.String()
	if strings.Index(body, "Daniel Lima") > strings.Index(body, "Érika Souza") || !strings.Contains(body, "Matrícula: 5011") {
		t.Errorf("expected the updated roster, sorted ignoring accents, in the response")
	}
}

func TestImportStudentsTwiceDoesNotDuplicate(t *testing.T) {
	env := newTeacherEnv(t)
	fields := map[string]string{"level": "GRADUATION"}

	env.postMultipart(env.auth, fields, "a.csv", "Ana,1\nBia,2\n")
	env.postMultipart(env.auth, fields, "a.csv", "Ana,1\nBia,2\nCaio,3\n")

	if n := len(env.activeStudents(t)); n != 3 {
		t.Errorf("expected 3 students after a second import, got %d", n)
	}
}

func TestImportStudentsRejectsBadRequestsSilently(t *testing.T) {
	env := newTeacherEnv(t)
	big := strings.Repeat("Aluno Muito Longo,1\n", importer.MaxBytes/10)
	cases := []struct {
		name     string
		fields   map[string]string
		filename string
		content  string
		want     int
	}{
		{"invalid level", map[string]string{"level": "NOPE"}, "a.csv", "Ana,1\n", http.StatusUnprocessableEntity},
		{"missing level", map[string]string{}, "a.csv", "Ana,1\n", http.StatusUnprocessableEntity},
		{"missing file", map[string]string{"level": "GRADUATION"}, "", "", http.StatusUnprocessableEntity},
		{"file too large", map[string]string{"level": "GRADUATION"}, "big.csv", big, http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		rec := env.postMultipart(env.auth, c.fields, c.filename, c.content)
		if rec.Code != c.want || rec.Body.Len() != 0 {
			t.Errorf("%s: want empty %d, got %d with body length %d", c.name, c.want, rec.Code, rec.Body.Len())
		}
	}
	if n := len(env.activeStudents(t)); n != 0 {
		t.Errorf("rejected imports must not create students, got %d", n)
	}
}

func TestImportStudentsWithOnlyUnusableLinesChangesNothing(t *testing.T) {
	env := newTeacherEnv(t)

	rec := env.postMultipart(env.auth, map[string]string{"level": "GRADUATION"}, "a.csv", ",1\n,2\n")

	if rec.Code != http.StatusOK || len(env.activeStudents(t)) != 0 {
		t.Errorf("expected 200 and no students, got %d", rec.Code)
	}
}

func TestImportStudentsIsTeacherOnly(t *testing.T) {
	env := newTeacherEnv(t)
	env.matrix.SetUserPowerLevel(testRoom, "@aluno:matrix.dc.ufc.br", 0)
	env.matrix.RegisterOpenIDToken("student-token", matrix.UserIdentity{UserID: "@aluno:matrix.dc.ufc.br", RoomID: testRoom})
	studentAuth := "?roomId=%21course%3Amatrix.dc.ufc.br&openIdToken=student-token"

	rec := env.postMultipart(studentAuth, map[string]string{"level": "GRADUATION"}, "a.csv", "Ana,1\n")

	if rec.Code != http.StatusForbidden || len(env.activeStudents(t)) != 0 {
		t.Errorf("a student must not import a roster, got %d", rec.Code)
	}
}

func TestStudentsTemplateNeedsNoAuthAndIsParsable(t *testing.T) {
	env := newTeacherEnv(t)

	rec := env.get("/widget/students/template.csv")

	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("expected a csv download, got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	parsed, err := importer.ParseStudentsCSV(rec.Body)
	if err != nil || len(parsed.Rows) != 2 || parsed.Rows[1].Name != "João Pereira" {
		t.Errorf("the template must be importable as is, got %+v (err: %v)", parsed, err)
	}
}
