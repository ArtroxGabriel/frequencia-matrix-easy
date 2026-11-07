package domain

// MinAttendancePercent is the minimum presence needed to be approved.
const MinAttendancePercent = 75

// MaxAbsencesFor returns how many periods a student may miss while still
// reaching MinAttendancePercent of the total course load.
func MaxAbsencesFor(totalPeriods int) int {
	if totalPeriods <= 0 {
		return 0
	}
	return totalPeriods * (100 - MinAttendancePercent) / 100
}

// CountPeriods sums the periods a student attended and missed. Cancelled
// sessions are skipped, so a class that did not happen never counts.
func CountPeriods(studentID string, sessions []AttendanceSession) (presences, absences int) {
	for _, session := range sessions {
		if session.Cancelled {
			continue
		}
		for _, entry := range session.Entries {
			if entry.StudentID != studentID {
				continue
			}
			if entry.IsPresent {
				presences += session.PeriodCount
			} else {
				absences += session.PeriodCount
			}
			break
		}
	}
	return presences, absences
}

// ClassifyAbsences compares absences with the limit. A student is in warning
// once 75% of the allowed absences are used.
func ClassifyAbsences(absences, maxAbsences int) AttendanceStatus {
	switch {
	case absences > maxAbsences:
		return StatusFailed
	case absences > 0 && absences*4 >= maxAbsences*3:
		return StatusWarning
	default:
		return StatusOK
	}
}

// CalculateStudentSummary assumes a student is present in every class that
// has no absence recorded: absences start at zero and only grow when the
// teacher marks one. The percentages are measured against the total course
// load, so no one is flagged before the limit is actually exceeded.
func CalculateStudentSummary(student Student, sessions []AttendanceSession, totalCoursePeriods int) StudentSummary {
	presences, absences := CountPeriods(student.ID, sessions)
	maxAbsences := MaxAbsencesFor(totalCoursePeriods)
	status := ClassifyAbsences(absences, maxAbsences)

	absencePercent, attendancePercent := 0.0, 0.0
	if totalCoursePeriods > 0 {
		absencePercent = float64(absences) / float64(totalCoursePeriods) * 100.0
		attendancePercent = 100.0 - absencePercent
	}

	return StudentSummary{
		Student:           student,
		Presences:         presences,
		Absences:          absences,
		AbsencePercent:    absencePercent,
		AttendancePercent: attendancePercent,
		MaxAbsences:       maxAbsences,
		RemainingAbsences: max(maxAbsences-absences, 0),
		Status:            status,
		IsApproved:        totalCoursePeriods > 0 && status != StatusFailed,
	}
}
