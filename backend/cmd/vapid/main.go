// Command vapid generates the key pair web push needs. Run it once and put the
// two values in .env; keep the private key secret.
//
//	go run ./backend/cmd/vapid
package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatalf("could not generate keys: %v", err)
	}
	fmt.Println("# Add these to .env (VAPID_SUBJECT is a contact the push services can reach you at):")
	fmt.Printf("VAPID_PUBLIC_KEY=%s\n", public)
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", private)
	fmt.Println("VAPID_SUBJECT=mailto:you@example.com")
}
