package journal

const CrashLimit = 3

type ResumeState struct {
	Records  int `json:"records"`
	Attempts int `json:"attempts"`
}

func WorkedRecords(recs []Record) int {
	n := 0
	for _, rec := range recs {
		if rec.Type == TypeDetached {
			continue
		}
		n++
	}
	return n
}

func NextResume(st State, recs []Record, prev ResumeState) (bool, ResumeState) {
	if st.Finished || st.Stopped {
		return false, prev
	}
	records := WorkedRecords(recs)
	attempts := prev.Attempts
	if prev.Records != records {
		attempts = 0
	}
	if attempts >= CrashLimit {
		return false, prev
	}
	return true, ResumeState{Records: records, Attempts: attempts + 1}
}
