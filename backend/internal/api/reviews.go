package api

import "slices"

// grade grades an answer to the card for the direction of the prompt. Only the terms of the answer side count.
func grade(card Card, direction Direction, answer string) Review {
	polish := direction == RuPl
	terms := append([]string{card.Ru}, card.AlternativesRu...)
	if polish {
		terms = append([]string{card.Pl}, card.AlternativesPl...)
	}
	key := answerKey(answer)
	if slices.ContainsFunc(terms, func(term string) bool { return answerKey(term) == key }) {
		return Review{Grade: Correct, Correction: terms[0]}
	}
	// A learner without a Polish keyboard types the nine Polish letters without their marks.
	if polish {
		key = searchKey(answer)
		if i := slices.IndexFunc(terms, func(term string) bool { return searchKey(term) == key }); i >= 0 {
			return Review{Grade: Almost, Correction: terms[i]}
		}
	}
	return Review{Grade: Again, Correction: terms[0]}
}
