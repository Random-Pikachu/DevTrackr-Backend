package main

import (
	"fmt"
	"time"

	"github.com/Random-Pikachu/DevTrackr-Backend/internal/collectors"
)

func main() {
	fmt.Println("Testing Leetcode Collector...")

	cf := collectors.NewLeetcodeCollector()

	_, err := cf.FetchDailyActivity("Random_Pikachu", time.Now())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Test finished!")
}