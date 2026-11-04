package domain

import (
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var accentFolder = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

func NormalizeForSort(s string) string {
	folded, _, err := transform.String(accentFolder, s)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return strings.ToLower(strings.TrimSpace(folded))
}

func CalculateClassDates(start time.Time, weekdays []time.Weekday, totalPeriods int) []time.Time {
	if totalPeriods <= 0 || len(weekdays) == 0 {
		return nil
	}

	validDays := make(map[time.Weekday]bool, len(weekdays))
	for _, w := range weekdays {
		validDays[w] = true
	}

	dates := make([]time.Time, 0, totalPeriods)
	curr := start
	for len(dates) < totalPeriods {
		if validDays[curr.Weekday()] {
			dates = append(dates, curr)
		}
		curr = curr.AddDate(0, 0, 1)
	}
	return dates
}

func SortAndGroupStudents(students []Student) ([]Student, []Student) {
	grad := make([]Student, 0, len(students))
	posGrad := make([]Student, 0, len(students))

	for _, s := range students {
		switch s.Level {
		case LevelGraduation:
			grad = append(grad, s)
		case LevelPosGraduation:
			posGrad = append(posGrad, s)
		}
	}

	sortStudents(grad)
	sortStudents(posGrad)
	return grad, posGrad
}

func sortStudents(list []Student) {
	sort.SliceStable(list, func(i, j int) bool {
		normI := NormalizeForSort(list[i].FullName)
		normJ := NormalizeForSort(list[j].FullName)
		if normI == normJ {
			return list[i].FullName < list[j].FullName
		}
		return normI < normJ
	})
}
