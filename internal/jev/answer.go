package jev

type Answer struct {
	Type       string
	Noul       float64
	Choice     string
	Confidence float64
}

type Answers map[string]Answer
