package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Parse the raw password from command line arguments
	password := flag.String("p", "", "The plain-text password to hash")
	flag.Parse()

	if *password == "" {
		fmt.Println("Usage: go run cmd/hasher/main.go -p <your-password>")
		os.Exit(1)
	}

	// Hash the password with a cost of 10
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	fmt.Println("\n🔒 Plain-text:")
	fmt.Printf("   %s\n", *password)

	fmt.Println("\n🔑 Bcrypt Hash (Copy this to your database):")
	fmt.Printf("   %s\n\n", string(hashedBytes))
}
