package main

import (
	"fmt"
	"math/rand"
)

func main() {
	target := rand.Intn(100) + 1
	fmt.Println("Guess the number (1-100)!")

	var guess int
	attempts := 0

	for {
		fmt.Print("Your guess: ")
		fmt.Scan(&guess)
		attempts++

		if guess < target {
			fmt.Println("Too low!")
		} else if guess > target {
			fmt.Println("Too high!")
		} else {
			fmt.Printf("Correct! You got it in %d attempts.\n", attempts)
			break
		}
	}
}
