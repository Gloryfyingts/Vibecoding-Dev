package main

import (
	"fmt"
	"math/rand"
	"time"
)

var quotes = []string{
	"Simplicity is the ultimate sophistication.",
	"Talk is cheap. Show me the code.",
	"First, solve the problem. Then, write the code.",
	"Code is like humor. When you have to explain it, it's bad.",
	"Any fool can write code that a computer can understand. Good programmers write code that humans can understand.",
	"The best error message is the one that never shows up.",
	"Make it work, make it right, make it fast.",
}

func main() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	idx := r.Intn(len(quotes))
	fmt.Println(quotes[idx])
}
