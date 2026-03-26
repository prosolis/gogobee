package plugin

import (
	"testing"
)

func TestScoreGuess_ExactMatch(t *testing.T) {
	results := scoreGuess("HELLO", "HELLO")
	for i, r := range results {
		if r != LetterCorrect {
			t.Errorf("position %d: got %d, want LetterCorrect", i, r)
		}
	}
}

func TestScoreGuess_AllAbsent(t *testing.T) {
	results := scoreGuess("XXXYZ", "HELLO")
	for i, r := range results {
		if r != LetterAbsent {
			t.Errorf("position %d: got %d, want LetterAbsent", i, r)
		}
	}
}

func TestScoreGuess_MixedResults(t *testing.T) {
	// WORLD vs HELLO:
	// W=absent, O=present, R=absent, L=correct(pos3 matches), D=absent
	results := scoreGuess("WORLD", "HELLO")
	expected := []LetterResult{LetterAbsent, LetterPresent, LetterAbsent, LetterCorrect, LetterAbsent}
	for i, r := range results {
		if r != expected[i] {
			t.Errorf("position %d: got %d, want %d", i, r, expected[i])
		}
	}
}

func TestScoreGuess_DuplicateLetters(t *testing.T) {
	// SPEED vs ABIDE:
	// S=absent, P=absent, E=present(E is in ABIDE pos 4), E=present? no, only one E in answer
	// Actually: ABIDE has E at position 4 (0-indexed: A=0,B=1,I=2,D=3,E=4)
	// SPEED: S=absent, P=absent, E=absent(no more E after first match), E=present, D=present
	results := scoreGuess("SPEED", "ABIDE")
	// S not in ABIDE -> absent
	// P not in ABIDE -> absent
	// E in ABIDE pos4 -> present (consumes the E)
	// E again but no more E in pool -> absent
	// D in ABIDE pos3 -> present
	expected := []LetterResult{LetterAbsent, LetterAbsent, LetterPresent, LetterAbsent, LetterPresent}
	for i, r := range results {
		if r != expected[i] {
			t.Errorf("position %d: got %d, want %d", i, r, expected[i])
		}
	}
}

func TestScoreGuess_DuplicateWithCorrect(t *testing.T) {
	// LLAMA vs HELLO:
	// L at 0: not correct (H), L is in HELLO -> present (consumes one L)
	// L at 1: not correct (E), L is in HELLO -> present (consumes second L)
	// A at 2: absent
	// M at 3: absent
	// A at 4: absent
	// Wait, HELLO has L at positions 2,3. So pool after first pass has H,E,L,L,O
	// Actually no first pass: no exact matches for LLAMA vs HELLO
	// Pool = H,E,L,L,O
	// L at 0: found L in pool, present (pool = H,E,L,O)
	// L at 1: found L in pool, present (pool = H,E,O)
	// A at 2: not found, absent
	// M at 3: not found, absent
	// A at 4: not found, absent
	results := scoreGuess("LLAMA", "HELLO")
	expected := []LetterResult{LetterPresent, LetterPresent, LetterAbsent, LetterAbsent, LetterAbsent}
	for i, r := range results {
		if r != expected[i] {
			t.Errorf("position %d: got %d, want %d", i, r, expected[i])
		}
	}
}

func TestScoreGuess_CorrectTakesPriority(t *testing.T) {
	// HELLO vs LLAMA: H=absent, E=absent, L=correct(pos2? no)
	// HELLO vs LEMON: H=absent, E=present, L=present, L=absent, O=present
	// Let's use a clearer case:
	// ALLAY vs LLAMA:
	// A at 0: not L -> but A is in LLAMA. First pass: check exact matches
	// pos0: A vs L -> no
	// pos1: L vs L -> CORRECT
	// pos2: L vs A -> no
	// pos3: A vs M -> no
	// pos4: Y vs A -> no
	// Pool (unmatched answer letters): L(0), A(2), M(3), A(4)
	// Second pass:
	// pos0: A -> found A in pool, present (pool: L, M, A)
	// pos2: L -> found L in pool, present (pool: M, A)
	// pos3: A -> found A in pool, present (pool: M)
	// pos4: Y -> not found, absent
	results := scoreGuess("ALLAY", "LLAMA")
	expected := []LetterResult{LetterPresent, LetterCorrect, LetterPresent, LetterPresent, LetterAbsent}
	for i, r := range results {
		if r != expected[i] {
			t.Errorf("position %d: got %d, want %d", i, r, expected[i])
		}
	}
}

func TestUpdateLetterStates_Upgrades(t *testing.T) {
	states := map[rune]LetterResult{}

	// First guess: A is absent
	updateLetterStates(states, "A", []LetterResult{LetterAbsent})
	if states['A'] != LetterAbsent {
		t.Errorf("A should be Absent, got %d", states['A'])
	}

	// Second guess: A is present — should upgrade
	updateLetterStates(states, "A", []LetterResult{LetterPresent})
	if states['A'] != LetterPresent {
		t.Errorf("A should be Present after upgrade, got %d", states['A'])
	}

	// Third guess: A is correct — should upgrade again
	updateLetterStates(states, "A", []LetterResult{LetterCorrect})
	if states['A'] != LetterCorrect {
		t.Errorf("A should be Correct after upgrade, got %d", states['A'])
	}
}

func TestUpdateLetterStates_NoDowngrade(t *testing.T) {
	states := map[rune]LetterResult{
		'A': LetterCorrect,
	}

	// A was correct, now appears absent in a different position — should NOT downgrade
	updateLetterStates(states, "A", []LetterResult{LetterAbsent})
	if states['A'] != LetterCorrect {
		t.Errorf("A should remain Correct, got %d", states['A'])
	}
}

func TestWordleBasePots(t *testing.T) {
	// Verify reward structure: fewer guesses = more money
	for i := 1; i < len(wordleBasePots)-1; i++ {
		if wordleBasePots[i] <= wordleBasePots[i+1] {
			t.Errorf("pot for %d guesses (%d) should be > pot for %d guesses (%d)",
				i, wordleBasePots[i], i+1, wordleBasePots[i+1])
		}
	}
}
