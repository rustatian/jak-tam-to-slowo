package api

import "testing"

func TestGrade(t *testing.T) {
	slowo := Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"Wyraz"}, AlternativesRu: []string{"словечко"}}
	zolw := Card{Pl: "Żółw", Ru: "Черепаха", AlternativesPl: []string{}, AlternativesRu: []string{}}
	tests := []struct {
		name           string
		card           Card
		direction      Direction
		answer         string
		wantGrade      Grade
		wantCorrection string
	}{
		{name: "Polish case and spaces at the ends", card: slowo, direction: RuPl, answer: " słowo ", wantGrade: Correct, wantCorrection: "Słowo"},
		{name: "Russian case and spaces at the ends", card: slowo, direction: PlRu, answer: "  СЛОВО  ", wantGrade: Correct, wantCorrection: "Слово"},
		{name: "a decomposed Polish answer", card: zolw, direction: RuPl, answer: "żółw", wantGrade: Correct, wantCorrection: "Żółw"},
		{name: "a run of white space in an expression", card: Card{Pl: "Zielona herbata", Ru: "Зелёный чай"}, direction: RuPl, answer: " ZIELONA \n HERBATA ", wantGrade: Correct, wantCorrection: "Zielona herbata"},
		{name: "a Polish alternative", card: slowo, direction: RuPl, answer: " WYRAZ ", wantGrade: Correct, wantCorrection: "Słowo"},
		{name: "a Russian alternative", card: slowo, direction: PlRu, answer: "словечко", wantGrade: Correct, wantCorrection: "Слово"},
		{name: "е for ё", card: Card{Pl: "Zielony", Ru: "Зелёный"}, direction: PlRu, answer: "зеленый", wantGrade: Correct, wantCorrection: "Зелёный"},
		{name: "a missing ł", card: slowo, direction: RuPl, answer: "slowo", wantGrade: Almost, wantCorrection: "Słowo"},
		{name: "missing ż, ó and ł", card: zolw, direction: RuPl, answer: "zolw", wantGrade: Almost, wantCorrection: "Żółw"},
		{name: "a missing ó in an alternative gives that alternative", card: Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"Słówko"}}, direction: RuPl, answer: "słowko", wantGrade: Almost, wantCorrection: "Słówko"},
		{name: "the first term that matches is the correction", card: Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"Słówo"}}, direction: RuPl, answer: "slowo", wantGrade: Almost, wantCorrection: "Słowo"},
		{name: "an exact alternative wins over an almost term", card: Card{Pl: "Słowo", Ru: "Слово", AlternativesPl: []string{"slowo"}}, direction: RuPl, answer: "slowo", wantGrade: Correct, wantCorrection: "Słowo"},
		{name: "an extra Polish mark is almost", card: Card{Pl: "Dom", Ru: "Дом"}, direction: RuPl, answer: "dóm", wantGrade: Almost, wantCorrection: "Dom"},
		{name: "a Russian term with Polish letters has no almost grade", card: Card{Pl: "Żabka", Ru: "Żabka"}, direction: PlRu, answer: "zabka", wantGrade: Again, wantCorrection: "Żabka"},
		{name: "и for й is again", card: Card{Pl: "Mój", Ru: "Мой"}, direction: PlRu, answer: "мои", wantGrade: Again, wantCorrection: "Мой"},
		{name: "a Russian answer in the Polish direction", card: slowo, direction: RuPl, answer: "Слово", wantGrade: Again, wantCorrection: "Słowo"},
		{name: "a Polish alternative in the Russian direction", card: slowo, direction: PlRu, answer: "Wyraz", wantGrade: Again, wantCorrection: "Слово"},
		{name: "an empty answer", card: slowo, direction: RuPl, answer: "", wantGrade: Again, wantCorrection: "Słowo"},
		{name: "only white space", card: slowo, direction: PlRu, answer: " \t\n ", wantGrade: Again, wantCorrection: "Слово"},
		{name: "punctuation is a difference", card: slowo, direction: RuPl, answer: "słowo.", wantGrade: Again, wantCorrection: "Słowo"},
		{name: "a mark that is not Polish is again", card: Card{Pl: "Exposé", Ru: "Экспозе"}, direction: RuPl, answer: "expose", wantGrade: Again, wantCorrection: "Exposé"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := grade(tt.card, tt.direction, tt.answer)
			if got.Grade != tt.wantGrade || got.Correction != tt.wantCorrection {
				t.Errorf("grade(%q) = %+v, want {Grade:%s Correction:%s}", tt.answer, got, tt.wantGrade, tt.wantCorrection)
			}
		})
	}
}
