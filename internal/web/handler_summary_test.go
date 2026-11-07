package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSummaryShowsStatusInactiveAndIgnoresCancelledClasses(t *testing.T) {
	env := newTeacherEnv(t)
	ids := env.addStudents(t, "Ana Em Dia", "Bia Atencao", "Caio Reprovado", "Dani Inativa")

	// Limit for 32 periods is 8. Bia misses 3 classes (6 periods, warning),
	// Caio misses 5 classes (10 periods, failed), Dani is removed afterwards.
	for i, date := range []string{"2026-03-03", "2026-03-05", "2026-03-10", "2026-03-12", "2026-03-17"} {
		absent := url.Values{"classDate": {date}, "periodCount": {"2"}, "absent": {ids[2]}}
		if i < 3 {
			absent.Add("absent", ids[1])
		}
		env.post("/widget/attendance", absent)
	}
	env.post("/widget/attendance/cancel", url.Values{"classDate": {"2026-03-19"}, "cancelled": {"true"}})
	_ = env.store.DeactivateStudent(context.Background(), testRoom, ids[3])

	rec := env.get("/widget/tab/summary" + env.auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{"Em dia", "Atenção", "Reprovado por falta", "inativo", "8 períodos", "75%"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in summary", want)
		}
	}
	if !strings.Contains(body, `text-2xl font-bold text-blue-950">5<`) {
		t.Errorf("expected 5 held classes (the cancelled one is not counted)")
	}
}

func TestSummaryStartsEveryoneWithZeroAbsences(t *testing.T) {
	env := newTeacherEnv(t)
	env.addStudents(t, "Ana")

	body := env.get("/widget/tab/summary" + env.auth).Body.String()

	if !strings.Contains(body, "0.0%") || !strings.Contains(body, "100.0%") || strings.Contains(body, "Reprovado") {
		t.Errorf("a student without absences must show 0%% absences, 100%% attendance and no failure")
	}
}
