package jev

import "sort"

func splitQuestions(qs map[string]Question) []map[string]Question {
	names := sortedQuestionNames(qs)
	if len(names) <= maxQuestionsPerReq {
		return []map[string]Question{qs}
	}
	var chunks []map[string]Question
	for i := 0; i < len(names); i += maxQuestionsPerReq {
		end := i + maxQuestionsPerReq
		if end > len(names) {
			end = len(names)
		}
		chunk := make(map[string]Question, end-i)
		for _, name := range names[i:end] {
			chunk[name] = qs[name]
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func sortedQuestionNames(qs map[string]Question) []string {
	names := make([]string, 0, len(qs))
	for name := range qs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
