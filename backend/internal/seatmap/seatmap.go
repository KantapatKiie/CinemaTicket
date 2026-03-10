package seatmap

import "fmt"

type SeatState struct {
	SeatID string
	Status string
}

func LockKey(showID, seatID string) string {
	return fmt.Sprintf("lock:seat:%s:%s", showID, seatID)
}

func Build(rows, cols int) []SeatState {
	result := make([]SeatState, 0, rows*cols)
	for r := 0; r < rows; r++ {
		rowRune := rune('A' + r)
		for c := 1; c <= cols; c++ {
			result = append(result, SeatState{SeatID: fmt.Sprintf("%c%d", rowRune, c), Status: "AVAILABLE"})
		}
	}
	return result
}
