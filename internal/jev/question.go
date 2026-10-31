package jev

type Question struct {
	Type         string
	Instructions any
	Criteria     any
}

func Noul(instructions any) Question {
	return Question{Type: "noul", Instructions: instructions}
}

func Choice(instructions any, criteria map[string]string) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: criteria}
}
